import { useEffect, useRef } from "react";
import maplibregl from "maplibre-gl";
import type {
  GeoJSONSource,
  Map as MLMap,
  StyleSpecification,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";

import { tileUrlTemplate } from "@/lib/api";
import type { GeoJSONGeometry, Layer } from "@/types";

export type DrawMode = "none" | "point" | "line" | "polygon";

export interface SelectedFeature {
  layer: string;
  id?: number | string;
  properties: Record<string, unknown>;
}

export interface MapViewProps {
  layers: Layer[];
  visible: Record<string, boolean>;
  basemapColor: string;
  flyToBounds?: [number, number, number, number] | null;
  drawMode?: DrawMode;
  /** 每次递增会触发“完成当前绘制”（供移动端按钮使用）。 */
  finishToken?: number;
  onSelectFeature?: (feature: SelectedFeature) => void;
  onDrawComplete?: (geometry: GeoJSONGeometry) => void;
  /** 绘制状态变化（是否在绘制、当前节点数）。 */
  onDrawStateChange?: (drawing: boolean, vertices: number) => void;
}

const EMPTY_STYLE: StyleSpecification = {
  version: 8,
  sources: {},
  layers: [
    {
      id: "background",
      type: "background",
      paint: { "background-color": "#f8fafc" },
    },
  ],
};

const DRAW_SOURCE = "draw-src";

function defaultType(geometryType: string): "fill" | "line" | "circle" {
  const g = geometryType.toUpperCase();
  if (g.includes("POINT")) return "circle";
  if (g.includes("LINE")) return "line";
  return "fill";
}

function buildLayerSpec(l: Layer, visible: boolean): Record<string, any> {
  const style = (l.style ?? {}) as Record<string, any>;
  const type: string = style.type ?? defaultType(l.geometryType);
  return {
    id: `lyr-${l.name}`,
    type,
    source: `src-${l.name}`,
    "source-layer": l.name,
    minzoom: l.minZoom,
    maxzoom: l.maxZoom,
    layout: { ...(style.layout ?? {}), visibility: visible ? "visible" : "none" },
    paint: style.paint ?? defaultPaint(type),
  };
}

function defaultPaint(type: string): Record<string, unknown> {
  switch (type) {
    case "circle":
      return {
        "circle-radius": 5,
        "circle-color": "#2563eb",
        "circle-stroke-color": "#ffffff",
        "circle-stroke-width": 1.5,
      };
    case "line":
      return { "line-color": "#2563eb", "line-width": 2 };
    default:
      return {
        "fill-color": "#60a5fa",
        "fill-opacity": 0.5,
        "fill-outline-color": "#1d4ed8",
      };
  }
}

function setupDrawLayers(map: MLMap) {
  if (!map.getSource(DRAW_SOURCE)) {
    map.addSource(DRAW_SOURCE, {
      type: "geojson",
      data: { type: "FeatureCollection", features: [] },
    });
  }
  if (!map.getLayer("draw-fill")) {
    map.addLayer({
      id: "draw-fill",
      type: "fill",
      source: DRAW_SOURCE,
      filter: ["==", ["geometry-type"], "Polygon"],
      paint: { "fill-color": "#22c55e", "fill-opacity": 0.25 },
    });
  }
  if (!map.getLayer("draw-line")) {
    map.addLayer({
      id: "draw-line",
      type: "line",
      source: DRAW_SOURCE,
      filter: ["in", ["geometry-type"], ["literal", ["LineString", "Polygon"]]],
      paint: { "line-color": "#16a34a", "line-width": 2, "line-dasharray": [2, 1] },
    });
  }
  if (!map.getLayer("draw-vertex")) {
    map.addLayer({
      id: "draw-vertex",
      type: "circle",
      source: DRAW_SOURCE,
      filter: ["==", ["geometry-type"], "Point"],
      paint: {
        "circle-radius": 5,
        "circle-color": "#ffffff",
        "circle-stroke-color": "#16a34a",
        "circle-stroke-width": 2,
      },
    });
  }
}

function updateDrawPreview(
  map: MLMap,
  mode: DrawMode,
  coords: [number, number][],
  hover: [number, number] | null
) {
  const src = map.getSource(DRAW_SOURCE) as GeoJSONSource | undefined;
  if (!src) return;
  const pts = [...coords];
  if (hover && (mode === "line" || mode === "polygon") && pts.length > 0) {
    pts.push(hover);
  }
  const features: any[] = pts.map((c) => ({
    type: "Feature",
    geometry: { type: "Point", coordinates: c },
    properties: {},
  }));
  if (mode === "line" && pts.length >= 2) {
    features.push({
      type: "Feature",
      geometry: { type: "LineString", coordinates: pts },
      properties: {},
    });
  }
  if (mode === "polygon" && pts.length >= 3) {
    features.push({
      type: "Feature",
      geometry: { type: "Polygon", coordinates: [[...pts, pts[0]]] },
      properties: {},
    });
  }
  src.setData({ type: "FeatureCollection", features });
}

export function MapView({
  layers,
  visible,
  basemapColor,
  flyToBounds,
  drawMode = "none",
  finishToken = 0,
  onSelectFeature,
  onDrawComplete,
  onDrawStateChange,
}: MapViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MLMap | null>(null);

  const drawModeRef = useRef<DrawMode>(drawMode);
  const coordsRef = useRef<[number, number][]>([]);
  const hoverRef = useRef<[number, number] | null>(null);

  const selectRef = useRef(onSelectFeature);
  selectRef.current = onSelectFeature;
  const completeRef = useRef(onDrawComplete);
  completeRef.current = onDrawComplete;
  const stateRef = useRef(onDrawStateChange);
  stateRef.current = onDrawStateChange;
  const finishRef = useRef<(() => void) | null>(null);
  const emitRef = useRef<(() => void) | null>(null);

  // 初始化地图
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: EMPTY_STYLE,
      center: [10, 25],
      zoom: 1.6,
      attributionControl: { compact: true },
    });
    map.addControl(new maplibregl.NavigationControl({ visualizePitch: true }), "top-right");
    map.addControl(new maplibregl.ScaleControl({ maxWidth: 120 }), "bottom-right");
    map.addControl(new maplibregl.FullscreenControl(), "top-right");

    const emit = () =>
      stateRef.current?.(drawModeRef.current !== "none", coordsRef.current.length);

    const finishDraw = (geometry: GeoJSONGeometry) => {
      coordsRef.current = [];
      hoverRef.current = null;
      updateDrawPreview(map, "none", [], null);
      completeRef.current?.(geometry);
      emit();
    };

    // 尝试结束线/面绘制（供双击或外部按钮调用）。
    const tryFinish = () => {
      const mode = drawModeRef.current;
      if (mode !== "line" && mode !== "polygon") return;
      const c = coordsRef.current;
      if (c.length > 1) {
        const last = c[c.length - 1];
        const prev = c[c.length - 2];
        if (last[0] === prev[0] && last[1] === prev[1]) c.pop();
      }
      if (mode === "line" && c.length >= 2) {
        finishDraw({ type: "LineString", coordinates: [...c] });
      } else if (mode === "polygon" && c.length >= 3) {
        finishDraw({ type: "Polygon", coordinates: [[...c, c[0]]] });
      }
    };
    finishRef.current = tryFinish;
    emitRef.current = emit;

    map.on("load", () => setupDrawLayers(map));

    map.on("click", (e) => {
      const mode = drawModeRef.current;
      const lngLat: [number, number] = [e.lngLat.lng, e.lngLat.lat];
      if (mode === "point") {
        finishDraw({ type: "Point", coordinates: lngLat });
        return;
      }
      if (mode === "line" || mode === "polygon") {
        coordsRef.current.push(lngLat);
        updateDrawPreview(map, mode, coordsRef.current, null);
        emit();
        return;
      }
      // 非绘制状态：查询要素
      const featureLayers = (map.getStyle().layers ?? [])
        .map((l) => l.id)
        .filter((id) => id.startsWith("lyr-"));
      const features = map.queryRenderedFeatures(e.point, { layers: featureLayers });
      if (!features.length) return;
      const f = features[0];
      const layerName = String(f.layer.id).replace(/^lyr-/, "").replace(/-outline$/, "");
      const props = (f.properties ?? {}) as Record<string, unknown>;
      selectRef.current?.({
        layer: layerName,
        id: f.id as number | undefined,
        properties: props,
      });
      const rows = Object.entries(props)
        .filter(([k]) => k !== "fid")
        .map(
          ([k, v]) =>
            `<div class="flex gap-2"><span class="font-medium">${escapeHtml(
              k
            )}</span><span class="text-muted-foreground ml-auto">${escapeHtml(
              String(v ?? "")
            )}</span></div>`
        )
        .join("");
      new maplibregl.Popup({ closeButton: true, maxWidth: "320px" })
        .setLngLat(e.lngLat)
        .setHTML(
          `<div class="space-y-1"><div class="text-xs uppercase tracking-wide text-muted-foreground">${escapeHtml(
            layerName
          )}</div>${rows || "<div>无属性</div>"}</div>`
        )
        .addTo(map);
    });

    map.on("dblclick", (e) => {
      const mode = drawModeRef.current;
      if (mode !== "line" && mode !== "polygon") return;
      e.preventDefault();
      tryFinish();
    });

    map.on("contextmenu", (e) => {
      if (drawModeRef.current === "none") return;
      e.preventDefault();
      coordsRef.current = [];
      hoverRef.current = null;
      updateDrawPreview(map, drawModeRef.current, [], null);
      emit();
    });

    map.on("mousemove", (e) => {
      const mode = drawModeRef.current;
      if ((mode === "line" || mode === "polygon") && coordsRef.current.length > 0) {
        hoverRef.current = [e.lngLat.lng, e.lngLat.lat];
        updateDrawPreview(map, mode, coordsRef.current, hoverRef.current);
        map.getCanvas().style.cursor = "crosshair";
        return;
      }
      if (mode !== "none") {
        map.getCanvas().style.cursor = "crosshair";
        return;
      }
      const ids = (map.getStyle().layers ?? [])
        .map((l) => l.id)
        .filter((id) => id.startsWith("lyr-"));
      const hit =
        ids.length > 0 && map.queryRenderedFeatures(e.point, { layers: ids }).length > 0;
      map.getCanvas().style.cursor = hit ? "pointer" : "";
    });

    mapRef.current = map;
    return () => {
      map.remove();
      mapRef.current = null;
    };
  }, []);

  // 切换绘制模式
  useEffect(() => {
    drawModeRef.current = drawMode;
    coordsRef.current = [];
    hoverRef.current = null;
    const map = mapRef.current;
    if (!map) return;
    if (drawMode === "none") {
      map.doubleClickZoom.enable();
      map.getCanvas().style.cursor = "";
    } else {
      map.doubleClickZoom.disable();
      map.getCanvas().style.cursor = "crosshair";
    }
    if (map.isStyleLoaded()) updateDrawPreview(map, drawMode, [], null);
    emitRef.current?.();
  }, [drawMode]);

  // 外部触发完成绘制
  useEffect(() => {
    if (finishToken > 0) finishRef.current?.();
  }, [finishToken]);

  // 同步图层
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const sync = () => {
      const desired = new Set<string>();
      for (const l of layers) {
        const srcId = `src-${l.name}`;
        const layId = `lyr-${l.name}`;
        const vis = visible[l.name] !== false;
        desired.add(layId);
        if (!map.getSource(srcId)) {
          map.addSource(srcId, {
            type: "vector",
            tiles: [tileUrlTemplate(l.name)],
            minzoom: l.minZoom,
            maxzoom: l.maxZoom,
          });
        }
        const spec = buildLayerSpec(l, vis);
        if (!map.getLayer(layId)) {
          map.addLayer(spec as any);
          if (spec.type === "fill") {
            desired.add(`${layId}-outline`);
            map.addLayer({
              id: `${layId}-outline`,
              type: "line",
              source: srcId,
              "source-layer": l.name,
              minzoom: l.minZoom,
              maxzoom: l.maxZoom,
              layout: { visibility: vis ? "visible" : "none" },
              paint: {
                "line-color":
                  (l.style as any)?.paint?.["fill-outline-color"] ?? "#1d4ed8",
                "line-width": 0.8,
              },
            } as any);
          }
        } else {
          map.setLayoutProperty(layId, "visibility", vis ? "visible" : "none");
          for (const [k, v] of Object.entries(spec.paint ?? {})) {
            try {
              map.setPaintProperty(layId, k, v as any);
            } catch {
              /* 忽略不支持的属性 */
            }
          }
          if (map.getLayer(`${layId}-outline`)) {
            map.setLayoutProperty(
              `${layId}-outline`,
              "visibility",
              vis ? "visible" : "none"
            );
          }
        }
      }
      for (const styleLayer of map.getStyle().layers ?? []) {
        const id = styleLayer.id;
        if (id.startsWith("lyr-") && !desired.has(id)) {
          map.removeLayer(id);
        }
      }
      const styleSources = map.getStyle().sources ?? {};
      for (const srcId of Object.keys(styleSources)) {
        if (srcId.startsWith("src-") && !desired.has(`lyr-${srcId.slice(4)}`)) {
          map.removeSource(srcId);
        }
      }
    };
    if (map.isStyleLoaded()) sync();
    else map.once("load", sync);
  }, [layers, visible]);

  // 底图颜色
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const apply = () => {
      if (map.getLayer("background")) {
        map.setPaintProperty("background", "background-color", basemapColor);
      }
    };
    if (map.isStyleLoaded()) apply();
    else map.once("load", apply);
  }, [basemapColor]);

  // 飞行到范围
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !flyToBounds) return;
    const [minx, miny, maxx, maxy] = flyToBounds;
    map.fitBounds(
      [
        [minx, miny],
        [maxx, maxy],
      ],
      { padding: 60, duration: 800, maxZoom: 12 }
    );
  }, [flyToBounds]);

  return <div ref={containerRef} className="h-full w-full" />;
}

function escapeHtml(v: string): string {
  return v
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

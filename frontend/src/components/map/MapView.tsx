import { useEffect, useRef } from "react";
import maplibregl from "maplibre-gl";
import type {
  GeoJSONSource,
  Map as MLMap,
  StyleSpecification,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";

import { basemapById } from "@/lib/basemaps";
import { tileUrlTemplate } from "@/lib/api";
import type {
  BasemapId,
  GeoJSONGeometry,
  Layer,
  LayerViewState,
} from "@/types";

export type DrawMode = "none" | "point" | "line" | "polygon";
export type EditMode = "none" | "move" | "vertex";

export interface SelectedFeature {
  layer: string;
  id?: number | string;
  properties: Record<string, unknown>;
}

export interface MapViewState {
  center: [number, number];
  zoom: number;
  bearing: number;
  pitch: number;
  bounds: [number, number, number, number];
}

export interface MapViewProps {
  layers: Layer[];
  view: Record<string, LayerViewState>;
  basemap: BasemapId;
  backgroundColor?: string;
  initialView?: { center: [number, number]; zoom: number; bearing?: number; pitch?: number };
  flyToBounds?: [number, number, number, number] | null;
  drawMode?: DrawMode;
  finishToken?: number;
  boxSelect?: boolean;
  editMode?: EditMode;
  snapping?: boolean;
  highlight?: { layer: string; fid: number | string; geometry: GeoJSONGeometry } | null;
  onSelectFeature?: (feature: SelectedFeature) => void;
  onDrawComplete?: (geometry: GeoJSONGeometry) => void;
  onDrawStateChange?: (drawing: boolean, vertices: number) => void;
  onBoxSelect?: (bbox: [number, number, number, number]) => void;
  onViewChange?: (state: MapViewState) => void;
  onGeometryChange?: (
    layer: string,
    fid: number | string,
    geometry: GeoJSONGeometry
  ) => void;
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
const BOX_SOURCE = "box-src";
const HL_SOURCE = "hl-src";
const VERTEX_SOURCE = "vertex-src";
const TRANSIENT_LAYERS = ["draw-fill", "draw-line", "draw-vertex", "box-fill", "box-line", "hl-fill", "hl-line", "hl-circle", "vertex-hit"];

type Coord = [number, number];

function defaultType(geometryType: string): "fill" | "line" | "circle" {
  const g = geometryType.toUpperCase();
  if (g.includes("POINT")) return "circle";
  if (g.includes("LINE")) return "line";
  return "fill";
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
    case "heatmap":
      return {
        "heatmap-radius": 20,
        "heatmap-opacity": 0.7,
        "heatmap-color": [
          "interpolate",
          ["linear"],
          ["heatmap-density"],
          0, "rgba(33,102,172,0)",
          0.2, "rgb(103,169,207)",
          0.4, "rgb(209,229,240)",
          0.6, "rgb(253,219,199)",
          0.8, "rgb(239,138,98)",
          1, "rgb(178,24,43)",
        ],
      };
    default:
      return {
        "fill-color": "#60a5fa",
        "fill-opacity": 0.5,
        "fill-outline-color": "#1d4ed8",
      };
  }
}

function withOpacity(paint: Record<string, unknown>, type: string, opacity: number): Record<string, unknown> {
  const out = { ...paint };
  if (opacity >= 1) return out;
  const scale = (key: string) => {
    const v = out[key];
    out[key] = typeof v === "number" ? v * opacity : opacity;
  };
  switch (type) {
    case "circle":
      scale("circle-opacity");
      scale("circle-stroke-opacity");
      break;
    case "line":
      scale("line-opacity");
      break;
    case "fill":
      scale("fill-opacity");
      break;
    case "heatmap":
      scale("heatmap-opacity");
      break;
    default:
      break;
  }
  return out;
}

function labelFieldOf(l: Layer): string {
  const v = (l.metadata as Record<string, unknown> | undefined)?.labelField;
  return typeof v === "string" ? v : "";
}

function popupFieldsOf(l: Layer): string[] {
  const v = (l.metadata as Record<string, unknown> | undefined)?.popupFields;
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

function buildLayerSpec(l: Layer, state: LayerViewState): Record<string, any> {
  const style = (state.styleOverride ?? l.style ?? {}) as Record<string, any>;
  const type: string = style.type ?? defaultType(l.geometryType);
  const basePaint = (style.paint ?? defaultPaint(type)) as Record<string, unknown>;
  return {
    id: `lyr-${l.name}`,
    type,
    source: `src-${l.name}`,
    "source-layer": l.name,
    minzoom: l.minZoom,
    maxzoom: l.maxZoom,
    layout: { ...(style.layout ?? {}), visibility: state.visible ? "visible" : "none" },
    paint: withOpacity(basePaint, type, state.opacity ?? 1),
  };
}

function setupTransientLayers(map: MLMap) {
  if (!map.getSource(DRAW_SOURCE)) {
    map.addSource(DRAW_SOURCE, {
      type: "geojson",
      data: { type: "FeatureCollection", features: [] },
    });
  }
  if (!map.getSource(BOX_SOURCE)) {
    map.addSource(BOX_SOURCE, {
      type: "geojson",
      data: { type: "FeatureCollection", features: [] },
    });
  }
  if (!map.getSource(HL_SOURCE)) {
    map.addSource(HL_SOURCE, {
      type: "geojson",
      data: { type: "FeatureCollection", features: [] },
    });
  }
  if (!map.getSource(VERTEX_SOURCE)) {
    map.addSource(VERTEX_SOURCE, {
      type: "geojson",
      data: { type: "FeatureCollection", features: [] },
    });
  }
  if (!map.getLayer("box-fill")) {
    map.addLayer({
      id: "box-fill",
      type: "fill",
      source: BOX_SOURCE,
      paint: { "fill-color": "#f59e0b", "fill-opacity": 0.15 },
    });
  }
  if (!map.getLayer("box-line")) {
    map.addLayer({
      id: "box-line",
      type: "line",
      source: BOX_SOURCE,
      paint: { "line-color": "#f59e0b", "line-width": 2, "line-dasharray": [2, 1] },
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
  if (!map.getLayer("hl-fill")) {
    map.addLayer({
      id: "hl-fill",
      type: "fill",
      source: HL_SOURCE,
      filter: ["==", ["geometry-type"], "Polygon"],
      paint: { "fill-color": "#f59e0b", "fill-opacity": 0.35 },
    });
  }
  if (!map.getLayer("hl-line")) {
    map.addLayer({
      id: "hl-line",
      type: "line",
      source: HL_SOURCE,
      filter: ["in", ["geometry-type"], ["literal", ["LineString", "Polygon"]]],
      paint: { "line-color": "#d97706", "line-width": 3 },
    });
  }
  if (!map.getLayer("hl-circle")) {
    map.addLayer({
      id: "hl-circle",
      type: "circle",
      source: HL_SOURCE,
      filter: ["==", ["geometry-type"], "Point"],
      paint: {
        "circle-radius": 9,
        "circle-color": "#f59e0b",
        "circle-opacity": 0.5,
        "circle-stroke-color": "#b45309",
        "circle-stroke-width": 2,
      },
    });
  }
  if (!map.getLayer("vertex-hit")) {
    map.addLayer({
      id: "vertex-hit",
      type: "circle",
      source: VERTEX_SOURCE,
      paint: {
        "circle-radius": 7,
        "circle-color": "#ffffff",
        "circle-stroke-color": "#d97706",
        "circle-stroke-width": 2,
      },
    });
  }
}

function updateDrawPreview(
  map: MLMap,
  mode: DrawMode,
  coords: Coord[],
  hover: Coord | null
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

function emptyFC(): any {
  return { type: "FeatureCollection", features: [] };
}

function featureOf(g: GeoJSONGeometry | null): any {
  if (!g) return emptyFC();
  return {
    type: "FeatureCollection",
    features: [{ type: "Feature", geometry: g, properties: {} }],
  };
}

function firstTransientLayer(map: MLMap): string | undefined {
  for (const l of map.getStyle().layers ?? []) {
    if (TRANSIENT_LAYERS.includes(l.id)) return l.id;
  }
  return undefined;
}

function applyBasemap(map: MLMap, basemap: BasemapId, color: string) {
  if (map.getLayer("basemap")) map.removeLayer("basemap");
  if (map.getSource("basemap")) map.removeSource("basemap");
  if (map.getLayer("background")) {
    map.setPaintProperty("background", "background-color", color);
  }
  const def = basemapById(basemap);
  if (!def.tiles?.length) return;
  map.addSource("basemap", {
    type: "raster",
    tiles: def.tiles,
    tileSize: 256,
    maxzoom: def.maxzoom ?? 19,
    attribution: def.attribution,
  });
  const before = (map.getStyle().layers ?? []).find(
    (l) => l.id !== "background" && l.id !== "basemap"
  )?.id;
  map.addLayer(
    {
      id: "basemap",
      type: "raster",
      source: "basemap",
      paint: { "raster-opacity": 1 },
    },
    before
  );
}

function syncLayers(map: MLMap, layers: Layer[], view: Record<string, LayerViewState>) {
  const desired = new Set<string>();
  for (const l of layers) {
    const state: LayerViewState = view[l.name] ?? { visible: true, opacity: 1 };
    const srcId = `src-${l.name}`;
    const layId = `lyr-${l.name}`;
    const labelId = `${layId}-label`;
    const outlineId = `${layId}-outline`;
    const vis = state.visible !== false;
    desired.add(layId);
    if (!map.getSource(srcId)) {
      map.addSource(srcId, {
        type: "vector",
        tiles: [tileUrlTemplate(l.name)],
        minzoom: l.minZoom,
        maxzoom: l.maxZoom,
      });
    }
    const spec = buildLayerSpec(l, state);
    const existing = map.getLayer(layId);
    if (existing && existing.type !== spec.type) {
      if (map.getLayer(outlineId)) map.removeLayer(outlineId);
      if (map.getLayer(labelId)) map.removeLayer(labelId);
      map.removeLayer(layId);
    }
    if (!map.getLayer(layId)) {
      map.addLayer(spec as any);
      if (spec.type === "fill") {
        desired.add(outlineId);
        map.addLayer({
          id: outlineId,
          type: "line",
          source: srcId,
          "source-layer": l.name,
          minzoom: l.minZoom,
          maxzoom: l.maxZoom,
          layout: { visibility: vis ? "visible" : "none" },
          paint: {
            "line-color": (l.style as any)?.paint?.["fill-outline-color"] ?? "#1d4ed8",
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
      if (map.getLayer(outlineId)) {
        map.setLayoutProperty(outlineId, "visibility", vis ? "visible" : "none");
      }
    }
    const labelField = labelFieldOf(l);
    if (labelField) {
      desired.add(labelId);
      const meta = (l.metadata ?? {}) as Record<string, any>;
      const layout = {
        visibility: vis ? "visible" : "none",
        "text-field": ["to-string", ["get", labelField]],
        "text-size": Number(meta.labelSize) || 12,
        "text-offset": [0, 0.6],
        "text-anchor": "top",
        "text-allow-overlap": false,
      };
      const paint = {
        "text-color": String(meta.labelColor ?? "#111827"),
        "text-halo-color": "#ffffff",
        "text-halo-width": 1.2,
      };
      if (!map.getLayer(labelId)) {
        map.addLayer({
          id: labelId,
          type: "symbol",
          source: srcId,
          "source-layer": l.name,
          minzoom: l.minZoom,
          maxzoom: l.maxZoom,
          layout,
          paint,
        } as any);
      } else {
        map.setLayoutProperty(labelId, "visibility", vis ? "visible" : "none");
        try {
          map.setLayoutProperty(labelId, "text-field", layout["text-field"]);
          map.setLayoutProperty(labelId, "text-size", layout["text-size"]);
          map.setPaintProperty(labelId, "text-color", paint["text-color"]);
        } catch {
          /* ignore */
        }
      }
    }
  }
  for (const styleLayer of map.getStyle().layers ?? []) {
    const id = styleLayer.id;
    if (id.startsWith("lyr-") && !desired.has(id)) {
      map.removeLayer(id);
    }
  }
  for (const srcId of Object.keys(map.getStyle().sources ?? {})) {
    if (srcId.startsWith("src-") && !desired.has(`lyr-${srcId.slice(4)}`)) {
      map.removeSource(srcId);
    }
  }
  // 保证图层顺序与列表一致（数组越靠后越在上层）。
  const before = firstTransientLayer(map);
  for (const l of layers) {
    const layId = `lyr-${l.name}`;
    if (map.getLayer(layId)) map.moveLayer(layId, before);
    const outlineId = `${layId}-outline`;
    if (map.getLayer(outlineId)) map.moveLayer(outlineId, before);
    const labelId = `${layId}-label`;
    if (map.getLayer(labelId)) map.moveLayer(labelId, before);
  }
}

function mapGeometryCoords(g: GeoJSONGeometry, fn: (c: Coord) => Coord): GeoJSONGeometry {
  const walk = (v: unknown): unknown => {
    if (!Array.isArray(v)) return v;
    if (typeof v[0] === "number" && typeof v[1] === "number") {
      return fn([v[0], v[1]]);
    }
    return v.map(walk);
  };
  return { ...g, coordinates: walk(g.coordinates) } as GeoJSONGeometry;
}

function translateGeometry(g: GeoJSONGeometry, dx: number, dy: number): GeoJSONGeometry {
  return mapGeometryCoords(g, ([x, y]) => [x + dx, y + dy]);
}

interface Vertex {
  coord: Coord;
  path: (string | number)[];
}

function verticesOf(g: GeoJSONGeometry): Vertex[] {
  const out: Vertex[] = [];
  if (g.type === "Point" && Array.isArray(g.coordinates)) {
    out.push({ coord: g.coordinates as Coord, path: ["coordinates"] });
  } else if (g.type === "LineString" && Array.isArray(g.coordinates)) {
    (g.coordinates as Coord[]).forEach((c, i) => out.push({ coord: c, path: ["coordinates", i] }));
  } else if (g.type === "Polygon" && Array.isArray(g.coordinates)) {
    (g.coordinates as Coord[][]).forEach((ring, ri) => {
      ring.forEach((c, pi) => {
        if (pi === ring.length - 1 && ring.length > 1) return;
        out.push({ coord: c, path: ["coordinates", ri, pi] });
      });
    });
  }
  return out;
}

function setVertex(g: GeoJSONGeometry, path: (string | number)[], coord: Coord): GeoJSONGeometry {
  const clone = JSON.parse(JSON.stringify(g)) as any;
  let obj: any = clone;
  for (let i = 0; i < path.length - 1; i++) obj = obj[path[i]];
  const last = path[path.length - 1];
  obj[last] = coord;
  // 闭合环：拖动首/尾节点时同步另一端。
  if (g.type === "Polygon" && path[0] === "coordinates") {
    const ri = Number(path[1]);
    const ring = clone.coordinates[ri] as Coord[];
    if (ring.length > 1) {
      if (Number(last) === 0) ring[ring.length - 1] = coord;
      if (Number(last) === ring.length - 1) ring[0] = coord;
    }
  }
  return clone as GeoJSONGeometry;
}

export function MapView({
  layers,
  view,
  basemap,
  backgroundColor = "#f8fafc",
  initialView,
  flyToBounds,
  drawMode = "none",
  finishToken = 0,
  boxSelect = false,
  editMode = "none",
  snapping = true,
  highlight = null,
  onSelectFeature,
  onDrawComplete,
  onDrawStateChange,
  onBoxSelect,
  onViewChange,
  onGeometryChange,
}: MapViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MLMap | null>(null);

  const drawModeRef = useRef<DrawMode>(drawMode);
  const boxSelectRef = useRef(boxSelect);
  const editModeRef = useRef<EditMode>(editMode);
  const snappingRef = useRef(snapping);
  const highlightRef = useRef(highlight);
  const layersRef = useRef(layers);
  const viewRef = useRef(view);
  const coordsRef = useRef<Coord[]>([]);
  const hoverRef = useRef<Coord | null>(null);
  const dragRef = useRef<
    | { kind: "box"; start: Coord }
    | { kind: "move"; start: Coord; origin: GeoJSONGeometry }
    | { kind: "vertex"; path: (string | number)[]; origin: GeoJSONGeometry }
    | null
  >(null);
  const workingGeomRef = useRef<GeoJSONGeometry | null>(null);

  const selectRef = useRef(onSelectFeature);
  selectRef.current = onSelectFeature;
  const completeRef = useRef(onDrawComplete);
  completeRef.current = onDrawComplete;
  const stateRef = useRef(onDrawStateChange);
  stateRef.current = onDrawStateChange;
  const boxRef = useRef(onBoxSelect);
  boxRef.current = onBoxSelect;
  const viewChangeRef = useRef(onViewChange);
  viewChangeRef.current = onViewChange;
  const geomChangeRef = useRef(onGeometryChange);
  geomChangeRef.current = onGeometryChange;
  const finishRef = useRef<(() => void) | null>(null);
  const emitRef = useRef<(() => void) | null>(null);

  highlightRef.current = highlight;
  layersRef.current = layers;
  viewRef.current = view;
  boxSelectRef.current = boxSelect;
  editModeRef.current = editMode;
  snappingRef.current = snapping;

  // 初始化地图
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: EMPTY_STYLE,
      center: initialView?.center ?? [10, 25],
      zoom: initialView?.zoom ?? 1.6,
      bearing: initialView?.bearing ?? 0,
      pitch: initialView?.pitch ?? 0,
      attributionControl: { compact: true },
    });
    map.addControl(new maplibregl.NavigationControl({ visualizePitch: true }), "top-right");
    map.addControl(new maplibregl.ScaleControl({ maxWidth: 120 }), "bottom-right");
    map.addControl(new maplibregl.FullscreenControl(), "top-right");

    const emit = () =>
      stateRef.current?.(drawModeRef.current !== "none" || boxSelectRef.current, coordsRef.current.length);

    const finishDraw = (geometry: GeoJSONGeometry) => {
      coordsRef.current = [];
      hoverRef.current = null;
      updateDrawPreview(map, "none", [], null);
      completeRef.current?.(geometry);
      emit();
    };

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

    const reportView = () => {
      const c = map.getCenter();
      const b = map.getBounds();
      viewChangeRef.current?.({
        center: [c.lng, c.lat],
        zoom: map.getZoom(),
        bearing: map.getBearing(),
        pitch: map.getPitch(),
        bounds: [b.getWest(), b.getSouth(), b.getEast(), b.getNorth()],
      });
    };

    map.on("load", () => {
      setupTransientLayers(map);
      syncLayers(map, layersRef.current, viewRef.current);
      applyBasemap(map, basemap, backgroundColor);
      reportView();
    });

    const setBox = (a: Coord, b: Coord) => {
      const src = map.getSource(BOX_SOURCE) as GeoJSONSource | undefined;
      if (!src) return;
      const [w, e] = [Math.min(a[0], b[0]), Math.max(a[0], b[0])];
      const [s, n] = [Math.min(a[1], b[1]), Math.max(a[1], b[1])];
      src.setData(
        featureOf({ type: "Polygon", coordinates: [[[w, s], [e, s], [e, n], [w, n], [w, s]]] })
      );
    };

    const startDrag = (e: maplibregl.MapMouseEvent) => {
      const start: Coord = [e.lngLat.lng, e.lngLat.lat];
      const mode = editModeRef.current;
      const sel = highlightRef.current;
      if (boxSelectRef.current) {
        dragRef.current = { kind: "box", start };
        setBox(start, start);
        map.dragPan.disable();
        return;
      }
      if (mode === "none" || !sel) return;

      if (mode === "vertex") {
        const hits = map.queryRenderedFeatures(e.point, { layers: ["vertex-hit"] });
        if (hits.length) {
          const props = hits[0].properties as Record<string, unknown>;
          const path = JSON.parse(String(props.path ?? "[]")) as (string | number)[];
          dragRef.current = { kind: "vertex", path, origin: sel.geometry };
          workingGeomRef.current = sel.geometry;
          map.dragPan.disable();
          e.preventDefault();
          return;
        }
      }

      if (mode === "move") {
        const hits = map.queryRenderedFeatures(e.point, {
          layers: ["hl-fill", "hl-line", "hl-circle"],
        });
        if (hits.length) {
          dragRef.current = { kind: "move", start, origin: sel.geometry };
          workingGeomRef.current = sel.geometry;
          map.dragPan.disable();
          e.preventDefault();
        }
      }
    };

    const dragMove = (e: maplibregl.MapMouseEvent) => {
      const drag = dragRef.current;
      if (!drag) return;
      if (drag.kind === "box") {
        setBox(drag.start, [e.lngLat.lng, e.lngLat.lat]);
        return;
      }
      if (drag.kind === "move") {
        const g = translateGeometry(
          drag.origin,
          e.lngLat.lng - drag.start[0],
          e.lngLat.lat - drag.start[1]
        );
        workingGeomRef.current = g;
        (map.getSource(HL_SOURCE) as GeoJSONSource | undefined)?.setData(featureOf(g));
        return;
      }
      if (drag.kind === "vertex") {
        let coord: Coord = [e.lngLat.lng, e.lngLat.lat];
        if (snappingRef.current) {
          const p = e.point;
          let best: { dist: number; coord: Coord } | null = null;
          const verts =
            ((map as any).__vertices as Array<{ path: (string | number)[]; coord: Coord }> | undefined) ?? [];
          for (const v of verts) {
            if (JSON.stringify(v.path) === JSON.stringify(drag.path)) continue;
            const q = map.project(v.coord);
            const d = Math.hypot(q.x - p.x, q.y - p.y);
            if (d < 10 && (!best || d < best.dist)) best = { dist: d, coord: v.coord };
          }
          if (best) coord = best.coord;
        }
        const g = setVertex(drag.origin, drag.path, coord);
        workingGeomRef.current = g;
        (map.getSource(HL_SOURCE) as GeoJSONSource | undefined)?.setData(featureOf(g));
        renderVertices(map, g, drag.path);
      }
    };

    const endDrag = (e: maplibregl.MapMouseEvent) => {
      const drag = dragRef.current;
      dragRef.current = null;
      map.dragPan.enable();
      if (!drag) return;
      if (drag.kind === "box") {
        (map.getSource(BOX_SOURCE) as GeoJSONSource | undefined)?.setData(emptyFC());
        const a = drag.start;
        const b: Coord = [e.lngLat.lng, e.lngLat.lat];
        const bbox: [number, number, number, number] = [
          Math.min(a[0], b[0]),
          Math.min(a[1], b[1]),
          Math.max(a[0], b[0]),
          Math.max(a[1], b[1]),
        ];
        if (bbox[2] - bbox[0] > 1e-9 && bbox[3] - bbox[1] > 1e-9) {
          boxRef.current?.(bbox);
        }
        return;
      }
      const g = workingGeomRef.current;
      const sel = highlightRef.current;
      if (g && sel) {
        geomChangeRef.current?.(sel.layer, sel.fid, g);
      }
      workingGeomRef.current = null;
    };

    map.on("mousedown", startDrag);
    map.on("mousemove", dragMove);
    map.on("mouseup", endDrag);

    map.on("click", (e) => {
      if (dragRef.current) return;
      const mode = drawModeRef.current;
      const lngLat: Coord = [e.lngLat.lng, e.lngLat.lat];
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
      if (boxSelectRef.current || editModeRef.current !== "none") return;
      const featureLayers = (map.getStyle().layers ?? [])
        .map((l) => l.id)
        .filter((id) => id.startsWith("lyr-") && !id.endsWith("-label") && !id.endsWith("-outline"));
      const features = map.queryRenderedFeatures(e.point, { layers: featureLayers });
      if (!features.length) return;
      const f = features[0];
      const layerName = String(f.layer.id).replace(/^lyr-/, "");
      const l = layersRef.current.find((x) => x.name === layerName);
      if (!l) return;
      const props = (f.properties ?? {}) as Record<string, unknown>;
      selectRef.current?.({
        layer: layerName,
        id: f.id as number | undefined,
        properties: props,
      });
      const fields = popupFieldsOf(l);
      const entries = Object.entries(props).filter(([k]) => k !== "fid");
      const shown = fields.length ? entries.filter(([k]) => fields.includes(k)) : entries;
      const titleValue = labelFieldOf(l) ? String(props[labelFieldOf(l)] ?? "") : "";
      const rows = shown
        .map(
          ([k, v]) =>
            `<div style="display:flex;gap:8px"><span style="font-weight:600">${escapeHtml(
              k
            )}</span><span style="color:#64748b;margin-left:auto">${escapeHtml(
              String(v ?? "")
            )}</span></div>`
        )
        .join("");
      new maplibregl.Popup({ closeButton: true, maxWidth: "320px" })
        .setLngLat(e.lngLat)
        .setHTML(
          `<div style="display:flex;flex-direction:column;gap:4px;font-size:12px">` +
            `<div style="text-transform:uppercase;letter-spacing:.05em;color:#94a3b8">${escapeHtml(
              l.title || layerName
            )}${titleValue ? " · " + escapeHtml(titleValue) : ""}</div>` +
            `${rows || "<div>无属性</div>"}</div>`
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
      if (mode !== "none" || boxSelectRef.current) {
        map.getCanvas().style.cursor = "crosshair";
        return;
      }
      if (editModeRef.current === "vertex") {
        const hitV = map.queryRenderedFeatures(e.point, { layers: ["vertex-hit"] });
        map.getCanvas().style.cursor = hitV.length ? "grab" : "";
        return;
      }
      if (editModeRef.current === "move") {
        const hitH = map.queryRenderedFeatures(e.point, { layers: ["hl-fill", "hl-line", "hl-circle"] });
        map.getCanvas().style.cursor = hitH.length ? "move" : "";
        return;
      }
      const ids = (map.getStyle().layers ?? [])
        .map((l) => l.id)
        .filter((id) => id.startsWith("lyr-") && !id.endsWith("-label") && !id.endsWith("-outline"));
      const hit = ids.length > 0 && map.queryRenderedFeatures(e.point, { layers: ids }).length > 0;
      map.getCanvas().style.cursor = hit ? "pointer" : "";
    });

    map.on("moveend", reportView);

    mapRef.current = map;
    return () => {
      map.remove();
      mapRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 切换绘制 / 框选模式
  useEffect(() => {
    drawModeRef.current = drawMode;
    coordsRef.current = [];
    hoverRef.current = null;
    const map = mapRef.current;
    if (!map) return;
    if (drawMode === "none") {
      map.doubleClickZoom.enable();
      map.getCanvas().style.cursor = boxSelect ? "crosshair" : "";
    } else {
      map.doubleClickZoom.disable();
      map.getCanvas().style.cursor = "crosshair";
    }
    if (map.isStyleLoaded()) updateDrawPreview(map, drawMode, [], null);
    emitRef.current?.();
  }, [drawMode, boxSelect]);

  // 外部触发完成绘制
  useEffect(() => {
    if (finishToken > 0) finishRef.current?.();
  }, [finishToken]);

  // 同步图层
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const sync = () => syncLayers(map, layers, view);
    if (map.isStyleLoaded()) sync();
    else map.once("load", sync);
  }, [layers, view]);

  // 底图 / 背景色
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const apply = () => applyBasemap(map, basemap, backgroundColor);
    if (map.isStyleLoaded()) apply();
    else map.once("load", apply);
  }, [basemap, backgroundColor]);

  // 初始视角（加载分享地图时）
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !initialView) return;
    map.jumpTo({
      center: initialView.center,
      zoom: initialView.zoom,
      bearing: initialView.bearing ?? 0,
      pitch: initialView.pitch ?? 0,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialView?.center?.[0], initialView?.center?.[1], initialView?.zoom]);

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
      { padding: 60, duration: 800, maxZoom: 14 }
    );
  }, [flyToBounds]);

  // 高亮要素 + 顶点
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const apply = () => {
      const src = map.getSource(HL_SOURCE) as GeoJSONSource | undefined;
      src?.setData(featureOf(highlight?.geometry ?? null));
      if (editMode !== "vertex" || !highlight) {
        (map.getSource(VERTEX_SOURCE) as GeoJSONSource | undefined)?.setData(emptyFC());
      } else {
        renderVertices(map, highlight.geometry);
      }
    };
    if (map.isStyleLoaded()) apply();
    else map.once("load", apply);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [highlight, editMode]);

  return <div ref={containerRef} className="h-full w-full" />;
}

function renderVertices(map: MLMap, g: GeoJSONGeometry, skipPath?: (string | number)[]) {
  const src = map.getSource(VERTEX_SOURCE) as GeoJSONSource | undefined;
  if (!src) return;
  const verts = verticesOf(g);
  (map as any).__vertices = verts;
  const features: any[] = verts
    .filter((v) => !skipPath || JSON.stringify(v.path) !== JSON.stringify(skipPath))
    .map((v) => ({
      type: "Feature",
      geometry: { type: "Point", coordinates: v.coord },
      properties: { path: JSON.stringify(v.path) },
    }));
  src.setData({ type: "FeatureCollection", features });
}

function escapeHtml(v: string): string {
  return v
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

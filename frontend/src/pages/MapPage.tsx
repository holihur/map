import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  ArrowDown,
  ArrowUp,
  Circle,
  Crosshair,
  Download,
  GripVertical,
  Layers as LayersIcon,
  Loader2,
  Magnet,
  Minus,
  MousePointer2,
  Move,
  Palette,
  Pencil,
  Pentagon,
  Printer,
  Redo2,
  RefreshCw,
  Search,
  Share2,
  SlidersHorizontal,
  Trash2,
  Undo2,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { MapView, type DrawMode, type EditMode, type MapViewState, type SelectedFeature } from "@/components/map/MapView";
import { StyleEditor } from "@/components/StyleEditor";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  createFeatures,
  createMap,
  deleteFeature,
  getFeature,
  layerBounds,
  listLayers,
  searchAll,
  updateFeature,
  wmsGetMapMultiUrl,
} from "@/lib/api";
import { BASEMAPS } from "@/lib/basemaps";
import { useAuth } from "@/lib/auth";
import {
  boundsOfGeometry,
  defaultWorkspace,
  loadLocalWorkspace,
  normalizeWorkspace,
  orderedLayers,
  saveLocalWorkspace,
} from "@/lib/workspace";
import type {
  BasemapId,
  GeoJSONFeature,
  GeoJSONGeometry,
  Layer,
  LayerField,
  MapWorkspace,
  SearchResponse,
} from "@/types";

/** 根据图层几何类型返回允许的绘制模式。 */
function allowedModes(layer: Layer | null): DrawMode[] {
  if (!layer) return [];
  const g = layer.geometryType.toUpperCase();
  if (g.includes("POINT")) return ["point"];
  if (g.includes("LINE")) return ["line"];
  if (g.includes("POLYGON")) return ["polygon"];
  return ["point", "line", "polygon"];
}

function minVertices(mode: DrawMode): number {
  if (mode === "polygon") return 3;
  if (mode === "line") return 2;
  return 1;
}

export function MapPage() {
  const { can } = useAuth();
  const editable = can("editor");
  const navigate = useNavigate();
  const [params] = useSearchParams();

  const savedRef = useRef<MapWorkspace | null>(loadLocalWorkspace());
  const [layers, setLayers] = useState<Layer[]>([]);
  const [ws, setWs] = useState<MapWorkspace>(() =>
    normalizeWorkspace(savedRef.current ?? defaultWorkspace([]), [])
  );
  const [initialView] = useState<
    { center: [number, number]; zoom: number; bearing?: number; pitch?: number } | undefined
  >(() =>
    savedRef.current
      ? {
          center: savedRef.current.center,
          zoom: savedRef.current.zoom,
          bearing: savedRef.current.bearing,
          pitch: savedRef.current.pitch,
        }
      : undefined
  );
  const [mapView, setMapView] = useState<MapViewState | null>(null);
  const [loading, setLoading] = useState(true);
  const [layerPanelOpen, setLayerPanelOpen] = useState(false);
  const [styleLayer, setStyleLayer] = useState<Layer | null>(null);
  const [shareOpen, setShareOpen] = useState(false);
  const [shareName, setShareName] = useState("我的地图");
  const [sharePublic, setSharePublic] = useState(true);
  const [sharing, setSharing] = useState(false);

  const [drawLayerName, setDrawLayerName] = useState<string>("");
  const [drawMode, setDrawMode] = useState<DrawMode>("none");
  const [finishToken, setFinishToken] = useState(0);
  const [drawing, setDrawing] = useState(false);
  const [vertices, setVertices] = useState(0);
  const [pendingGeometry, setPendingGeometry] = useState<GeoJSONGeometry | null>(null);
  const [props, setProps] = useState<Record<string, unknown>>({});
  const [boxSelect, setBoxSelect] = useState(false);
  const [editMode, setEditMode] = useState<EditMode>("none");
  const [snapping, setSnapping] = useState(true);
  const [history, setHistory] = useState<GeoJSONGeometry[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);

  const [selected, setSelected] = useState<SelectedFeature | null>(null);
  const [selectedFeature, setSelectedFeature] = useState<GeoJSONFeature | null>(null);
  const [highlight, setHighlight] = useState<
    { layer: string; fid: number | string; geometry: GeoJSONGeometry } | null
  >(null);
  const [editing, setEditing] = useState<{ layer: string; feature: GeoJSONFeature } | null>(null);
  const [editText, setEditText] = useState("");

  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [results, setResults] = useState<SearchResponse | null>(null);
  const focusRef = useRef(false);

  const drawLayer = useMemo(
    () => layers.find((l) => l.name === drawLayerName) ?? null,
    [layers, drawLayerName]
  );
  const ordered = useMemo(() => orderedLayers(layers, ws), [layers, ws.order]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const data = await listLayers();
      setLayers(data);
      setWs((prev) => normalizeWorkspace(prev, data));
      setDrawLayerName((prev) => prev || data[0]?.name || "");
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    saveLocalWorkspace(ws);
  }, [ws]);

  const updateViewState = (name: string, patch: Partial<{ visible: boolean; opacity: number }>) => {
    setWs((prev) => ({
      ...prev,
      layers: {
        ...prev.layers,
        [name]: { ...(prev.layers[name] ?? { visible: true, opacity: 1 }), ...patch },
      },
    }));
  };

  const moveLayer = (name: string, direction: -1 | 1) => {
    setWs((prev) => {
      const order = [...prev.order];
      const i = order.indexOf(name);
      const j = i + direction;
      if (i < 0 || j < 0 || j >= order.length) return prev;
      [order[i], order[j]] = [order[j], order[i]];
      return { ...prev, order };
    });
  };

  const dropLayer = (source: string, target: string) => {
    if (source === target) return;
    setWs((prev) => {
      const order = prev.order.filter((n) => n !== source);
      const idx = order.indexOf(target);
      order.splice(idx < 0 ? order.length : idx, 0, source);
      return { ...prev, order };
    });
  };

  const zoomTo = async (layer: Layer) => {
    try {
      if (layer.bounds) {
        setFlyTo([...layer.bounds]);
      } else {
        const b = await layerBounds(layer.name);
        if (b) setFlyTo(b);
        else toast.info("该图层暂无数据范围");
      }
      setLayerPanelOpen(false);
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const [flyTo, setFlyTo] = useState<[number, number, number, number] | null>(null);

  const focusFeature = useCallback(
    async (layer: string, fid: number | string, fit = false) => {
      try {
        const f = await getFeature(layer, Number(fid));
        setSelected({ layer, id: fid, properties: f.properties ?? {} });
        setSelectedFeature(f);
        setHighlight({ layer, fid, geometry: f.geometry as GeoJSONGeometry });
        if (f.geometry) {
          setHistory([f.geometry as GeoJSONGeometry]);
          setHistoryIndex(0);
        }
        if (fit && f.geometry) {
          const b = boundsOfGeometry(f.geometry as GeoJSONGeometry);
          if (b) setFlyTo(b);
        }
      } catch (err) {
        toast.error((err as Error).message);
      }
    },
    []
  );

  // URL 参数：?layer=&fid=&bbox=
  useEffect(() => {
    if (layers.length === 0 || focusRef.current) return;
    focusRef.current = true;
    const layerName = params.get("layer");
    const fid = params.get("fid");
    const bbox = params.get("bbox");
    if (bbox) {
      const parts = bbox.split(",").map(Number);
      if (parts.length === 4 && parts.every((n) => Number.isFinite(n))) setFlyTo(parts as any);
      if (layerName) setDrawLayerName(layerName);
    } else if (layerName && fid) {
      void focusFeature(layerName, fid, true);
    } else if (layerName) {
      const l = layers.find((x) => x.name === layerName);
      if (l) void zoomTo(l);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layers]);

  // 搜索（防抖）
  useEffect(() => {
    if (!query.trim()) {
      setResults(null);
      return;
    }
    const timer = window.setTimeout(() => {
      setSearching(true);
      searchAll(query.trim(), { limit: 5 })
        .then(setResults)
        .catch(() => setResults(null))
        .finally(() => setSearching(false));
    }, 300);
    return () => window.clearTimeout(timer);
  }, [query]);

  const handleSelect = (sel: SelectedFeature) => {
    setSelected(sel);
    if (sel.id !== undefined) {
      void focusFeature(sel.layer, sel.id, false);
    } else {
      setSelectedFeature(null);
      setHighlight(null);
      setHistory([]);
      setHistoryIndex(-1);
    }
  };

  const recordGeometry = (g: GeoJSONGeometry) => {
    setHistory((prev) => {
      const next = historyIndex >= 0 ? prev.slice(0, historyIndex + 1) : [];
      return [...next, g];
    });
    setHistoryIndex((i) => i + 1);
  };

  const handleGeometryChange = async (
    layer: string,
    fid: number | string,
    geometry: GeoJSONGeometry
  ) => {
    if (!selectedFeature) return;
    try {
      const updated = await updateFeature(layer, Number(fid), {
        ...selectedFeature,
        geometry,
      });
      setSelectedFeature(updated);
      setHighlight({ layer, fid, geometry });
      recordGeometry(geometry);
      toast.success("几何已更新");
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const applyHistoryGeometry = async (index: number) => {
    if (!selectedFeature || !highlight || index < 0 || index >= history.length) return;
    try {
      const g = history[index];
      const updated = await updateFeature(highlight.layer, Number(highlight.fid), {
        ...selectedFeature,
        geometry: g,
      });
      setSelectedFeature(updated);
      setHighlight({ ...highlight, geometry: g });
      setHistoryIndex(index);
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const handleDrawComplete = (geometry: GeoJSONGeometry) => {
    if (!drawLayer) return;
    setPendingGeometry(geometry);
    setProps(initialProps(drawLayer.fields));
    setDrawMode("none");
  };

  const submitNewFeature = async () => {
    if (!drawLayer || !pendingGeometry) return;
    try {
      await createFeatures(
        drawLayer.name,
        {
          type: "Feature",
          geometry: pendingGeometry,
          properties: normalizeProps(drawLayer.fields, props),
        },
        4326
      );
      toast.success("要素已保存");
      setPendingGeometry(null);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const openEdit = async (sel: SelectedFeature) => {
    if (sel.id === undefined) {
      toast.error("该要素缺少 id，无法编辑");
      return;
    }
    try {
      const f = await getFeature(sel.layer, Number(sel.id));
      setEditing({ layer: sel.layer, feature: f });
      setEditText(JSON.stringify(f, null, 2));
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const submitEdit = async () => {
    if (!editing) return;
    try {
      const parsed = JSON.parse(editText) as GeoJSONFeature;
      await updateFeature(editing.layer, Number(editing.feature.id), parsed);
      toast.success("要素已更新");
      setEditing(null);
      if (parsed.geometry) {
        setSelectedFeature(parsed);
        setHighlight({
          layer: editing.layer,
          fid: Number(editing.feature.id),
          geometry: parsed.geometry,
        });
      }
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const removeSelected = async () => {
    if (!selected || selected.id === undefined) return;
    if (!window.confirm(`删除要素 #${selected.id}？`)) return;
    try {
      await deleteFeature(selected.layer, Number(selected.id));
      toast.success("要素已删除");
      setSelected(null);
      setSelectedFeature(null);
      setHighlight(null);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const exportImage = () => {
    if (!mapView) return;
    const names = ordered.filter((l) => ws.layers[l.name]?.visible !== false).map((l) => l.name);
    if (!names.length) {
      toast.info("没有可见图层");
      return;
    }
    const width = 1600;
    const height = Math.round(
      (width * (mapView.bounds[3] - mapView.bounds[1])) /
        Math.max(1e-9, mapView.bounds[2] - mapView.bounds[0])
    );
    window.open(
      wmsGetMapMultiUrl({
        layers: names,
        bbox: mapView.bounds,
        width,
        height: Math.min(4000, Math.max(200, height)),
        transparent: false,
      }),
      "_blank"
    );
  };

  const printImage = () => {
    if (!mapView) return;
    const names = ordered.filter((l) => ws.layers[l.name]?.visible !== false);
    if (!names.length) {
      toast.info("没有可见图层");
      return;
    }
    const width = 1600;
    const height = Math.min(
      4000,
      Math.max(
        200,
        Math.round(
          (width * (mapView.bounds[3] - mapView.bounds[1])) /
            Math.max(1e-9, mapView.bounds[2] - mapView.bounds[0])
        )
      )
    );
    const url = wmsGetMapMultiUrl({
      layers: names.map((l) => l.name),
      bbox: mapView.bounds,
      width,
      height,
      transparent: false,
    });
    const legend = names
      .map(
        (l) =>
          `<div style="display:flex;align-items:center;gap:8px;margin:2px 0"><span style="display:inline-block;width:22px;height:12px;border-radius:3px;border:1px solid #475569;background:${layerSwatchColor(
            l
          )}"></span>${l.title || l.name}</div>`
      )
      .join("");
    const w = window.open("", "_blank");
    if (!w) {
      toast.error("浏览器阻止了打印窗口");
      return;
    }
    w.document.write(`<!doctype html><html><head><title>${ws.name || "MapServer"}</title><style>
      body{font-family:system-ui,sans-serif;margin:24px;color:#0f172a}
      h1{font-size:18px;margin:0 0 12px}
      img{max-width:100%;border:1px solid #e2e8f0}
      .legend{margin-top:12px;font-size:12px}
      @media print{body{margin:0}h1{font-size:16px}}
    </style></head><body>
      <h1>${ws.name || "MapServer 地图"}</h1>
      <img src="${url}" onload="window.print()" />
      <div class="legend">${legend}</div>
    </body></html>`);
    w.document.close();
  };

  const share = async () => {
    setSharing(true);
    try {
      const m = await createMap(shareName.trim() || "我的地图", ws, sharePublic);
      const url = `${window.location.origin}/share/${m.token}`;
      try {
        await navigator.clipboard.writeText(url);
        toast.success("分享链接已复制");
      } catch {
        toast.success("地图已保存");
      }
      window.prompt("分享链接（可复制）", url);
      setShareOpen(false);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setSharing(false);
    }
  };

  const handleBoxSelect = (bbox: [number, number, number, number]) => {
    setBoxSelect(false);
    const target = drawLayerName || ordered[0]?.name || "";
    if (!target) {
      toast.info("请先选择图层");
      return;
    }
    navigate(`/data?layer=${encodeURIComponent(target)}&bbox=${bbox.map((n) => n.toFixed(6)).join(",")}`);
  };

  const modes = allowedModes(drawLayer);
  const canFinish = drawing && vertices >= minVertices(drawMode);

  const layerRow = (l: Layer) => {
    const state = ws.layers[l.name] ?? { visible: true, opacity: 1 };
    return (
      <div
        key={l.id}
        draggable
        onDragStart={(e) => e.dataTransfer.setData("text/plain", l.name)}
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault();
          dropLayer(e.dataTransfer.getData("text/plain"), l.name);
        }}
        className="group rounded-md border-b px-2 py-2 last:border-b-0 hover:bg-accent/60"
      >
        <div className="flex items-center gap-2">
          <GripVertical className="h-4 w-4 shrink-0 cursor-grab text-muted-foreground" />
          <Switch
            checked={state.visible !== false}
            onCheckedChange={(v) => updateViewState(l.name, { visible: v })}
          />
          <span
            className="h-3 w-3 shrink-0 rounded-sm border"
            style={{ backgroundColor: layerSwatchColor(l) }}
            title="图例颜色"
          />
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{l.title || l.name}</div>
            <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <span className="truncate font-mono">{l.name}</span>
              <Badge variant="outline" className="px-1 py-0 text-[10px]">
                {l.geometryType}
              </Badge>
              <span>{l.featureCount}</span>
              {l.visibility === "private" && (
                <Badge variant="secondary" className="px-1 py-0 text-[10px]">
                  私有
                </Badge>
              )}
            </div>
          </div>
          <div className="flex shrink-0 items-center">
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              title="上移"
              onClick={() => moveLayer(l.name, -1)}
            >
              <ArrowUp className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              title="下移"
              onClick={() => moveLayer(l.name, 1)}
            >
              <ArrowDown className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              title="缩放到图层"
              onClick={() => void zoomTo(l)}
            >
              <Crosshair className="h-4 w-4" />
            </Button>
            {editable && (
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                title="图层样式"
                onClick={() => setStyleLayer(l)}
              >
                <Palette className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
        <div className="mt-1 flex items-center gap-2 pl-8">
          <span className="w-8 text-[10px] text-muted-foreground">透明</span>
          <input
            type="range"
            min={0.05}
            max={1}
            step={0.05}
            value={state.opacity ?? 1}
            className="h-1 flex-1"
            onChange={(e) => updateViewState(l.name, { opacity: Number(e.target.value) })}
          />
          <span className="w-7 text-right text-[10px] tabular-nums text-muted-foreground">
            {Math.round((state.opacity ?? 1) * 100)}%
          </span>
        </div>
      </div>
    );
  };

  const searchBox = (
    <div className="relative">
      <Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
      <Input
        className="h-9 pl-8"
        placeholder="搜索图层、属性或地点…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {query && (
        <div className="mt-1 max-h-72 overflow-y-auto rounded-md border bg-popover text-popover-foreground shadow-md">
          {searching && <div className="px-3 py-2 text-xs text-muted-foreground">搜索中…</div>}
          {results?.layers.map((l) => (
            <button
              key={`l-${l.name}`}
              className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent"
              onClick={() => {
                const layer = layers.find((x) => x.name === l.name);
                if (layer) void zoomTo(layer);
                setQuery("");
              }}
            >
              <LayersIcon className="h-4 w-4 text-muted-foreground" />
              <div className="min-w-0">
                <div className="truncate font-medium">{l.title || l.name}</div>
                <div className="text-[11px] text-muted-foreground">图层 · {l.featureCount} 个要素</div>
              </div>
            </button>
          ))}
          {results?.features.map((f) => {
            const name =
              String(f.feature.properties?.name ?? f.feature.properties?.title ?? "") ||
              `#${f.feature.id}`;
            return (
              <button
                key={`f-${f.layer}-${f.feature.id}`}
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent"
                onClick={() => {
                  if (f.feature.id !== undefined) void focusFeature(f.layer, f.feature.id, true);
                  setQuery("");
                }}
              >
                <Crosshair className="h-4 w-4 text-muted-foreground" />
                <div className="min-w-0">
                  <div className="truncate font-medium">{name}</div>
                  <div className="truncate text-[11px] text-muted-foreground">
                    {f.layerTitle || f.layer}
                  </div>
                </div>
              </button>
            );
          })}
          {!searching && results && results.layers.length === 0 && results.features.length === 0 && (
            <div className="px-3 py-3 text-xs text-muted-foreground">没有匹配结果</div>
          )}
        </div>
      )}
    </div>
  );

  const layerList = (
    <>
      <div className="border-b p-2">{searchBox}</div>
      <ScrollArea className="max-h-[44vh] md:max-h-[38vh]">
        <div className="p-1">
          {ordered.length === 0 && !loading && (
            <p className="px-2 py-4 text-center text-xs text-muted-foreground">暂无图层</p>
          )}
          {ordered.map(layerRow)}
        </div>
      </ScrollArea>
      <Separator />
      <div className="flex flex-wrap items-center gap-2 px-3 py-2.5">
        <span className="text-xs text-muted-foreground">底图</span>
        <Select
          value={ws.basemap}
          onValueChange={(v) => setWs((p) => ({ ...p, basemap: v as BasemapId }))}
        >
          <SelectTrigger className="h-8 w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {BASEMAPS.map((b) => (
              <SelectItem key={b.id} value={b.id}>
                {b.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button variant="outline" size="sm" onClick={exportImage} title="导出当前视图">
          <Download className="h-3.5 w-3.5" /> 图片
        </Button>
        <Button variant="outline" size="sm" onClick={printImage} title="打印当前视图">
          <Printer className="h-3.5 w-3.5" /> 打印
        </Button>
        <Button variant="outline" size="sm" onClick={() => setShareOpen(true)}>
          <Share2 className="h-3.5 w-3.5" /> 分享
        </Button>
      </div>
    </>
  );

  const drawControls = (compact: boolean) => {
    const modeButtons = (
      <>
        {modes.includes("point") && (
          <Button
            size="sm"
            className={compact ? "flex-1" : ""}
            variant={drawMode === "point" ? "default" : "outline"}
            onClick={() => setDrawMode(drawMode === "point" ? "none" : "point")}
          >
            <Circle className="h-4 w-4" /> 点
          </Button>
        )}
        {modes.includes("line") && (
          <Button
            size="sm"
            className={compact ? "flex-1" : ""}
            variant={drawMode === "line" ? "default" : "outline"}
            onClick={() => setDrawMode(drawMode === "line" ? "none" : "line")}
          >
            <Minus className="h-4 w-4" /> 线
          </Button>
        )}
        {modes.includes("polygon") && (
          <Button
            size="sm"
            className={compact ? "flex-1" : ""}
            variant={drawMode === "polygon" ? "default" : "outline"}
            onClick={() => setDrawMode(drawMode === "polygon" ? "none" : "polygon")}
          >
            <Pentagon className="h-4 w-4" /> 面
          </Button>
        )}
      </>
    );

    const finishButtons = (drawMode === "line" || drawMode === "polygon") && (
      <>
        <Button
          size="sm"
          variant="default"
          className={compact ? "flex-1" : ""}
          disabled={!canFinish}
          onClick={() => setFinishToken((t) => t + 1)}
        >
          完成{drawing ? `（${vertices}）` : ""}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          className={compact ? "flex-1" : ""}
          onClick={() => setDrawMode("none")}
        >
          <X className="h-4 w-4" /> 取消
        </Button>
      </>
    );

    if (compact) {
      return (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="shrink-0 text-xs text-muted-foreground">绘制到</span>
            <Select
              value={drawLayerName}
              onValueChange={(v) => {
                setDrawLayerName(v);
                setDrawMode("none");
              }}
            >
              <SelectTrigger className="h-9 min-w-0 flex-1">
                <SelectValue placeholder="选择图层" />
              </SelectTrigger>
              <SelectContent>
                {layers.map((l) => (
                  <SelectItem key={l.id} value={l.name}>
                    {l.title || l.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2 overflow-x-auto">
            {modeButtons}
            {finishButtons}
            <Button
              size="sm"
              variant={boxSelect ? "default" : "outline"}
              onClick={() => setBoxSelect((v) => !v)}
              title="框选后跳到数据表"
            >
              框选
            </Button>
          </div>
        </div>
      );
    }

    return (
      <>
        <Select
          value={drawLayerName}
          onValueChange={(v) => {
            setDrawLayerName(v);
            setDrawMode("none");
          }}
        >
          <SelectTrigger className="h-8 w-36">
            <SelectValue placeholder="选择图层" />
          </SelectTrigger>
          <SelectContent>
            {layers.map((l) => (
              <SelectItem key={l.id} value={l.name}>
                {l.title || l.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Separator orientation="vertical" className="h-6" />
        {modeButtons}
        {finishButtons}
        <Separator orientation="vertical" className="h-6" />
        <Button
          size="sm"
          variant={boxSelect ? "default" : "outline"}
          onClick={() => setBoxSelect((v) => !v)}
          title="框选后跳到数据表"
        >
          框选
        </Button>
      </>
    );
  };

  return (
    <div className="relative h-full w-full">
      <MapView
        layers={ordered}
        view={ws.layers}
        basemap={ws.basemap}
        backgroundColor={ws.backgroundColor}
        initialView={initialView}
        flyToBounds={flyTo}
        drawMode={drawMode}
        finishToken={finishToken}
        boxSelect={boxSelect}
        editMode={editMode}
        snapping={snapping}
        highlight={highlight}
        onSelectFeature={handleSelect}
        onDrawComplete={handleDrawComplete}
        onDrawStateChange={(d, v) => {
          setDrawing(d);
          setVertices(v);
        }}
        onBoxSelect={handleBoxSelect}
        onViewChange={(v) => {
          setMapView(v);
          setWs((prev) => ({
            ...prev,
            center: v.center,
            zoom: v.zoom,
            bearing: v.bearing,
            pitch: v.pitch,
          }));
        }}
        onGeometryChange={(layer, fid, g) => void handleGeometryChange(layer, fid, g)}
      />

      {/* 桌面端绘制工具条 */}
      {editable && (
        <Card className="absolute left-1/2 top-4 hidden -translate-x-1/2 shadow-lg md:block">
          <CardContent className="flex flex-wrap items-center gap-2 p-3">
            {drawControls(false)}
            {selectedFeature && (
              <>
                <Separator orientation="vertical" className="h-6" />
                <Button
                  size="sm"
                  variant={editMode === "move" ? "default" : "outline"}
                  onClick={() => setEditMode(editMode === "move" ? "none" : "move")}
                  title="拖动移动要素"
                >
                  <Move className="h-4 w-4" /> 移动
                </Button>
                <Button
                  size="sm"
                  variant={editMode === "vertex" ? "default" : "outline"}
                  onClick={() => setEditMode(editMode === "vertex" ? "none" : "vertex")}
                  title="拖动节点"
                >
                  <MousePointer2 className="h-4 w-4" /> 节点
                </Button>
                <Button
                  size="sm"
                  variant={snapping ? "default" : "outline"}
                  onClick={() => setSnapping((v) => !v)}
                  title="节点吸附"
                >
                  <Magnet className="h-4 w-4" />
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={historyIndex <= 0}
                  onClick={() => void applyHistoryGeometry(historyIndex - 1)}
                  title="撤销"
                >
                  <Undo2 className="h-4 w-4" />
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={historyIndex >= history.length - 1}
                  onClick={() => void applyHistoryGeometry(historyIndex + 1)}
                  title="重做"
                >
                  <Redo2 className="h-4 w-4" />
                </Button>
              </>
            )}
          </CardContent>
        </Card>
      )}

      {/* 桌面端图层面板 */}
      <Card className="absolute left-4 top-4 hidden w-80 shadow-lg md:block">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 p-3">
          <CardTitle className="text-sm">图层</CardTitle>
          <Button variant="ghost" size="icon" onClick={() => void refresh()}>
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
          </Button>
        </CardHeader>
        <CardContent className="p-0">{layerList}</CardContent>
      </Card>

      {/* 移动端图层按钮 + 抽屉 */}
      <Button
        className="absolute left-3 top-3 z-10 shadow-lg md:hidden"
        size="icon"
        variant="secondary"
        aria-label="图层"
        onClick={() => setLayerPanelOpen(true)}
      >
        <LayersIcon className="h-5 w-5" />
      </Button>
      <Sheet open={layerPanelOpen} onOpenChange={setLayerPanelOpen}>
        <SheetContent side="left" className="w-80 p-0">
          <SheetHeader className="border-b p-4 pt-safe">
            <SheetTitle className="flex items-center justify-between">
              图层
              <Button variant="ghost" size="icon" onClick={() => void refresh()}>
                {loading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
              </Button>
            </SheetTitle>
          </SheetHeader>
          {layerList}
        </SheetContent>
      </Sheet>

      {/* 移动端绘制底栏 */}
      {editable && (
        <div className="absolute inset-x-0 bottom-0 z-10 border-t bg-card/95 px-3 py-2.5 pb-safe backdrop-blur md:hidden">
          {drawControls(true)}
        </div>
      )}

      {/* 选中要素 */}
      {selected && (
        <Card
          className={[
            "absolute inset-x-2 z-20 max-h-[45%] overflow-y-auto shadow-lg md:inset-x-auto md:left-4 md:w-80 md:max-h-none",
            editable ? "bottom-36 md:bottom-4" : "bottom-3 md:bottom-4",
          ].join(" ")}
        >
          <CardHeader className="flex flex-row items-center justify-between space-y-0 p-3">
            <CardTitle className="truncate text-sm">
              要素 · <span className="font-mono">{selected.layer}</span>
              {selected.id !== undefined && (
                <span className="ml-1 text-muted-foreground">#{selected.id}</span>
              )}
            </CardTitle>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 shrink-0 text-xs"
              onClick={() => {
                setSelected(null);
                setSelectedFeature(null);
                setHighlight(null);
                setEditMode("none");
              }}
            >
              关闭
            </Button>
          </CardHeader>
          <CardContent className="p-3 pt-0">
            <div className="space-y-1 text-sm">
              {Object.entries(selected.properties)
                .filter(([k]) => k !== "fid")
                .map(([k, v]) => (
                  <div key={k} className="flex justify-between gap-3">
                    <span className="font-medium">{k}</span>
                    <span className="text-right text-muted-foreground">{String(v ?? "—")}</span>
                  </div>
                ))}
            </div>
            {editable && (
              <div className="mt-3 flex flex-wrap gap-2">
                <Button size="sm" variant="outline" onClick={() => void openEdit(selected)}>
                  <Pencil className="h-4 w-4" /> 属性
                </Button>
                {selectedFeature && (
                  <Button
                    size="sm"
                    variant={editMode === "move" ? "default" : "outline"}
                    onClick={() => setEditMode(editMode === "move" ? "none" : "move")}
                  >
                    <Move className="h-4 w-4" /> 移动
                  </Button>
                )}
                {selectedFeature && (
                  <Button
                    size="sm"
                    variant={editMode === "vertex" ? "default" : "outline"}
                    onClick={() => setEditMode(editMode === "vertex" ? "none" : "vertex")}
                  >
                    <SlidersHorizontal className="h-4 w-4" /> 节点
                  </Button>
                )}
                <Button size="sm" variant="ghost" onClick={() => void removeSelected()}>
                  <Trash2 className="h-4 w-4 text-destructive" /> 删除
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* 新要素属性录入 */}
      <Dialog open={!!pendingGeometry} onOpenChange={(v) => !v && setPendingGeometry(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>填写属性 · {drawLayer?.title || drawLayer?.name}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 py-1">
            {drawLayer?.fields.length === 0 && (
              <p className="text-sm text-muted-foreground">该图层没有属性字段</p>
            )}
            {drawLayer?.fields.map((f) => (
              <div key={f.name} className="grid gap-1.5">
                <Label className="text-xs">
                  {f.name}
                  <span className="ml-1 text-muted-foreground">({f.type})</span>
                </Label>
                {f.type === "boolean" ? (
                  <Select
                    value={String(props[f.name] ?? "false")}
                    onValueChange={(v) => setProps((p) => ({ ...p, [f.name]: v === "true" }))}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="true">true</SelectItem>
                      <SelectItem value="false">false</SelectItem>
                    </SelectContent>
                  </Select>
                ) : (
                  <Input
                    type={isNumeric(f.type) ? "number" : "text"}
                    inputMode={isNumeric(f.type) ? "decimal" : undefined}
                    value={String(props[f.name] ?? "")}
                    onChange={(e) => setProps((p) => ({ ...p, [f.name]: e.target.value }))}
                  />
                )}
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingGeometry(null)}>
              取消
            </Button>
            <Button onClick={submitNewFeature}>保存要素</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑要素 JSON */}
      <Dialog open={!!editing} onOpenChange={(v) => !v && setEditing(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>编辑要素属性</DialogTitle>
          </DialogHeader>
          <Textarea
            rows={14}
            className="font-mono text-xs"
            value={editText}
            onChange={(e) => setEditText(e.target.value)}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)}>
              取消
            </Button>
            <Button onClick={submitEdit}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 分享地图 */}
      <Dialog open={shareOpen} onOpenChange={setShareOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>分享地图</DialogTitle>
            <DialogDescription>
              保存当前视角、底图、图层可见性、顺序与透明度，生成无需登录的查看链接。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 py-1">
            <div className="grid gap-1.5">
              <Label>名称</Label>
              <Input value={shareName} onChange={(e) => setShareName(e.target.value)} />
            </div>
            <label className="flex items-center justify-between rounded-md border px-3 py-2 text-sm">
              公开链接（任何人可查看）
              <Switch checked={sharePublic} onCheckedChange={setSharePublic} />
            </label>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShareOpen(false)}>
              取消
            </Button>
            <Button onClick={() => void share()} disabled={sharing}>
              {sharing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Share2 className="h-4 w-4" />}
              生成链接
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <StyleEditor
        layer={styleLayer}
        open={!!styleLayer}
        onOpenChange={(v) => !v && setStyleLayer(null)}
        onSaved={(updated) => {
          setLayers((prev) => prev.map((l) => (l.name === updated.name ? updated : l)));
          setStyleLayer(null);
        }}
      />
    </div>
  );
}

function layerSwatchColor(l: Layer): string {
  const paint = (l.style as any)?.paint ?? {};
  const c = paint["fill-color"] ?? paint["line-color"] ?? paint["circle-color"];
  if (typeof c === "string" && c.startsWith("#")) return c;
  const g = l.geometryType.toUpperCase();
  if (g.includes("POINT")) return "#2563eb";
  if (g.includes("LINE")) return "#2563eb";
  return "#60a5fa";
}

function isNumeric(type: string): boolean {
  return ["integer", "bigint", "double"].includes(type);
}

function initialProps(fields: LayerField[]): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of fields) out[f.name] = f.type === "boolean" ? false : "";
  return out;
}

function normalizeProps(
  fields: LayerField[],
  props: Record<string, unknown>
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of fields) {
    const v = props[f.name];
    if (v === "" || v === undefined || v === null) {
      out[f.name] = null;
    } else if (isNumeric(f.type)) {
      out[f.name] = Number(v);
    } else if (f.type === "boolean") {
      out[f.name] = Boolean(v);
    } else {
      out[f.name] = v;
    }
  }
  return out;
}

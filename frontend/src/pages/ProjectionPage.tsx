import { useEffect, useMemo, useRef, useState } from "react";
import Map from "ol/Map";
import View from "ol/View";
import VectorTileLayer from "ol/layer/VectorTile";
import VectorTileSource from "ol/source/VectorTile";
import MVT from "ol/format/MVT";
import TileGrid from "ol/tilegrid/TileGrid";
import { register } from "ol/proj/proj4";
import { get as getProjection } from "ol/proj";
import { Circle as CircleStyle, Fill, Stroke, Style } from "ol/style";
import proj4 from "proj4";
import "ol/ol.css";
import { Info, Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { listLayers, tileUrlTemplateTMS } from "@/lib/api";
import type { Layer } from "@/types";

// 通过 proj4 注册一个非内置投影，演示“自定义投影”。
const CUSTOM_DEFS: Record<string, string> = {
  "EPSG:3413":
    "+proj=stere +lat_0=90 +lat_ts=70 +lon_0=-45 +x_0=0 +y_0=0 +datum=WGS84 +units=m +no_defs",
};

let projRegistered = false;
function ensureProjections() {
  if (projRegistered) return;
  const defs: Record<string, string> = {};
  for (const [code, def] of Object.entries(CUSTOM_DEFS)) {
    proj4.defs(code, def);
    defs[code] = def;
  }
  register(proj4);
  projRegistered = true;
}

const MAX_ZOOM = 12;

/** WorldCRS84Quad 瓦片网格：2^(z+1) × 2^z，原点左上。 */
const crs84Grid = new TileGrid({
  extent: [-180, -90, 180, 90],
  origin: [-180, 90],
  resolutions: Array.from({ length: MAX_ZOOM + 1 }, (_, z) => 180 / (256 * 2 ** z)),
  tileSize: 256,
});

interface ProjOption {
  id: string;
  label: string;
  center: [number, number];
  zoom: number;
}

const PROJECTIONS: ProjOption[] = [
  { id: "EPSG:4326", label: "EPSG:4326 等距圆柱（CRS84）", center: [0, 20], zoom: 1 },
  { id: "EPSG:3857", label: "EPSG:3857 Web 墨卡托", center: [0, 25], zoom: 1 },
  { id: "EPSG:3413", label: "EPSG:3413 北极极射赤面（自定义）", center: [0, 0], zoom: 0 },
];

const TILE_STYLE = new Style({
  fill: new Fill({ color: "rgba(99, 102, 241, 0.35)" }),
  stroke: new Stroke({ color: "#4338ca", width: 1 }),
  image: new CircleStyle({
    radius: 4,
    fill: new Fill({ color: "#f97316" }),
    stroke: new Stroke({ color: "#ffffff", width: 1 }),
  }),
});

export function ProjectionPage() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [layers, setLayers] = useState<Layer[]>([]);
  const [layerName, setLayerName] = useState("");
  const [projId, setProjId] = useState("EPSG:4326");
  const [loading, setLoading] = useState(true);

  ensureProjections();

  const load = async () => {
    setLoading(true);
    try {
      const data = await listLayers();
      setLayers(data);
      setLayerName((prev) => prev || data[0]?.name || "");
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const proj = useMemo(() => PROJECTIONS.find((p) => p.id === projId)!, [projId]);

  useEffect(() => {
    if (!containerRef.current || !layerName) return;
    const projection = getProjection(projId);
    const dataProjection = getProjection("EPSG:4326");
    if (!projection || !dataProjection) return;

    const source = new VectorTileSource({
      format: new MVT(),
      projection: dataProjection,
      tileGrid: crs84Grid,
      url: tileUrlTemplateTMS(layerName, "WorldCRS84Quad"),
      maxZoom: MAX_ZOOM,
      wrapX: false,
    });

    const vectorLayer = new VectorTileLayer({
      source,
      style: TILE_STYLE,
      declutter: false,
    });

    const map = new Map({
      target: containerRef.current,
      layers: [vectorLayer],
      view: new View({
        projection,
        center: proj.center,
        zoom: proj.zoom,
        maxZoom: MAX_ZOOM + 2,
      }),
    });

    return () => {
      map.setTarget(undefined);
    };
  }, [layerName, projId, proj]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">投影视图（OpenLayers）</h1>
          <p className="text-sm text-muted-foreground">
            以 WorldCRS84Quad 矢量瓦片为数据源，在客户端重投影到不同坐标系。
          </p>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <div className="grid gap-1.5">
            <Label className="text-xs">图层</Label>
            <Select value={layerName} onValueChange={setLayerName}>
              <SelectTrigger className="h-9 w-40">
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
          <div className="grid gap-1.5">
            <Label className="text-xs">视图投影</Label>
            <Select value={projId} onValueChange={setProjId}>
              <SelectTrigger className="h-9 w-64">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PROJECTIONS.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <button
            onClick={() => void load()}
            className="inline-flex h-9 items-center gap-2 rounded-md border border-input bg-background px-3 text-sm shadow-sm hover:bg-accent"
          >
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
            刷新
          </button>
        </div>
      </div>

      <div className="relative min-h-0 flex-1">
        <div ref={containerRef} className="h-full w-full bg-slate-100" />
        <Card className="absolute bottom-3 left-3 max-w-[min(92vw,420px)] shadow-lg">
          <CardHeader className="p-3 pb-1">
            <CardTitle className="flex items-center gap-2 text-xs">
              <Info className="h-3.5 w-3.5" /> 数据源
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 p-3 pt-0 text-[11px] text-muted-foreground">
            <div>
              TMS：<span className="font-mono">WorldCRS84Quad</span>（CRS:84，2^(z+1)×2^z）
            </div>
            <div className="break-all font-mono">{tileUrlTemplateTMS(layerName || "{layer}", "WorldCRS84Quad")}</div>
            <div>
              OpenLayers + proj4 客户端重投影；服务端矢量瓦片始终按矩阵集坐标系编码。
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

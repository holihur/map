import { useEffect, useMemo, useRef, useState } from "react";
import Map from "ol/Map";
import View from "ol/View";
import TileLayer from "ol/layer/Tile";
import XYZ from "ol/source/XYZ";
import TileGrid from "ol/tilegrid/TileGrid";
import Projection from "ol/proj/Projection";
import "ol/ol.css";
import { ExternalLink, Globe2, Info, KeyRound, Loader2, Plus } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { createExternalLayer } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { MAP_PROVIDERS, providerById } from "@/lib/providers";
import type { MapProvider } from "@/lib/providers";

const BAIDU_PROJECTION = new Projection({
  code: "baidu",
  units: "m",
  extent: [-20037726.37, -11708041.66, 20037726.37, 12474104.17],
});

// 百度使用 18 级、原点 (0,0)、y 轴向北的独立瓦片网格。
const BAIDU_RESOLUTIONS = Array.from({ length: 19 }, (_, i) => Math.pow(2, 18 - i));
const BAIDU_GRID = new TileGrid({
  origin: [0, 0],
  resolutions: BAIDU_RESOLUTIONS,
  tileSize: 256,
});

function baiduSource() {
  return new XYZ({
    projection: BAIDU_PROJECTION,
    tileGrid: BAIDU_GRID,
    crossOrigin: "anonymous",
    tileUrlFunction: (tileCoord) => {
      const z = tileCoord[0];
      const x = tileCoord[1];
      const y = -tileCoord[2] - 1;
      const s = (Math.abs(x + y) % 8) + 1;
      return `https://maponline${s}.bdimg.com/tile/?qt=vtile&x=${x}&y=${y}&z=${z}&styles=pl&scaler=1&udt=20240101`;
    },
  });
}

function urlFor(p: MapProvider, token: string): string {
  return p.url.split("{token}").join(token.trim());
}

export function ProvidersPage() {
  const { can } = useAuth();
  const editable = can("editor");
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<Map | null>(null);
  const [providerId, setProviderId] = useState("amap");
  const [token, setToken] = useState("");
  const [adding, setAdding] = useState(false);

  const provider = useMemo(() => providerById(providerId) ?? MAP_PROVIDERS[0], [providerId]);
  // OpenLayers 预览页只展示可直接用 z/x/y 访问的底图；WMS/MVT/GeoJSON 请在第三方图层中添加。
  const previewProviders = useMemo(
    () => MAP_PROVIDERS.filter((p) => ["xyz", "arcgis", "wmts"].includes(p.sourceType)),
    []
  );
  const isBaidu = provider.id === "baidu";
  const url = urlFor(provider, token);

  useEffect(() => {
    if (!containerRef.current) return;
    const source = isBaidu ? baiduSource() : new XYZ({
      url,
      crossOrigin: "anonymous",
      maxZoom: provider.maxZoom ?? 19,
    });
    const layer = new TileLayer({ source });
    const view = isBaidu
      ? new View({
          projection: BAIDU_PROJECTION,
          center: [12958175, 4825923.77],
          zoom: 12,
          maxZoom: 18,
        })
      : new View({
          projection: "EPSG:3857",
          center: [11639170, 4825923],
          zoom: 5,
          maxZoom: provider.maxZoom ?? 19,
        });
    const map = new Map({
      target: containerRef.current,
      layers: [layer],
      view,
    });
    mapRef.current = map;
    return () => {
      map.setTarget(undefined);
      mapRef.current = null;
    };
  }, [provider, url, isBaidu]);

  const addToMap = async () => {
    if (!editable) {
      toast.error("需要编辑者权限");
      return;
    }
    if (!provider.supportsMapLibre) {
      toast.error(provider.notes || "该服务商不支持 MapLibre 主地图");
      return;
    }
    if (provider.needsToken && !token.trim()) {
      toast.error("请先填写该服务商的 Token");
      return;
    }
    setAdding(true);
    try {
      const base = provider.id.replace(/[^a-z0-9_]/g, "_");
      const name = `${base}_${Date.now().toString(36).slice(-5)}`;
      await createExternalLayer({
        name,
        title: provider.label,
        description: provider.notes || "",
        sourceType: provider.sourceType,
        sourceUrl: url,
        sourceOptions: {
          attribution: provider.attribution,
          proxy: true,
        },
        style: {
          type: "raster",
          paint: { "raster-opacity": 1 },
        },
        metadata: {
          provider: provider.id,
          attribution: provider.attribution,
          bounds: [-180, -85, 180, 85],
        },
        minZoom: 0,
        maxZoom: provider.maxZoom ?? 22,
      });
      toast.success(`已添加「${provider.label}」，可在图层管理中调整`);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-col gap-3 border-b p-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">第三方底图 / 地图服务商</h1>
          <p className="text-sm text-muted-foreground">
            主流地图服务商预览。支持一键添加到主地图；百度使用 BD-09M 投影，仅在本页 OpenLayers 视图中正确显示。
          </p>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <div className="grid gap-1.5">
            <Label className="text-xs">服务商</Label>
            <Select value={providerId} onValueChange={setProviderId}>
              <SelectTrigger className="h-9 w-64">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {previewProviders.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.group} · {p.label}
                    {p.supportsMapLibre ? "" : "（OpenLayers）"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {provider.needsToken && (
            <div className="grid gap-1.5">
              <Label className="flex items-center gap-1.5 text-xs">
                <KeyRound className="h-3.5 w-3.5" /> Token / Key
              </Label>
              <Input
                className="h-9 w-56"
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="填写服务商 Token"
              />
            </div>
          )}
          <Button
            className="h-9"
            variant="outline"
            onClick={() => void addToMap()}
            disabled={!editable || adding || !provider.supportsMapLibre}
            title={
              provider.supportsMapLibre
                ? "作为第三方图层添加到主地图"
                : "百度投影不受 MapLibre 支持"
            }
          >
            {adding ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
            添加到主地图
          </Button>
        </div>
      </div>

      <div className="relative min-h-0 flex-1">
        <div ref={containerRef} className="h-full w-full bg-slate-100" />
        <Card className="absolute bottom-3 left-3 max-w-[min(92vw,460px)] shadow-lg">
          <CardHeader className="p-3 pb-1">
            <CardTitle className="flex items-center gap-2 text-xs">
              <Globe2 className="h-3.5 w-3.5" /> {provider.label}
              <Badge variant="outline" className="text-[10px]">
                {provider.group}
              </Badge>
              {!provider.supportsMapLibre && (
                <Badge variant="secondary" className="text-[10px]">
                  OpenLayers / BD-09M
                </Badge>
              )}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 p-3 pt-0 text-[11px] text-muted-foreground">
            <div className="break-all font-mono">{url || "（百度使用内置瓦片地址）"}</div>
            <div>{provider.attribution}</div>
            {provider.notes && (
              <div className="flex items-start gap-1.5">
                <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span>{provider.notes}</span>
              </div>
            )}
            {provider.needsToken && !token && (
              <div className="text-amber-600">该服务商需要 Token，请在上方填写。</div>
            )}
            <div className="flex items-center gap-2 pt-1">
              <ExternalLink className="h-3.5 w-3.5" />
              <span>坐标系：{isBaidu ? "BD-09M" : "Web Mercator (EPSG:3857)"}</span>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

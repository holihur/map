import { useEffect, useState } from "react";
import { ExternalLink, Loader2, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  apiBaseLabel,
  capabilities,
  getStats,
  listTileMatrixSets,
  metricsUrl,
  purgeCache,
  rasterTileUrlTemplate,
  tileUrlTemplate,
  wfsCapabilitiesUrl,
  wmsCapabilitiesUrl,
  wmsGetMapUrl,
  wmtsCapabilitiesUrl,
  type ServerStats,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Capabilities, TileMatrixSet } from "@/types";

export function CapabilitiesPage() {
  const { can } = useAuth();
  const [data, setData] = useState<Capabilities | null>(null);
  const [stats, setStats] = useState<ServerStats | null>(null);
  const [tms, setTms] = useState<TileMatrixSet[]>([]);
  const [loading, setLoading] = useState(true);

  const [wmsLayer, setWmsLayer] = useState("");
  const [wmsSrid, setWmsSrid] = useState(4326);
  const [wmsImg, setWmsImg] = useState("");

  const load = async () => {
    setLoading(true);
    try {
      const [cap, st, sets] = await Promise.all([
        capabilities(),
        getStats(),
        listTileMatrixSets(),
      ]);
      setData(cap);
      setStats(st);
      setTms(sets);
      setWmsLayer((prev) => prev || String(cap.layers[0]?.name ?? ""));
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const preview = () => {
    const layer = data?.layers.find((l) => String(l.name) === wmsLayer);
    if (!layer) return;
    const raw = (layer.bounds as [number, number, number, number] | undefined) ?? [
      -180, -90, 180, 90,
    ];
    const bbox: [number, number, number, number] =
      wmsSrid === 3857
        ? [
            lonToMercator(raw[0]),
            latToMercator(raw[1]),
            lonToMercator(raw[2]),
            latToMercator(raw[3]),
          ]
        : raw;
    setWmsImg(
      wmsGetMapUrl({
        layer: wmsLayer,
        srid: wmsSrid,
        bbox,
        width: 600,
        height: 300,
        swapAxes: wmsSrid === 4326,
      })
    );
  };

  const purge = async () => {
    try {
      await purgeCache();
      toast.success("缓存已清空");
      void load();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-5xl space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">服务能力</h1>
            <p className="text-sm text-muted-foreground">
              类似 OGC GetCapabilities 的服务元数据与接口说明。
            </p>
          </div>
          <Button variant="outline" onClick={() => void load()}>
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
            刷新
          </Button>
        </div>

        {data && (
          <>
            <Card>
              <CardHeader>
                <CardTitle className="text-base">
                  {data.service} · v{data.version}
                </CardTitle>
              </CardHeader>
              <CardContent className="grid gap-3 text-sm sm:grid-cols-2">
                <div>
                  <div className="text-muted-foreground">服务地址</div>
                  <code className="text-xs">{apiBaseLabel() || "同源"}</code>
                </div>
                <div>
                  <div className="text-muted-foreground">支持格式</div>
                  <div className="flex flex-wrap gap-1 pt-1">
                    {Object.entries(data.formats).map(([k, v]) => (
                      <Badge key={k} variant="outline">
                        {k}: {v.join(", ")}
                      </Badge>
                    ))}
                  </div>
                </div>
                <div className="sm:col-span-2">
                  <div className="text-muted-foreground">瓦片 URL 模板</div>
                  <code className="break-all text-xs">{tileUrlTemplate("{layer}")}</code>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0">
                <CardTitle className="text-base">缓存</CardTitle>
                {can("admin") && (
                  <Button variant="ghost" size="sm" onClick={() => void purge()}>
                    <Trash2 className="h-3.5 w-3.5" /> 清空缓存
                  </Button>
                )}
              </CardHeader>
              <CardContent className="grid gap-3 text-sm sm:grid-cols-2">
                <div>
                  <div className="text-muted-foreground">瓦片缓存</div>
                  <div className="pt-1 text-xs">
                    命中 {stats?.tileCache.hits ?? 0} · 未命中 {stats?.tileCache.misses ?? 0} · 条目{" "}
                    {stats?.tileCache.size ?? 0}/{stats?.tileCache.max ?? 0} · 淘汰{" "}
                    {stats?.tileCache.evicted ?? 0}
                  </div>
                </div>
                <div>
                  <div className="text-muted-foreground">元数据缓存</div>
                  <div className="pt-1 text-xs">
                    命中 {stats?.metaCache.hits ?? 0} · 未命中 {stats?.metaCache.misses ?? 0} · 条目{" "}
                    {stats?.metaCache.size ?? 0}
                  </div>
                </div>
                <div>
                  <div className="text-muted-foreground">磁盘缓存</div>
                  <div className="pt-1 text-xs">
                    {stats?.diskCache.enabled
                      ? `${stats.diskCache.files} 个文件 · ${(stats.diskCache.bytes / 1024).toFixed(1)} KB`
                      : "未启用"}
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">标准 OGC 接口</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-2 text-sm">
                {[
                  { label: "WMS GetCapabilities", url: wmsCapabilitiesUrl() },
                  { label: "WMTS GetCapabilities", url: wmtsCapabilitiesUrl() },
                  { label: "WFS GetCapabilities", url: wfsCapabilitiesUrl() },
                  { label: "Prometheus 指标", url: metricsUrl() },
                ].map((s) => (
                  <a
                    key={s.label}
                    href={s.url}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 rounded-md border px-3 py-1.5 text-xs hover:bg-accent"
                  >
                    <ExternalLink className="h-3.5 w-3.5" /> {s.label}
                  </a>
                ))}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0">
                <CardTitle className="text-base">栅格地图服务（WMS）</CardTitle>
                <Button asChild variant="ghost" size="sm">
                  <a href={wmsCapabilitiesUrl()} target="_blank" rel="noreferrer">
                    <ExternalLink className="h-3.5 w-3.5" /> GetCapabilities
                  </a>
                </Button>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <p className="text-muted-foreground">
                  支持任意 EPSG 坐标系的栅格出图（PostGIS 服务端重投影）。WMS 1.3.0 中 EPSG:4326
                  使用 lat,lon 轴顺序，已自动处理。
                </p>
                <div className="flex flex-wrap items-end gap-3">
                  <div className="grid gap-1.5">
                    <Label className="text-xs">图层</Label>
                    <Select value={wmsLayer} onValueChange={setWmsLayer}>
                      <SelectTrigger className="h-9 w-44">
                        <SelectValue placeholder="选择图层" />
                      </SelectTrigger>
                      <SelectContent>
                        {(data?.layers ?? []).map((l) => (
                          <SelectItem key={String(l.name)} value={String(l.name)}>
                            {String(l.title || l.name)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-1.5">
                    <Label className="text-xs">坐标系</Label>
                    <Select
                      value={String(wmsSrid)}
                      onValueChange={(v) => setWmsSrid(Number(v))}
                    >
                      <SelectTrigger className="h-9 w-40">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="4326">EPSG:4326</SelectItem>
                        <SelectItem value="3857">EPSG:3857</SelectItem>
                        {(data?.layers ?? [])
                          .filter((l) => ![4326, 3857].includes(Number(l.srid)))
                          .slice(0, 1)
                          .map((l) => (
                            <SelectItem key={String(l.srid)} value={String(l.srid)}>
                              EPSG:{String(l.srid)}
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <Button onClick={preview} disabled={!wmsLayer}>
                    预览 GetMap
                  </Button>
                </div>
                {wmsImg && (
                  <div className="space-y-2">
                    <div className="overflow-hidden rounded-lg border bg-[repeating-conic-gradient(#e5e7eb_0%_25%,white_0%_50%)] bg-[length:16px_16px]">
                      <img src={wmsImg} alt="WMS GetMap 预览" className="block w-full" />
                    </div>
                    <code className="block break-all text-[11px] text-muted-foreground">
                      {wmsImg}
                    </code>
                  </div>
                )}
                <div>
                  <div className="text-muted-foreground">GetCapabilities</div>
                  <code className="break-all text-xs">{wmsCapabilitiesUrl()}</code>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">瓦片矩阵集（TileMatrixSet）</CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm">
                {tms.map((g) => (
                  <div
                    key={g.id}
                    className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-3"
                  >
                    <div>
                      <div className="font-medium">{g.id}</div>
                      <div className="text-xs text-muted-foreground">
                        {g.crs} · {g.tileSize}px · {g.scheme} · 原点左上
                      </div>
                    </div>
                    <code className="text-[11px]">{g.tileUrl}</code>
                  </div>
                ))}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0">
                <CardTitle className="text-base">栅格瓦片（PNG / WMTS）</CardTitle>
                <Button asChild variant="ghost" size="sm">
                  <a href={wmtsCapabilitiesUrl()} target="_blank" rel="noreferrer">
                    <ExternalLink className="h-3.5 w-3.5" /> WMTS GetCapabilities
                  </a>
                </Button>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <p className="text-muted-foreground">
                  XYZ 模板可直接接入 OpenLayers / Leaflet 的 XYZ 图层；也提供标准 WMTS（KVP 与 RESTful）。
                </p>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  {[
                    [2, 2, 1],
                    [2, 3, 1],
                    [2, 2, 2],
                    [2, 3, 2],
                  ].map(([z, x, y]) => (
                    <img
                      key={`${z}-${x}-${y}`}
                      alt={`tile ${z}/${x}/${y}`}
                      className="w-full rounded-md border bg-[repeating-conic-gradient(#e5e7eb_0%_25%,white_0%_50%)] bg-[length:16px_16px]"
                      src={rasterTileUrlTemplate(wmsLayer || "cities")
                        .replace("{z}", String(z))
                        .replace("{x}", String(x))
                        .replace("{y}", String(y))}
                    />
                  ))}
                </div>
                <div className="space-y-1">
                  <div className="text-muted-foreground">XYZ 模板</div>
                  <code className="block break-all text-xs">
                    {rasterTileUrlTemplate(wmsLayer || "{layer}")}
                  </code>
                  <div className="text-muted-foreground">WMTS（KVP）</div>
                  <code className="block break-all text-xs">
                    {`${apiBaseLabel() || ""}/api/wmts?SERVICE=WMTS&REQUEST=GetTile&LAYER=${wmsLayer || "{layer}"}&TILEMATRIXSET=WebMercatorQuad&TILEMATRIX={z}&TILEROW={y}&TILECOL={x}&FORMAT=image/png`}
                  </code>
                  <div className="text-muted-foreground">WMTS（RESTful）</div>
                  <code className="block break-all text-xs">
                    {`${apiBaseLabel() || ""}/api/wmts/rest/${wmsLayer || "{layer}"}/default/WebMercatorQuad/{z}/{y}/{x}.png`}
                  </code>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">图层接口（{data.layers.length}）</CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm">
                {data.layers.map((l, i) => (
                  <div
                    key={i}
                    className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-3"
                  >
                    <div>
                      <div className="font-medium">
                        {String(l.title || l.name)}{" "}
                        <span className="font-mono text-xs text-muted-foreground">
                          {String(l.name)}
                        </span>
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {String(l.geometryType)} · {String(l.featureCount)} 要素
                      </div>
                    </div>
                    <div className="text-right">
                      <code className="block text-[11px]">
                        {String(l.tiles)}
                      </code>
                      <code className="block text-[11px]">{String(l.features)}</code>
                    </div>
                  </div>
                ))}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">原始 JSON</CardTitle>
              </CardHeader>
              <CardContent>
                <pre className="max-h-96 overflow-auto rounded-md bg-muted p-4 text-[11px] leading-relaxed">
                  {JSON.stringify(data, null, 2)}
                </pre>
              </CardContent>
            </Card>
          </>
        )}
      </div>
    </div>
  );
}

// Web Mercator 正变换（用于把 4326 范围转成 3857 传给 WMS）
const EARTH_RADIUS = 6378137;

function lonToMercator(lon: number): number {
  return (lon * Math.PI * EARTH_RADIUS) / 180;
}

function latToMercator(lat: number): number {
  const clamped = Math.max(-89.9, Math.min(89.9, lat));
  const phi = (clamped * Math.PI) / 180;
  return EARTH_RADIUS * Math.log(Math.tan(Math.PI / 4 + phi / 2));
}

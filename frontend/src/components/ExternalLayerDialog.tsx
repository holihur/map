import { useEffect, useMemo, useState } from "react";
import { KeyRound, Loader2, Save, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { createExternalLayer, updateExternalLayer } from "@/lib/api";
import { MAP_PROVIDERS } from "@/lib/providers";
import type { ExternalLayer, ExternalSourceType, LayerStyle } from "@/types";

function isRaster(type: ExternalSourceType) {
  return ["xyz", "wms", "wmts", "arcgis"].includes(type);
}

function draftFrom(layer: ExternalLayer | null) {
  const style = (layer?.style ?? {}) as any;
  const paint = (style.paint ?? {}) as any;
  const meta = (layer?.metadata ?? {}) as any;
  const opts = (layer?.sourceOptions ?? {}) as any;
  const color =
    String(paint["fill-color"] ?? paint["line-color"] ?? paint["circle-color"] ?? "#2563eb") ||
    "#2563eb";
  return {
    name: layer?.name ?? "",
    title: layer?.title ?? "",
    description: layer?.description ?? "",
    sourceType: (layer?.sourceType ?? "xyz") as ExternalSourceType,
    sourceUrl: layer?.sourceUrl ?? "",
    token: "",
    sourceLayer: typeof opts.sourceLayer === "string" ? opts.sourceLayer : "",
    tileSize: Number(opts.tileSize ?? 256) || 256,
    scheme: opts.scheme === "tms" ? "tms" : "xyz",
    proxy: opts.proxy !== false,
    attribution: String(opts.attribution ?? meta.attribution ?? ""),
    styleType: String(style.type ?? (isRaster((layer?.sourceType ?? "xyz") as ExternalSourceType) ? "raster" : "fill")),
    color,
    opacity: Number(paint["fill-opacity"] ?? paint["line-opacity"] ?? paint["circle-opacity"] ?? paint["raster-opacity"] ?? 1) || 1,
    width: Number(paint["line-width"] ?? 2) || 2,
    radius: Number(paint["circle-radius"] ?? 5) || 5,
    outline: String(paint["fill-outline-color"] ?? "#1d4ed8"),
    labelField: typeof meta.labelField === "string" ? meta.labelField : "",
    popupFields: Array.isArray(meta.popupFields) ? meta.popupFields.join(",") : "",
    bounds: Array.isArray(meta.bounds) ? (meta.bounds as number[]) : [],
    minZoom: layer?.minZoom ?? 0,
    maxZoom: layer?.maxZoom ?? 22,
    visibility: (layer?.visibility ?? "public") as "public" | "private",
    provider: String(meta.provider ?? ""),
    sortOrder: layer?.sortOrder ?? 0,
  };
}

export function ExternalLayerDialog({
  layer,
  open,
  onOpenChange,
  onSaved,
}: {
  layer: ExternalLayer | null;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onSaved: (layer: ExternalLayer) => void;
}) {
  const [draft, setDraft] = useState(() => draftFrom(layer));
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) setDraft(draftFrom(layer));
  }, [open, layer]);

  const update = (patch: Partial<typeof draft>) => setDraft((d) => ({ ...d, ...patch }));
  const preset = useMemo(
    () => MAP_PROVIDERS.find((p) => p.id === draft.provider),
    [draft.provider]
  );
  const raster = isRaster(draft.sourceType);

  const applyPreset = (id: string) => {
    const p = MAP_PROVIDERS.find((x) => x.id === id);
    if (!p) return;
    setDraft((d) => ({
      ...d,
      provider: id,
      sourceType: p.sourceType,
      sourceUrl: p.url || d.sourceUrl,
      attribution: p.attribution ?? d.attribution,
      maxZoom: p.maxZoom ?? d.maxZoom,
      styleType: p.styleType ?? d.styleType,
      title: d.title || p.label,
      name: d.name || (p.id === "custom" ? "" : p.id.replace(/[^a-z0-9_]/g, "_")),
    }));
  };

  const submit = async () => {
    if (!draft.name.trim()) {
      toast.error("请填写图层名称");
      return;
    }
    if (!draft.sourceUrl.trim()) {
      toast.error("请填写数据源 URL");
      return;
    }
    if (draft.sourceUrl.includes("{token}") && !draft.token.trim()) {
      toast.error("该服务商需要 Token，请填写访问密钥");
      return;
    }
    let url = draft.sourceUrl.trim();
    if (draft.token.trim()) {
      if (url.includes("{token}")) {
        url = url.split("{token}").join(draft.token.trim());
      } else if (/access_token=[^&]+/.test(url)) {
        url = url.replace(/access_token=[^&]+/, `access_token=${draft.token.trim()}`);
      } else {
        url += (url.includes("?") ? "&" : "?") + "access_token=" + draft.token.trim();
      }
    }
    const style: LayerStyle = raster
      ? {
          type: "raster",
          paint: {
            "raster-opacity": draft.opacity,
          },
        }
      : {
          type: draft.styleType as any,
          paint:
            draft.styleType === "line"
              ? { "line-color": draft.color, "line-width": draft.width, "line-opacity": draft.opacity }
              : draft.styleType === "circle"
                ? {
                    "circle-color": draft.color,
                    "circle-radius": draft.radius,
                    "circle-opacity": draft.opacity,
                    "circle-stroke-color": "#ffffff",
                    "circle-stroke-width": 1.5,
                  }
                : draft.styleType === "heatmap"
                  ? { "heatmap-radius": Math.max(5, draft.radius * 4), "heatmap-opacity": draft.opacity }
                  : {
                      "fill-color": draft.color,
                      "fill-opacity": draft.opacity,
                      "fill-outline-color": draft.outline,
                    },
        };
    const popupFields = draft.popupFields
      .split(",")
      .map((x: string) => x.trim())
      .filter(Boolean);
    const metadata: Record<string, unknown> = {
      provider: draft.provider || undefined,
      attribution: draft.attribution || undefined,
      labelField: draft.labelField || undefined,
      popupFields,
      bounds:
        draft.bounds.length === 4 && draft.bounds.every((n) => Number.isFinite(n))
          ? draft.bounds
          : undefined,
    };
    const sourceOptions: Record<string, unknown> = {
      sourceLayer: draft.sourceLayer || undefined,
      tileSize: raster ? draft.tileSize : undefined,
      scheme: raster ? draft.scheme : undefined,
      attribution: draft.attribution || undefined,
      proxy: draft.proxy,
    };
    setSaving(true);
    try {
      const input = {
        name: draft.name.trim().toLowerCase(),
        title: draft.title.trim() || draft.name.trim(),
        description: draft.description.trim(),
        sourceType: draft.sourceType,
        sourceUrl: url,
        sourceOptions,
        style,
        metadata,
        minZoom: draft.minZoom,
        maxZoom: draft.maxZoom,
        sortOrder: draft.sortOrder,
        visibility: draft.visibility,
      };
      const saved = layer
        ? await updateExternalLayer(layer.name, input)
        : await createExternalLayer(input);
      toast.success(layer ? "第三方图层已更新" : "第三方图层已创建");
      onSaved(saved);
      onOpenChange(false);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles className="h-4 w-4" />
            {layer ? "编辑第三方图层" : "添加第三方图层"}
          </DialogTitle>
          <DialogDescription>
            支持高德、Google、Mapbox、WMS、WMTS、XYZ、MVT 与 GeoJSON。第三方图层不写入 PostGIS，
            由浏览器直接作为底图/叠加层渲染。
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-1">
          <div className="grid gap-1.5">
            <Label>服务商预设</Label>
            <Select value={draft.provider || "custom-xyz"} onValueChange={applyPreset}>
              <SelectTrigger>
                <SelectValue placeholder="选择服务商或自定义" />
              </SelectTrigger>
              <SelectContent>
                {MAP_PROVIDERS.filter((p) => p.supportsMapLibre).map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.group} · {p.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-[11px] text-muted-foreground">
              高德 / Google 在国内使用 GCJ-02 偏移；与 WGS84 数据叠加时可能有数百米偏差。百度使用
              BD-09M 自定义投影，MapLibre 无法直接渲染。
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label>名称（英文标识）</Label>
              <Input
                value={draft.name}
                disabled={!!layer}
                onChange={(e) => update({ name: e.target.value })}
                placeholder="amap_road"
              />
            </div>
            <div className="grid gap-1.5">
              <Label>标题</Label>
              <Input value={draft.title} onChange={(e) => update({ title: e.target.value })} />
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label>类型</Label>
              <Select
                value={draft.sourceType}
                onValueChange={(v) => {
                  const t = v as ExternalSourceType;
                  update({
                    sourceType: t,
                    styleType: isRaster(t) ? "raster" : draft.styleType === "raster" ? "fill" : draft.styleType,
                  });
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="xyz">XYZ 栅格瓦片</SelectItem>
                  <SelectItem value="wms">WMS</SelectItem>
                  <SelectItem value="wmts">WMTS</SelectItem>
                  <SelectItem value="mvt">矢量瓦片 MVT</SelectItem>
                  <SelectItem value="geojson">GeoJSON URL</SelectItem>
                  <SelectItem value="arcgis">ArcGIS 瓦片</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label>可见性</Label>
              <Select
                value={draft.visibility}
                onValueChange={(v) => update({ visibility: v as "public" | "private" })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="public">公开</SelectItem>
                  <SelectItem value="private">私有（仅编辑者/管理员）</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label>数据源 URL</Label>
            <Textarea
              rows={3}
              className="font-mono text-xs"
              value={draft.sourceUrl}
              onChange={(e) => update({ sourceUrl: e.target.value })}
              placeholder="https://.../{z}/{x}/{y}.png"
            />
            <p className="text-[11px] text-muted-foreground">
              XYZ 使用 <code>{"{z}/{x}/{y}"}</code>；WMS 使用 <code>{"{bbox-epsg-3857}"}</code>；
              WMTS 可用 <code>{"{z}/{y}/{x}"}</code>；MVT 填 .pbf/.mvt 模板。
            </p>
          </div>

          <label className="flex items-start justify-between gap-3 rounded-md border p-3 text-sm">
            <span>
              通过服务器代理
              <span className="mt-0.5 block text-[11px] text-muted-foreground">
                推荐开启：由 MapServer 代理瓦片/数据请求，避免浏览器 CORS 限制，并可缓存；WMS 会自动计算
                BBOX。
              </span>
            </span>
            <Switch
              checked={draft.proxy}
              onCheckedChange={(v) => update({ proxy: v })}
            />
          </label>

          {(preset?.needsToken || draft.sourceUrl.includes("{token}")) && (
            <div className="grid gap-1.5">
              <Label className="flex items-center gap-1.5">
                <KeyRound className="h-3.5 w-3.5" /> 访问 Token（如 Mapbox）
              </Label>
              <Input
                type="password"
                value={draft.token}
                onChange={(e) => update({ token: e.target.value })}
                placeholder="pk.xxx（保存后会写入 URL）"
              />
            </div>
          )}

          <div className="grid gap-3 sm:grid-cols-3">
            <div className="grid gap-1.5">
              <Label>最小缩放</Label>
              <Input
                type="number"
                value={draft.minZoom}
                onChange={(e) => update({ minZoom: Number(e.target.value) || 0 })}
              />
            </div>
            <div className="grid gap-1.5">
              <Label>最大缩放</Label>
              <Input
                type="number"
                value={draft.maxZoom}
                onChange={(e) => update({ maxZoom: Number(e.target.value) || 22 })}
              />
            </div>
            <div className="grid gap-1.5">
              <Label>排序</Label>
              <Input
                type="number"
                value={draft.sortOrder}
                onChange={(e) => update({ sortOrder: Number(e.target.value) || 0 })}
              />
            </div>
          </div>

          {!raster && (
            <div className="grid gap-3 rounded-md border p-3 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label>渲染类型</Label>
                <Select
                  value={draft.styleType}
                  onValueChange={(v) => update({ styleType: v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="fill">面</SelectItem>
                    <SelectItem value="line">线</SelectItem>
                    <SelectItem value="circle">点</SelectItem>
                    <SelectItem value="heatmap">热力图</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {draft.sourceType === "mvt" && (
                <div className="grid gap-1.5">
                  <Label>source-layer</Label>
                  <Input
                    value={draft.sourceLayer}
                    onChange={(e) => update({ sourceLayer: e.target.value })}
                    placeholder="矢量瓦片中的图层名"
                  />
                </div>
              )}
              <div className="grid gap-1.5">
                <Label>颜色</Label>
                <Input
                  type="color"
                  className="h-9 p-1"
                  value={draft.color}
                  onChange={(e) => update({ color: e.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>透明度 {Math.round(draft.opacity * 100)}%</Label>
                <input
                  type="range"
                  min={0.05}
                  max={1}
                  step={0.05}
                  value={draft.opacity}
                  onChange={(e) => update({ opacity: Number(e.target.value) })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>标注字段（可选）</Label>
                <Input
                  value={draft.labelField}
                  onChange={(e) => update({ labelField: e.target.value })}
                  placeholder="name"
                />
              </div>
              <div className="grid gap-1.5">
                <Label>弹窗字段（逗号分隔）</Label>
                <Input
                  value={draft.popupFields}
                  onChange={(e) => update({ popupFields: e.target.value })}
                  placeholder="name,population"
                />
              </div>
            </div>
          )}

          {raster && (
            <div className="grid gap-3 rounded-md border p-3 sm:grid-cols-3">
              <div className="grid gap-1.5">
                <Label>透明度 {Math.round(draft.opacity * 100)}%</Label>
                <input
                  type="range"
                  min={0.05}
                  max={1}
                  step={0.05}
                  value={draft.opacity}
                  onChange={(e) => update({ opacity: Number(e.target.value) })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>瓦片尺寸</Label>
                <Select
                  value={String(draft.tileSize)}
                  onValueChange={(v) => update({ tileSize: Number(v) })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="256">256</SelectItem>
                    <SelectItem value="512">512</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label>瓦片方案</Label>
                <Select value={draft.scheme} onValueChange={(v) => update({ scheme: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="xyz">XYZ（左上原点）</SelectItem>
                    <SelectItem value="tms">TMS（左下原点）</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          <div className="grid gap-1.5">
            <Label>归属声明（attribution）</Label>
            <Input
              value={draft.attribution}
              onChange={(e) => update({ attribution: e.target.value })}
              placeholder="© 高德地图"
            />
          </div>

          <div className="grid gap-1.5">
            <Label>数据范围（可选，用于“缩放到图层”）</Label>
            <div className="grid grid-cols-4 gap-2">
              {["minX", "minY", "maxX", "maxY"].map((label, i) => (
                <Input
                  key={label}
                  type="number"
                  step="0.0001"
                  placeholder={label}
                  value={draft.bounds[i] ?? ""}
                  onChange={(e) => {
                    const next = [...draft.bounds];
                    next[i] = Number(e.target.value);
                    update({ bounds: next });
                  }}
                />
              ))}
            </div>
          </div>

          {preset && (
            <div className="flex flex-wrap items-center gap-2 rounded-md bg-muted/40 p-2 text-[11px] text-muted-foreground">
              <Badge variant="outline">{preset.sourceType.toUpperCase()}</Badge>
              <span className="break-all font-mono">{preset.url || "自定义 URL"}</span>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => void submit()} disabled={saving}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {layer ? "保存" : "创建"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

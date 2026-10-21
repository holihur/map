import { useEffect, useMemo, useState } from "react";
import { Loader2, Palette, Save } from "lucide-react";
import { toast } from "sonner";

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
import { layerStats, updateLayer } from "@/lib/api";
import type { FieldStat, Layer } from "@/types";

const PALETTE = [
  "#2563eb",
  "#16a34a",
  "#f97316",
  "#dc2626",
  "#9333ea",
  "#0891b2",
  "#ca8a04",
  "#db2777",
  "#4f46e5",
  "#65a30d",
];

type RenderType = "fill" | "line" | "circle" | "heatmap";

interface Draft {
  type: RenderType;
  color: string;
  color2: string;
  opacity: number;
  width: number;
  radius: number;
  outline: string;
  labelField: string;
  labelSize: number;
  labelColor: string;
  popupFields: string[];
  minZoom: number;
  maxZoom: number;
  classField: string;
}

function draftFromLayer(l: Layer): Draft {
  const style = (l.style ?? {}) as Record<string, any>;
  const paint = (style.paint ?? {}) as Record<string, any>;
  const meta = (l.metadata ?? {}) as Record<string, any>;
  const geom = l.geometryType.toUpperCase();
  const fallback: RenderType = geom.includes("POINT") ? "circle" : geom.includes("LINE") ? "line" : "fill";
  const rawType = String(style.type ?? "");
  const type: RenderType = ["fill", "line", "circle", "heatmap"].includes(rawType)
    ? (rawType as RenderType)
    : fallback;
  return {
    type,
    color:
      String(
        paint["fill-color"] ??
          paint["line-color"] ??
          paint["circle-color"] ??
          paint["heatmap-color"] ??
          "#2563eb"
      ) || "#2563eb",
    color2: String(paint["fill-outline-color"] ?? paint["circle-stroke-color"] ?? "#ffffff"),
    opacity: Number(paint["fill-opacity"] ?? paint["line-opacity"] ?? paint["circle-opacity"] ?? 1) || 1,
    width: Number(paint["line-width"] ?? 2) || 2,
    radius: Number(paint["circle-radius"] ?? 5) || 5,
    outline: String(paint["fill-outline-color"] ?? "#1d4ed8"),
    labelField: typeof meta.labelField === "string" ? meta.labelField : "",
    labelSize: Number(meta.labelSize ?? 12) || 12,
    labelColor: String(meta.labelColor ?? "#111827"),
    popupFields: Array.isArray(meta.popupFields)
      ? meta.popupFields.filter((x): x is string => typeof x === "string")
      : [],
    minZoom: l.minZoom,
    maxZoom: l.maxZoom,
    classField: typeof meta.classField === "string" ? meta.classField : "",
  };
}

export function StyleEditor({
  layer,
  open,
  onOpenChange,
  onSaved,
}: {
  layer: Layer | null;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onSaved: (layer: Layer) => void;
}) {
  const [draft, setDraft] = useState<Draft | null>(null);
  const [stats, setStats] = useState<FieldStat[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !layer) return;
    setDraft(draftFromLayer(layer));
    setStats([]);
    void layerStats(layer.name)
      .then(setStats)
      .catch(() => setStats([]));
  }, [open, layer]);

  const mode = draft?.type ?? "fill";
  const canHeatmap = layer?.geometryType.toUpperCase().includes("POINT");

  const classValues = useMemo(() => {
    if (!draft?.classField) return [];
    const st = stats.find((s) => s.field === draft.classField);
    return (st?.top ?? []).slice(0, 10);
  }, [draft?.classField, stats]);

  const update = (patch: Partial<Draft>) => setDraft((d) => (d ? { ...d, ...patch } : d));

  const submit = async () => {
    if (!layer || !draft) return;
    setSaving(true);
    try {
      const type = mode;
      const paint: Record<string, unknown> = {};
      if (type === "fill") {
        if (classValues.length) {
          paint["fill-color"] = [
            "match",
            ["to-string", ["get", draft.classField]],
            ...classValues.flatMap((v, i) => [String(v.value), PALETTE[i % PALETTE.length]]),
            draft.color,
          ];
        } else {
          paint["fill-color"] = draft.color;
        }
        paint["fill-opacity"] = draft.opacity;
        paint["fill-outline-color"] = draft.outline;
      } else if (type === "line") {
        paint["line-color"] = draft.color;
        paint["line-width"] = draft.width;
        paint["line-opacity"] = draft.opacity;
      } else if (type === "circle") {
        paint["circle-color"] = draft.color;
        paint["circle-radius"] = draft.radius;
        paint["circle-opacity"] = draft.opacity;
        paint["circle-stroke-color"] = draft.color2;
        paint["circle-stroke-width"] = 1.5;
      } else {
        paint["heatmap-radius"] = Math.max(5, draft.radius * 4);
        paint["heatmap-opacity"] = draft.opacity;
      }
      const metadata = {
        ...(layer.metadata ?? {}),
        labelField: draft.labelField || undefined,
        labelSize: draft.labelSize,
        labelColor: draft.labelColor,
        popupFields: draft.popupFields,
        classField: draft.classField || undefined,
      };
      const updated = await updateLayer(layer.name, {
        style: { type, paint },
        metadata,
        minZoom: draft.minZoom,
        maxZoom: draft.maxZoom,
      } as any);
      toast.success("样式已保存");
      onSaved(updated);
      onOpenChange(false);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Palette className="h-4 w-4" /> 图层样式 · {layer?.title || layer?.name}
          </DialogTitle>
          <DialogDescription>
            可视化调整绘制样式、标注与弹窗字段；保存后立即影响地图与瓦片。
          </DialogDescription>
        </DialogHeader>

        {draft && (
          <div className="grid gap-5 py-1">
            <div className="grid gap-3 sm:grid-cols-3">
              {canHeatmap && (
                <div className="grid gap-1.5">
                  <Label>渲染方式</Label>
                  <Select
                    value={draft.type}
                    onValueChange={(v) => update({ type: v as RenderType })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="circle">点符号</SelectItem>
                      <SelectItem value="heatmap">热力图</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              )}
              <div className="grid gap-1.5">
                <Label>填充 / 线 / 点颜色</Label>
                <Input
                  type="color"
                  className="h-9 p-1"
                  value={draft.color}
                  onChange={(e) => update({ color: e.target.value })}
                />
              </div>
              {mode === "fill" && (
                <div className="grid gap-1.5">
                  <Label>边界颜色</Label>
                  <Input
                    type="color"
                    className="h-9 p-1"
                    value={draft.outline}
                    onChange={(e) => update({ outline: e.target.value })}
                  />
                </div>
              )}
              {mode === "circle" && (
                <div className="grid gap-1.5">
                  <Label>描边颜色</Label>
                  <Input
                    type="color"
                    className="h-9 p-1"
                    value={draft.color2}
                    onChange={(e) => update({ color2: e.target.value })}
                  />
                </div>
              )}
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
              {mode === "line" && (
                <div className="grid gap-1.5">
                  <Label>线宽 {draft.width}px</Label>
                  <input
                    type="range"
                    min={0.5}
                    max={12}
                    step={0.5}
                    value={draft.width}
                    onChange={(e) => update({ width: Number(e.target.value) })}
                  />
                </div>
              )}
              {mode === "circle" && (
                <div className="grid gap-1.5">
                  <Label>点半径 {draft.radius}px</Label>
                  <input
                    type="range"
                    min={1}
                    max={20}
                    step={1}
                    value={draft.radius}
                    onChange={(e) => update({ radius: Number(e.target.value) })}
                  />
                </div>
              )}
            </div>

            <div className="grid gap-3 rounded-md border p-3 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label>按字段分级着色（可选）</Label>
                <Select
                  value={draft.classField || "__none__"}
                  onValueChange={(v) => update({ classField: v === "__none__" ? "" : v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">不使用</SelectItem>
                    {layer?.fields.map((f) => (
                      <SelectItem key={f.name} value={f.name}>
                        {f.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {classValues.length > 0 && (
                  <div className="flex flex-wrap gap-1 pt-1">
                    {classValues.map((v, i) => (
                      <span
                        key={v.value}
                        className="inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
                      >
                        <span
                          className="h-2.5 w-2.5 rounded-sm"
                          style={{ backgroundColor: PALETTE[i % PALETTE.length] }}
                        />
                        {v.value}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <div className="grid gap-1.5">
                <Label>标注字段</Label>
                <Select
                  value={draft.labelField || "__none__"}
                  onValueChange={(v) => update({ labelField: v === "__none__" ? "" : v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">不显示标注</SelectItem>
                    {layer?.fields.map((f) => (
                      <SelectItem key={f.name} value={f.name}>
                        {f.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div className="flex items-center gap-2">
                  <Input
                    type="number"
                    className="h-8 w-20"
                    value={draft.labelSize}
                    onChange={(e) => update({ labelSize: Number(e.target.value) || 12 })}
                  />
                  <Input
                    type="color"
                    className="h-8 w-14 p-1"
                    value={draft.labelColor}
                    onChange={(e) => update({ labelColor: e.target.value })}
                  />
                </div>
              </div>
            </div>

            <div className="grid gap-2 rounded-md border p-3">
              <Label>点击弹窗显示字段</Label>
              <div className="flex flex-wrap gap-2">
                {layer?.fields.map((f) => {
                  const checked = draft.popupFields.includes(f.name);
                  return (
                    <label
                      key={f.name}
                      className="inline-flex items-center gap-1.5 rounded border px-2 py-1 text-xs"
                    >
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={(e) =>
                          update({
                            popupFields: e.target.checked
                              ? [...draft.popupFields, f.name]
                              : draft.popupFields.filter((x) => x !== f.name),
                          })
                        }
                      />
                      {f.name}
                    </label>
                  );
                })}
                {layer?.fields.length === 0 && (
                  <span className="text-xs text-muted-foreground">无属性字段</span>
                )}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label>最小缩放级别</Label>
                <Input
                  type="number"
                  min={0}
                  max={22}
                  value={draft.minZoom}
                  onChange={(e) => update({ minZoom: Number(e.target.value) || 0 })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>最大缩放级别</Label>
                <Input
                  type="number"
                  min={0}
                  max={22}
                  value={draft.maxZoom}
                  onChange={(e) => update({ maxZoom: Number(e.target.value) || 22 })}
                />
              </div>
            </div>

            <div className="rounded-md border bg-muted/40 p-3">
              <div className="mb-2 text-xs font-medium text-muted-foreground">图例预览</div>
              <div className="flex items-center gap-2 text-sm">
                <span
                  className="inline-block h-4 w-8 rounded"
                  style={{
                    backgroundColor: draft.color,
                    opacity: draft.opacity,
                    border: `2px solid ${mode === "fill" ? draft.outline : draft.color2}`,
                    borderRadius: mode === "circle" ? "999px" : "4px",
                  }}
                />
                <span>{layer?.title || layer?.name}</span>
                {draft.labelField && (
                  <span className="text-xs text-muted-foreground">· 标注 {draft.labelField}</span>
                )}
              </div>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => void submit()} disabled={!draft || saving}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            保存样式
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Circle,
  Crosshair,
  Layers as LayersIcon,
  Loader2,
  Minus,
  Pencil,
  Pentagon,
  RefreshCw,
  Trash2,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { MapView, type DrawMode, type SelectedFeature } from "@/components/map/MapView";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
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
  deleteFeature,
  getFeature,
  layerBounds,
  listLayers,
  updateFeature,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { GeoJSONFeature, GeoJSONGeometry, Layer, LayerField } from "@/types";

const BASEMAPS = [
  { name: "浅色", color: "#f8fafc" },
  { name: "深色", color: "#0f172a" },
  { name: "米色", color: "#f5f0e6" },
];

/** 根据图层几何类型返回允许的绘制模式。 */
function allowedModes(layer: Layer | null): DrawMode[] {
  if (!layer) return [];
  const g = layer.geometryType.toUpperCase();
  if (g.includes("POINT")) return ["point"];
  if (g.includes("LINE")) return ["line"];
  if (g.includes("POLYGON")) return ["polygon"];
  return ["point", "line", "polygon"];
}

/** 完成绘制所需的最少节点数。 */
function minVertices(mode: DrawMode): number {
  if (mode === "polygon") return 3;
  if (mode === "line") return 2;
  return 1;
}

export function MapPage() {
  const { can } = useAuth();
  const editable = can("editor");

  const [layers, setLayers] = useState<Layer[]>([]);
  const [visible, setVisible] = useState<Record<string, boolean>>({});
  const [basemapColor, setBasemapColor] = useState(BASEMAPS[0].color);
  const [flyToBounds, setFlyToBounds] = useState<[number, number, number, number] | null>(null);
  const [loading, setLoading] = useState(true);
  const [layerPanelOpen, setLayerPanelOpen] = useState(false);

  const [drawLayerName, setDrawLayerName] = useState<string>("");
  const [drawMode, setDrawMode] = useState<DrawMode>("none");
  const [finishToken, setFinishToken] = useState(0);
  const [drawing, setDrawing] = useState(false);
  const [vertices, setVertices] = useState(0);
  const [pendingGeometry, setPendingGeometry] = useState<GeoJSONGeometry | null>(null);
  const [props, setProps] = useState<Record<string, unknown>>({});

  const [selected, setSelected] = useState<SelectedFeature | null>(null);
  const [editing, setEditing] = useState<{ layer: string; feature: GeoJSONFeature } | null>(null);
  const [editText, setEditText] = useState("");

  const drawLayer = useMemo(
    () => layers.find((l) => l.name === drawLayerName) ?? null,
    [layers, drawLayerName]
  );

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const data = await listLayers();
      setLayers(data);
      setVisible((prev) => {
        const next: Record<string, boolean> = {};
        for (const l of data) next[l.name] = prev[l.name] ?? true;
        return next;
      });
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

  const zoomTo = async (layer: Layer) => {
    try {
      const b = await layerBounds(layer.name);
      if (b) setFlyToBounds([...b]);
      else toast.info("该图层暂无数据范围");
      setLayerPanelOpen(false);
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
      await createFeatures(drawLayer.name, {
        type: "Feature",
        geometry: pendingGeometry,
        properties: normalizeProps(drawLayer.fields, props),
      }, 4326);
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
      setSelected(null);
      void refresh();
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
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const modes = allowedModes(drawLayer);
  const canFinish = drawing && vertices >= minVertices(drawMode);

  /** 图层面板内容（桌面卡片与移动抽屉共用）。 */
  const layerList = (
    <>
      <ScrollArea className="max-h-[48vh] md:max-h-[40vh]">
        <div className="space-y-1 p-2">
          {layers.length === 0 && !loading && (
            <p className="px-2 py-4 text-center text-xs text-muted-foreground">暂无图层</p>
          )}
          {layers.map((l) => (
            <div
              key={l.id}
              className="flex items-center gap-2 rounded-md px-2 py-2 hover:bg-accent"
            >
              <Switch
                checked={visible[l.name] !== false}
                onCheckedChange={(v) => setVisible((prev) => ({ ...prev, [l.name]: v }))}
              />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{l.title || l.name}</div>
                <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                  <span className="truncate font-mono">{l.name}</span>
                  <Badge variant="outline" className="px-1 py-0 text-[10px]">
                    {l.geometryType}
                  </Badge>
                  <span>{l.featureCount}</span>
                </div>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 shrink-0"
                title="缩放到图层"
                onClick={() => void zoomTo(l)}
              >
                <Crosshair className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
      </ScrollArea>
      <Separator />
      <div className="flex items-center gap-2 px-3 py-2.5">
        <span className="text-xs text-muted-foreground">底图</span>
        {BASEMAPS.map((b) => (
          <button
            key={b.name}
            onClick={() => setBasemapColor(b.color)}
            className={`h-7 w-7 rounded-full border-2 transition ${
              basemapColor === b.color ? "border-primary ring-1 ring-primary" : "border-border"
            }`}
            style={{ backgroundColor: b.color }}
            title={b.name}
          />
        ))}
      </div>
    </>
  );

  /** 绘制控件（桌面工具条与移动底栏共用）。 */
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
            {drawMode === "point" && (
              <span className="shrink-0 text-xs text-muted-foreground">点击地图放置点</span>
            )}
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
          <SelectTrigger className="h-8 w-40">
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
        {drawMode === "point" && (
          <span className="text-xs text-muted-foreground">点击地图放置点；再次点击按钮可退出</span>
        )}
      </>
    );
  };

  return (
    <div className="relative h-full w-full">
      <MapView
        layers={layers}
        visible={visible}
        basemapColor={basemapColor}
        flyToBounds={flyToBounds}
        drawMode={drawMode}
        finishToken={finishToken}
        onSelectFeature={setSelected}
        onDrawComplete={handleDrawComplete}
        onDrawStateChange={(d, v) => {
          setDrawing(d);
          setVertices(v);
        }}
      />

      {/* 桌面端绘制工具条 */}
      {editable && (
        <Card className="absolute left-1/2 top-4 hidden -translate-x-1/2 shadow-lg md:block">
          <CardContent className="flex flex-wrap items-center gap-2 p-3">
            <span className="text-xs text-muted-foreground">绘制到</span>
            {drawControls(false)}
          </CardContent>
        </Card>
      )}

      {/* 桌面端图层面板 */}
      <Card className="absolute left-4 top-4 hidden w-72 shadow-lg md:block">
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
        <SheetContent side="left" className="w-72 p-0">
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
              onClick={() => setSelected(null)}
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
              <div className="mt-3 flex gap-2">
                <Button size="sm" variant="outline" onClick={() => void openEdit(selected)}>
                  <Pencil className="h-4 w-4" /> 编辑
                </Button>
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
            <DialogTitle>编辑要素</DialogTitle>
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
    </div>
  );
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

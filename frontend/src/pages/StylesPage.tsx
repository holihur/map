import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, RefreshCw, Star, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";

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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { createStyle, deleteStyle, listStyles, updateStyle, uploadSLD } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { MapStyle } from "@/types";

const DEFAULT_STYLE = {
  version: 8,
  sources: {},
  layers: [
    { id: "background", type: "background", paint: { "background-color": "#f8fafc" } },
  ],
};

export function StylesPage() {
  const { can } = useAuth();
  const editable = can("editor");
  const [styles, setStyles] = useState<MapStyle[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [sldOpen, setSldOpen] = useState(false);
  const [sldName, setSldName] = useState("");
  const [sldText, setSldText] = useState("");
  const [sldDefault, setSldDefault] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [json, setJson] = useState(JSON.stringify(DEFAULT_STYLE, null, 2));

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setStyles(await listStyles());
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const openCreate = () => {
    setEditingId(null);
    setName("");
    setIsDefault(styles.length === 0);
    setJson(JSON.stringify(DEFAULT_STYLE, null, 2));
    setDialogOpen(true);
  };

  const openEdit = (s: MapStyle) => {
    setEditingId(s.id);
    setName(s.name);
    setIsDefault(s.isDefault);
    setJson(JSON.stringify(s.style, null, 2));
    setDialogOpen(true);
  };

  const submit = async () => {
    if (!name.trim()) {
      toast.error("请填写样式名称");
      return;
    }
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(json);
    } catch {
      toast.error("样式 JSON 解析失败");
      return;
    }
    try {
      if (editingId) {
        await updateStyle(editingId, { name, isDefault, style: parsed });
        toast.success("样式已更新");
      } else {
        await createStyle({ name, isDefault, style: parsed });
        toast.success("样式已创建");
      }
      setDialogOpen(false);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const submitSLD = async () => {
    if (!sldName.trim() || !sldText.trim()) {
      toast.error("请填写样式名称与 SLD 内容");
      return;
    }
    try {
      await uploadSLD(sldName.trim(), sldText, sldDefault);
      toast.success(`SLD 样式「${sldName}」已保存`);
      setSldOpen(false);
      setSldName("");
      setSldText("");
      setSldDefault(false);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const makeDefault = async (s: MapStyle) => {
    try {
      await updateStyle(s.id, { name: s.name, isDefault: true, style: s.style });
      toast.success(`「${s.name}」已设为默认样式`);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const remove = async (s: MapStyle) => {
    if (!window.confirm(`删除样式「${s.name}」？`)) return;
    try {
      await deleteStyle(s.id);
      toast.success("样式已删除");
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">样式管理</h1>
            <p className="text-sm text-muted-foreground">
              保存 MapLibre Style 规范样式，供地图渲染使用。
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => void refresh()}>
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              刷新
            </Button>
            {editable && (
              <Button variant="outline" onClick={() => setSldOpen(true)}>
                <Upload className="h-4 w-4" /> 导入 SLD
              </Button>
            )}
            {editable && (
              <Button onClick={openCreate}>
                <Plus className="h-4 w-4" /> 新建样式
              </Button>
            )}
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {styles.map((s) => (
            <Card key={s.id}>
              <CardHeader className="flex flex-row items-center justify-between space-y-0">
                <CardTitle className="flex items-center gap-2 text-base">
                  {s.name}
                  {s.isDefault && <Badge variant="success">默认</Badge>}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <pre className="max-h-40 overflow-auto rounded-md bg-muted p-3 text-[11px] leading-relaxed">
                  {JSON.stringify(s.style, null, 2)}
                </pre>
                <div className="flex items-center gap-2">
                  {editable && (
                    <Button variant="outline" size="sm" onClick={() => openEdit(s)}>
                      编辑
                    </Button>
                  )}
                  {editable && !s.isDefault && (
                    <Button variant="ghost" size="sm" onClick={() => void makeDefault(s)}>
                      <Star className="h-3.5 w-3.5" /> 设为默认
                    </Button>
                  )}
                  {editable && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="ml-auto"
                      onClick={() => void remove(s)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
          {styles.length === 0 && !loading && (
            <Card className="md:col-span-2 lg:col-span-3">
              <CardContent className="py-16 text-center text-sm text-muted-foreground">
                暂无样式
              </CardContent>
            </Card>
          )}
        </div>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingId ? "编辑样式" : "新建样式"}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label>名称</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="flex items-center gap-3">
              <Switch checked={isDefault} onCheckedChange={setIsDefault} />
              <Label>设为默认样式</Label>
            </div>
            <div className="grid gap-2">
              <Label>样式 JSON</Label>
              <Textarea
                rows={14}
                className="font-mono text-xs"
                value={json}
                onChange={(e) => setJson(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={submit}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={sldOpen} onOpenChange={setSldOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>导入 SLD 样式</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label>样式名称</Label>
              <Input
                value={sldName}
                onChange={(e) => setSldName(e.target.value)}
                placeholder="例如 green-countries"
              />
            </div>
            <div className="flex items-center gap-3">
              <Switch checked={sldDefault} onCheckedChange={setSldDefault} />
              <Label>设为默认样式</Label>
            </div>
            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label>SLD 内容</Label>
                <input
                  type="file"
                  accept=".sld,.xml"
                  className="text-xs file:mr-2 file:rounded file:border file:border-input file:bg-background file:px-2 file:py-1"
                  onChange={async (e) => {
                    const f = e.target.files?.[0];
                    if (f) setSldText(await f.text());
                  }}
                />
              </div>
              <Textarea
                rows={12}
                className="font-mono text-xs"
                value={sldText}
                onChange={(e) => setSldText(e.target.value)}
                placeholder='<StyledLayerDescriptor>...</StyledLayerDescriptor>'
              />
              <p className="text-[11px] text-muted-foreground">
                支持按比例尺（Min/MaxScaleDenominator）选择规则；WMS 可用 STYLES=名称 引用。
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSldOpen(false)}>
              取消
            </Button>
            <Button onClick={submitSLD}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

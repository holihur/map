import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  ArrowDown,
  ArrowUp,
  Globe2,
  Loader2,
  Lock,
  Palette,
  Plus,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-react";
import { toast } from "sonner";

import { CRSInput } from "@/components/CRSInput";
import { ExternalLayerDialog } from "@/components/ExternalLayerDialog";
import { ImportLayerDialog } from "@/components/ImportLayerDialog";
import { LayerFormDialog } from "@/components/LayerFormDialog";
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
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  deleteExternalLayer,
  deleteLayer,
  getLayerAccess,
  listExternalLayers,
  listLayers,
  listUsers,
  reorderLayers,
  setLayerAccess,
  transformLayer,
  type AuthUser,
  type LayerAccess,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { ExternalLayer, Layer } from "@/types";

export function LayersPage() {
  const { can } = useAuth();
  const editable = can("editor");
  const isAdmin = can("admin");
  const [layers, setLayers] = useState<Layer[]>([]);
  const [externalLayers, setExternalLayers] = useState<ExternalLayer[]>([]);
  const [loading, setLoading] = useState(true);
  const [externalOpen, setExternalOpen] = useState(false);
  const [editExternal, setEditExternal] = useState<ExternalLayer | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [styleLayer, setStyleLayer] = useState<Layer | null>(null);
  const [transformTarget, setTransformTarget] = useState<Layer | null>(null);
  const [transformSrid, setTransformSrid] = useState(4326);

  const [accessTarget, setAccessTarget] = useState<Layer | null>(null);
  const [accessVisibility, setAccessVisibility] = useState<"public" | "private">("public");
  const [accessRows, setAccessRows] = useState<LayerAccess[]>([]);
  const [users, setUsers] = useState<AuthUser[]>([]);
  const [accessSaving, setAccessSaving] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [base, external] = await Promise.all([listLayers(), listExternalLayers()]);
      setLayers(base);
      setExternalLayers(external);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const removeExternal = async (layer: ExternalLayer) => {
    if (!window.confirm(`删除第三方图层「${layer.title || layer.name}」？`)) return;
    try {
      await deleteExternalLayer(layer.name);
      toast.success("第三方图层已删除");
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const remove = async (layer: Layer) => {
    if (
      !window.confirm(
        `确定删除图层「${layer.title || layer.name}」？物理表 ${layer.tableName} 及其数据将被永久删除。`
      )
    )
      return;
    try {
      await deleteLayer(layer.name);
      toast.success("图层已删除");
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const move = async (layer: Layer, direction: -1 | 1) => {
    const names = layers.map((l) => l.name);
    const i = names.indexOf(layer.name);
    const j = i + direction;
    if (i < 0 || j < 0 || j >= names.length) return;
    [names[i], names[j]] = [names[j], names[i]];
    try {
      await reorderLayers(names);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const openTransform = (layer: Layer) => {
    setTransformTarget(layer);
    setTransformSrid(layer.srid);
  };

  const submitTransform = async () => {
    if (!transformTarget) return;
    try {
      const updated = await transformLayer(transformTarget.name, transformSrid);
      toast.success(`图层坐标系已转换为 EPSG:${updated.srid}`);
      setTransformTarget(null);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const openAccess = async (layer: Layer) => {
    setAccessTarget(layer);
    setAccessVisibility(layer.visibility ?? "public");
    setAccessRows([]);
    try {
      const settings = await getLayerAccess(layer.name);
      setAccessVisibility(settings.visibility ?? layer.visibility ?? "public");
      setAccessRows(settings.access ?? []);
    } catch (err) {
      toast.error((err as Error).message);
    }
    if (editable) {
      try {
        setUsers(await listUsers());
      } catch {
        setUsers([]);
      }
    } else {
      setUsers([]);
    }
  };

  const submitAccess = async () => {
    if (!accessTarget) return;
    setAccessSaving(true);
    try {
      await setLayerAccess(accessTarget.name, accessVisibility, accessRows);
      toast.success("发布设置已保存");
      setAccessTarget(null);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setAccessSaving(false);
    }
  };

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">图层管理</h1>
            <p className="text-sm text-muted-foreground">
              管理 featureType、可视化样式、排序与发布权限。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => void refresh()}>
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              刷新
            </Button>
            {editable && (
              <Button
                variant="outline"
                onClick={() => {
                  setEditExternal(null);
                  setExternalOpen(true);
                }}
              >
                <Globe2 className="h-4 w-4" /> 第三方图层
              </Button>
            )}
            {editable && (
              <Button variant="outline" onClick={() => setImportOpen(true)}>
                <Upload className="h-4 w-4" /> 上传数据建层
              </Button>
            )}
            {editable && (
              <Button onClick={() => setDialogOpen(true)}>
                <Plus className="h-4 w-4" /> 新建图层
              </Button>
            )}
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">全部图层（{layers.length}）</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-16">顺序</TableHead>
                  <TableHead>名称</TableHead>
                  <TableHead>几何类型</TableHead>
                  <TableHead className="hidden sm:table-cell">坐标系</TableHead>
                  <TableHead className="hidden lg:table-cell">字段</TableHead>
                  <TableHead className="hidden sm:table-cell">缩放</TableHead>
                  <TableHead className="text-right">要素数</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {layers.map((l, index) => (
                  <TableRow key={l.id}>
                    <TableCell>
                      <div className="flex items-center gap-0.5">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-6"
                          disabled={!editable || index === 0}
                          onClick={() => void move(l, -1)}
                          title="上移"
                        >
                          <ArrowUp className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-6"
                          disabled={!editable || index === layers.length - 1}
                          onClick={() => void move(l, 1)}
                          title="下移"
                        >
                          <ArrowDown className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">{l.title || l.name}</div>
                      <div className="flex items-center gap-1 font-mono text-xs text-muted-foreground">
                        {l.name} · {l.tableName}
                        {l.visibility === "private" && (
                          <Badge variant="secondary" className="text-[9px]">
                            私有
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">{l.geometryType}</Badge>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">
                      <Badge variant="outline" className="font-mono text-[10px]">
                        EPSG:{l.srid}
                      </Badge>
                    </TableCell>
                    <TableCell className="hidden lg:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {l.fields.length === 0 && (
                          <span className="text-xs text-muted-foreground">—</span>
                        )}
                        {l.fields.map((f) => (
                          <Badge key={f.name} variant="outline" className="font-mono text-[10px]">
                            {f.name}:{f.type}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell text-xs text-muted-foreground">
                      {l.minZoom}–{l.maxZoom}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{l.featureCount}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-0.5">
                        {editable && (
                          <Button
                            variant="ghost"
                            size="icon"
                            title="样式"
                            onClick={() => setStyleLayer(l)}
                          >
                            <Palette className="h-4 w-4" />
                          </Button>
                        )}
                        {editable && (
                          <Button
                            variant="ghost"
                            size="icon"
                            title="发布与权限"
                            onClick={() => void openAccess(l)}
                          >
                            <Lock className="h-4 w-4" />
                          </Button>
                        )}
                        <Button asChild variant="ghost" size="sm">
                          <Link to={`/data?layer=${l.name}`}>数据</Link>
                        </Button>
                        {editable && (
                          <Button
                            variant="ghost"
                            size="icon"
                            title="转换坐标系"
                            onClick={() => openTransform(l)}
                          >
                            <Globe2 className="h-4 w-4" />
                          </Button>
                        )}
                        {isAdmin && (
                          <Button variant="ghost" size="icon" onClick={() => void remove(l)}>
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
                {layers.length === 0 && !loading && (
                  <TableRow>
                    <TableCell
                      colSpan={8}
                      className="py-10 text-center text-sm text-muted-foreground"
                    >
                      暂无图层，点击「新建图层」或上传数据开始
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0">
            <CardTitle className="text-base">
              第三方图层（{externalLayers.length}）
            </CardTitle>
            {editable && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setEditExternal(null);
                  setExternalOpen(true);
                }}
              >
                <Plus className="h-4 w-4" /> 添加
              </Button>
            )}
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>类型</TableHead>
                  <TableHead className="hidden md:table-cell">数据源</TableHead>
                  <TableHead className="hidden sm:table-cell">缩放</TableHead>
                  <TableHead>可见性</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {externalLayers.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell>
                      <div className="font-medium">{l.title || l.name}</div>
                      <div className="font-mono text-xs text-muted-foreground">{l.name}</div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">{l.sourceType.toUpperCase()}</Badge>
                      {typeof (l.metadata as any)?.provider === "string" && (
                        <Badge variant="outline" className="ml-1 text-[10px]">
                          {(l.metadata as any).provider}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className="hidden max-w-[320px] truncate font-mono text-[11px] text-muted-foreground md:table-cell">
                      {l.sourceUrl}
                    </TableCell>
                    <TableCell className="hidden sm:table-cell text-xs text-muted-foreground">
                      {l.minZoom}–{l.maxZoom}
                    </TableCell>
                    <TableCell>
                      <Badge variant={l.visibility === "private" ? "secondary" : "outline"}>
                        {l.visibility === "private" ? "私有" : "公开"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      {editable && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setEditExternal(l);
                            setExternalOpen(true);
                          }}
                        >
                          编辑
                        </Button>
                      )}
                      {editable && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => void removeExternal(l)}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
                {externalLayers.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={6} className="py-8 text-center text-sm text-muted-foreground">
                      暂无第三方图层，可添加高德 / Google / Mapbox / WMS / WMTS / MVT / GeoJSON
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      <LayerFormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onCreated={() => void refresh()}
      />
      <ExternalLayerDialog
        layer={editExternal}
        open={externalOpen}
        onOpenChange={(v) => {
          setExternalOpen(v);
          if (!v) setEditExternal(null);
        }}
        onSaved={() => void refresh()}
      />
      <ImportLayerDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        onCreated={() => void refresh()}
      />
      <StyleEditor
        layer={styleLayer}
        open={!!styleLayer}
        onOpenChange={(v) => !v && setStyleLayer(null)}
        onSaved={(updated) => {
          setLayers((prev) => prev.map((l) => (l.name === updated.name ? updated : l)));
          setStyleLayer(null);
        }}
      />

      <Dialog open={!!transformTarget} onOpenChange={(v) => !v && setTransformTarget(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>转换坐标系</DialogTitle>
            <DialogDescription>
              将图层「{transformTarget?.title || transformTarget?.name}」的全部几何从
              EPSG:{transformTarget?.srid} 转换到目标坐标系（会重写 geom 列并重建空间索引）。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2 py-1">
            <Label>目标坐标系 (SRID)</Label>
            <CRSInput value={transformSrid} onChange={setTransformSrid} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setTransformTarget(null)}>
              取消
            </Button>
            <Button
              onClick={submitTransform}
              disabled={!transformTarget || transformSrid === transformTarget.srid}
            >
              转换
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 发布与权限 */}
      <Dialog open={!!accessTarget} onOpenChange={(v) => !v && setAccessTarget(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>发布设置 · {accessTarget?.title || accessTarget?.name}</DialogTitle>
            <DialogDescription>
              私有图层仅授权用户可读取；公开图层可被任意客户端匿名访问。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-1">
            <div className="grid gap-1.5">
              <Label>可见性</Label>
              <Select
                value={accessVisibility}
                onValueChange={(v) => setAccessVisibility(v as "public" | "private")}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="public">公开（匿名可读）</SelectItem>
                  <SelectItem value="private">私有（仅授权用户）</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>授权用户</Label>
                {editable && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      setAccessRows((rows) => [
                        ...rows,
                        { userId: users[0]?.id ?? 0, username: users[0]?.username ?? "", level: "read" },
                      ])
                    }
                    disabled={users.length === 0}
                  >
                    <Plus className="h-3.5 w-3.5" /> 添加
                  </Button>
                )}
              </div>
              {accessRows.length === 0 && (
                <p className="text-xs text-muted-foreground">
                  "暂无授权用户"
                </p>
              )}
              {accessRows.map((row, i) => (
                <div key={i} className="flex items-center gap-2">
                  <Select
                    value={String(row.userId)}
                    onValueChange={(v) => {
                      const u = users.find((x) => String(x.id) === v);
                      setAccessRows((rows) =>
                        rows.map((r, ri) =>
                          ri === i ? { ...r, userId: Number(v), username: u?.username ?? "" } : r
                        )
                      );
                    }}
                  >
                    <SelectTrigger className="flex-1">
                      <SelectValue placeholder="选择用户" />
                    </SelectTrigger>
                    <SelectContent>
                      {users.map((u) => (
                        <SelectItem key={u.id} value={String(u.id)}>
                          {u.username}（{u.role}）
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Select
                    value={row.level}
                    onValueChange={(v) =>
                      setAccessRows((rows) =>
                        rows.map((r, ri) =>
                          ri === i ? { ...r, level: v as "read" | "write" } : r
                        )
                      )
                    }
                  >
                    <SelectTrigger className="w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="read">只读</SelectItem>
                      <SelectItem value="write">可写</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={() => setAccessRows((rows) => rows.filter((_, ri) => ri !== i))}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAccessTarget(null)}>
              取消
            </Button>
            <Button onClick={() => void submitAccess()} disabled={accessSaving}>
              {accessSaving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              保存设置
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

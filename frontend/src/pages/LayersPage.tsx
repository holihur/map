import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Globe2, Loader2, Plus, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { CRSInput } from "@/components/CRSInput";
import { LayerFormDialog } from "@/components/LayerFormDialog";
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { deleteLayer, listLayers, transformLayer } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Layer } from "@/types";

export function LayersPage() {
  const { can } = useAuth();
  const editable = can("editor");
  const isAdmin = can("admin");
  const [layers, setLayers] = useState<Layer[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [transformTarget, setTransformTarget] = useState<Layer | null>(null);
  const [transformSrid, setTransformSrid] = useState(4326);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setLayers(await listLayers());
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

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

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">图层管理</h1>
            <p className="text-sm text-muted-foreground">
              管理 GeoServer 式的 featureType：每个图层对应一张 PostGIS 空间表。
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
                {layers.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell>
                      <div className="font-medium">{l.title || l.name}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {l.name} · {l.tableName}
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
                    <TableCell className="text-right tabular-nums">
                      {l.featureCount}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
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
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => void remove(l)}
                          >
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
                      colSpan={7}
                      className="py-10 text-center text-sm text-muted-foreground"
                    >
                      暂无图层，点击「新建图层」开始
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
    </div>
  );
}

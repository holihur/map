import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { ChevronLeft, ChevronRight, Download, Loader2, Plus, RefreshCw, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { CRSInput } from "@/components/CRSInput";
import {
  Dialog,
  DialogContent,
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import {
  createFeatures,
  deleteFeature,
  exportAllUrl,
  exportUrl,
  importLayer,
  listFeatures,
  listLayers,
  updateFeature,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { GeoJSONFeature, Layer } from "@/types";

const PAGE_SIZE = 25;

export function DataPage() {
  const { can } = useAuth();
  const editable = can("editor");
  const [params, setParams] = useSearchParams();
  const [layers, setLayers] = useState<Layer[]>([]);
  const [features, setFeatures] = useState<GeoJSONFeature[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(false);

  const [filterField, setFilterField] = useState<string>("");
  const [filterValue, setFilterValue] = useState("");
  const [appliedFilter, setAppliedFilter] = useState<{ field: string; value: string } | null>(
    null
  );

  const [addOpen, setAddOpen] = useState(false);
  const [addText, setAddText] = useState("");
  const [addSrid, setAddSrid] = useState(4326);
  const [importOpen, setImportOpen] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importSrid, setImportSrid] = useState(4326);
  const [importing, setImporting] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [editing, setEditing] = useState<GeoJSONFeature | null>(null);
  const [editText, setEditText] = useState("");

  const layerName = params.get("layer") ?? "";
  const layer = useMemo(
    () => layers.find((l) => l.name === layerName) ?? null,
    [layers, layerName]
  );

  useEffect(() => {
    void (async () => {
      try {
        const data = await listLayers();
        setLayers(data);
        if (!params.get("layer") && data.length > 0) {
          setParams({ layer: data[0].name }, { replace: true });
        }
      } catch (err) {
        toast.error((err as Error).message);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const loadFeatures = useCallback(async () => {
    if (!layerName) return;
    setLoading(true);
    try {
      const filters: Record<string, string> = {};
      if (appliedFilter) filters[appliedFilter.field] = appliedFilter.value;
      const data = await listFeatures(layerName, {
        limit: PAGE_SIZE,
        offset,
        filters,
      });
      setFeatures(data.features ?? []);
      setTotal(data.total ?? 0);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [layerName, offset, appliedFilter]);

  useEffect(() => {
    void loadFeatures();
  }, [loadFeatures]);

  useEffect(() => {
    setOffset(0);
    setAppliedFilter(null);
    setFilterField("");
    setFilterValue("");
  }, [layerName]);

  const applyFilter = () => {
    if (!filterField || !filterValue) {
      setAppliedFilter(null);
    } else {
      setAppliedFilter({ field: filterField, value: filterValue });
    }
    setOffset(0);
  };

  const remove = async (f: GeoJSONFeature) => {
    if (!layer) return;
    const fid = Number(f.id);
    if (!window.confirm(`删除要素 #${fid}？`)) return;
    try {
      await deleteFeature(layer.name, fid);
      toast.success("要素已删除");
      void loadFeatures();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const submitAdd = async () => {
    if (!layer) return;
    try {
      const parsed = JSON.parse(addText);
      const res = await createFeatures(layer.name, parsed, addSrid);
      toast.success(`已插入 ${res.count} 个要素`);
      setAddOpen(false);
      setAddText("");
      void loadFeatures();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const submitImport = async () => {
    if (!layer || !importFile) return;
    setImporting(true);
    try {
      const res = await importLayer(layer.name, importFile, importSrid);
      toast.success(`已导入 ${res.imported} 个要素（${res.format}）`);
      setImportOpen(false);
      setImportFile(null);
      void loadFeatures();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setImporting(false);
    }
  };

  const openEdit = (f: GeoJSONFeature) => {
    setEditing(f);
    setEditText(JSON.stringify(f, null, 2));
  };

  const submitEdit = async () => {
    if (!layer || !editing) return;
    try {
      const parsed = JSON.parse(editText) as GeoJSONFeature;
      await updateFeature(layer.name, Number(editing.id), parsed);
      toast.success("要素已更新");
      setEditing(null);
      void loadFeatures();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const page = Math.floor(offset / PAGE_SIZE) + 1;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">数据浏览</h1>
            <p className="text-sm text-muted-foreground">
              以 GeoJSON 方式查询与编辑图层要素。
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Select
              value={layerName}
              onValueChange={(v) => setParams({ layer: v }, { replace: true })}
            >
              <SelectTrigger className="w-52">
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
            <Button variant="outline" onClick={() => void loadFeatures()}>
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
            </Button>
            <Button disabled={!layer} variant="outline" onClick={() => setExportOpen(true)}>
              <Download className="h-4 w-4" /> 导出
            </Button>
            <Button disabled={!layer || !editable} variant="outline" onClick={() => setImportOpen(true)}>
              <Upload className="h-4 w-4" /> 导入
            </Button>
            <Button disabled={!layer || !editable} onClick={() => setAddOpen(true)}>
              <Plus className="h-4 w-4" /> 新增要素
            </Button>
          </div>
        </div>

        {layer && (
          <Card>
            <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3 space-y-0">
              <CardTitle className="flex items-center gap-2 text-base">
                {layer.title || layer.name}
                <Badge variant="secondary">{layer.geometryType}</Badge>
                <span className="text-sm font-normal text-muted-foreground">
                  共 {total} 个要素
                </span>
              </CardTitle>
              <div className="flex items-center gap-2">
                <Select value={filterField} onValueChange={setFilterField}>
                  <SelectTrigger className="w-40">
                    <SelectValue placeholder="过滤字段" />
                  </SelectTrigger>
                  <SelectContent>
                    {layer.fields.map((f) => (
                      <SelectItem key={f.name} value={f.name}>
                        {f.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Input
                  className="w-44"
                  placeholder="值（支持模糊）"
                  value={filterValue}
                  onChange={(e) => setFilterValue(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && applyFilter()}
                />
                <Button variant="outline" onClick={applyFilter}>
                  过滤
                </Button>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-20">fid</TableHead>
                    {layer.fields.map((f) => (
                      <TableHead key={f.name}>{f.name}</TableHead>
                    ))}
                    <TableHead className="w-32 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {features.map((f) => (
                    <TableRow key={String(f.id)}>
                      <TableCell className="font-mono text-xs">{String(f.id)}</TableCell>
                      {layer.fields.map((field) => (
                        <TableCell key={field.name} className="max-w-[220px] truncate">
                          {formatValue(f.properties?.[field.name])}
                        </TableCell>
                      ))}
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          {editable && (
                            <>
                              <Button variant="ghost" size="sm" onClick={() => openEdit(f)}>
                                编辑
                              </Button>
                              <Button variant="ghost" size="icon" onClick={() => void remove(f)}>
                                <Trash2 className="h-4 w-4 text-destructive" />
                              </Button>
                            </>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                  {features.length === 0 && !loading && (
                    <TableRow>
                      <TableCell
                        colSpan={layer.fields.length + 2}
                        className="py-10 text-center text-sm text-muted-foreground"
                      >
                        没有符合条件的要素
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
            <div className="flex items-center justify-between border-t px-4 py-3 text-sm">
              <span className="text-muted-foreground">
                第 {page} / {pageCount} 页
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset === 0}
                  onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
                >
                  <ChevronLeft className="h-4 w-4" /> 上一页
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset + PAGE_SIZE >= total}
                  onClick={() => setOffset((o) => o + PAGE_SIZE)}
                >
                  下一页 <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </Card>
        )}

        {!layer && !loading && (
          <Card>
            <CardContent className="py-16 text-center text-sm text-muted-foreground">
              请选择或先创建一个图层
            </CardContent>
          </Card>
        )}
      </div>

      {/* 新增要素 */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>新增要素（GeoJSON）</DialogTitle>
          </DialogHeader>
          <div className="grid gap-2">
            <Label>粘贴 Feature 或 FeatureCollection</Label>
            <Textarea
              rows={12}
              className="font-mono text-xs"
              placeholder={`{\n  "type": "Feature",\n  "geometry": { "type": "Point", "coordinates": [116.4, 39.9] },\n  "properties": { "name": "示例" }\n}`}
              value={addText}
              onChange={(e) => setAddText(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label>输入坐标系 (SRID)</Label>
            <CRSInput value={addSrid} onChange={setAddSrid} />
            <p className="text-[11px] text-muted-foreground">
              GeoJSON 规范为 EPSG:4326；若坐标是其它坐标系，请在此选择，后端会自动转换到图层坐标系。
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              取消
            </Button>
            <Button onClick={submitAdd} disabled={!addText.trim()}>
              插入
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 导出 */}
      <Dialog open={exportOpen} onOpenChange={setExportOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>导出「{layer?.title || layer?.name}」</DialogTitle>
          </DialogHeader>
          <div className="grid gap-2 py-1">
            <p className="text-sm text-muted-foreground">
              选择导出格式（新窗口下载；坐标系为 EPSG:4326）。
            </p>
            {[
              { fmt: "geojson", label: "GeoJSON (.geojson)" },
              { fmt: "csv", label: "CSV （含 WKT 几何列）" },
              { fmt: "shapefile", label: "Shapefile (.zip)" },
              { fmt: "gpkg", label: "GeoPackage (.gpkg)" },
            ].map((o) => (
              <a
                key={o.fmt}
                className="flex items-center justify-between rounded-md border px-3 py-2 text-sm hover:bg-accent"
                href={layer ? exportUrl(layer.name, o.fmt) : "#"}
                onClick={() => setExportOpen(false)}
              >
                {o.label}
                <Download className="h-4 w-4 text-muted-foreground" />
              </a>
            ))}
            <a
              className="flex items-center justify-between rounded-md border border-dashed px-3 py-2 text-sm hover:bg-accent"
              href={exportAllUrl("gpkg")}
              onClick={() => setExportOpen(false)}
            >
              全部图层 → GeoPackage（多表）
              <Download className="h-4 w-4 text-muted-foreground" />
            </a>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExportOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 导入 */}
      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>导入到「{layer?.title || layer?.name}」</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 py-1">
            <div className="grid gap-1.5">
              <Label>选择文件</Label>
              <input
                type="file"
                accept=".geojson,.json,.csv,.zip,.gpkg"
                className="block w-full text-sm file:mr-3 file:rounded-md file:border file:border-input file:bg-background file:px-3 file:py-1.5 file:text-sm"
                onChange={(e) => setImportFile(e.target.files?.[0] ?? null)}
              />
              <p className="text-[11px] text-muted-foreground">
                支持 GeoJSON / CSV / Shapefile(.zip) / GeoPackage(.gpkg)，按扩展名自动识别。
              </p>
            </div>
            <div className="grid gap-1.5">
              <Label>文件坐标系 (SRID)</Label>
              <CRSInput value={importSrid} onChange={setImportSrid} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setImportOpen(false)}>
              取消
            </Button>
            <Button onClick={submitImport} disabled={!importFile || importing}>
              {importing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
              导入
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑要素 */}
      <Dialog open={!!editing} onOpenChange={(v) => !v && setEditing(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>编辑要素 #{editing ? String(editing.id) : ""}</DialogTitle>
          </DialogHeader>
          <Textarea
            rows={16}
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

function formatValue(v: unknown): string {
  if (v === null || v === undefined) return "—";
  if (typeof v === "boolean") return v ? "true" : "false";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

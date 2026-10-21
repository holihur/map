import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  ChevronLeft,
  ChevronRight,
  Download,
  Filter,
  Loader2,
  MapPin,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  Upload,
  X,
} from "lucide-react";
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
  analyzeFeatures,
  batchFeatures,
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
import type { FeatureQuery, GeoJSONFeature, Layer } from "@/types";

const PAGE_SIZE = 25;
const PAGE_SIZES = [25, 50, 100];

const OPERATORS = [
  { value: "eq", label: "等于" },
  { value: "neq", label: "不等于" },
  { value: "gt", label: "大于" },
  { value: "gte", label: "大于等于" },
  { value: "lt", label: "小于" },
  { value: "lte", label: "小于等于" },
  { value: "like", label: "包含" },
  { value: "isnull", label: "为空" },
  { value: "notnull", label: "不为空" },
];

interface FilterRow {
  id: number;
  field: string;
  op: string;
  value: string;
}

let filterSeq = 1;

export function DataPage() {
  const { can } = useAuth();
  const editable = can("editor");
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [layers, setLayers] = useState<Layer[]>([]);
  const [features, setFeatures] = useState<GeoJSONFeature[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState(PAGE_SIZE);
  const [loading, setLoading] = useState(false);

  const [filters, setFilters] = useState<FilterRow[]>([]);
  const [textQuery, setTextQuery] = useState("");
  const [appliedFilters, setAppliedFilters] = useState<FilterRow[]>([]);
  const [appliedQuery, setAppliedQuery] = useState("");
  const [orderBy, setOrderBy] = useState("");
  const [order, setOrder] = useState<"asc" | "desc">("asc");
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());

  const [addOpen, setAddOpen] = useState(false);
  const [addText, setAddText] = useState("");
  const [addSrid, setAddSrid] = useState(4326);
  const [importOpen, setImportOpen] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importSrid, setImportSrid] = useState(4326);
  const [importMapping, setImportMapping] = useState("");
  const [importing, setImporting] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [editing, setEditing] = useState<GeoJSONFeature | null>(null);
  const [editText, setEditText] = useState("");
  const [batchOpen, setBatchOpen] = useState(false);
  const [batchField, setBatchField] = useState("");
  const [batchValue, setBatchValue] = useState("");
  const [analyzeOpen, setAnalyzeOpen] = useState(false);
  const [analyzeOp, setAnalyzeOp] = useState<"buffer" | "centroid" | "convexhull" | "simplify" | "envelope">("buffer");
  const [analyzeDistance, setAnalyzeDistance] = useState(0.01);
  const [analyzeTolerance, setAnalyzeTolerance] = useState(0.001);
  const [analyzing, setAnalyzing] = useState(false);

  const layerName = params.get("layer") ?? "";
  const bbox = params.get("bbox") ?? "";
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
          setParams({ ...Object.fromEntries(params), layer: data[0].name }, { replace: true });
        }
      } catch (err) {
        toast.error((err as Error).message);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filterMap = useMemo(() => {
    const out: Record<string, string> = {};
    for (const f of appliedFilters) {
      if (!f.field || f.op === "isnull" || f.op === "notnull") continue;
      if (!f.value && f.op !== "like") continue;
      out[f.op === "eq" ? f.field : `${f.field}.${f.op}`] = f.value;
    }
    for (const f of appliedFilters) {
      if ((f.op === "isnull" || f.op === "notnull") && f.field) {
        out[`${f.field}.${f.op}`] = "1";
      }
    }
    return out;
  }, [appliedFilters]);

  const loadFeatures = useCallback(async () => {
    if (!layerName) return;
    setLoading(true);
    try {
      const query: FeatureQuery = {
        limit: pageSize,
        offset,
        filters: filterMap,
      };
      if (appliedQuery) query.q = appliedQuery;
      if (bbox) query.bbox = bbox;
      if (orderBy) {
        query.orderBy = orderBy;
        query.order = order;
      }
      const data = await listFeatures(layerName, query);
      setFeatures(data.features ?? []);
      setTotal(data.total ?? 0);
      setSelectedIds(new Set());
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [layerName, offset, pageSize, filterMap, appliedQuery, bbox, orderBy, order]);

  useEffect(() => {
    void loadFeatures();
  }, [loadFeatures]);

  useEffect(() => {
    setOffset(0);
    setAppliedFilters([]);
    setAppliedQuery("");
    setFilters([]);
    setTextQuery("");
    setOrderBy("");
    setSelectedIds(new Set());
    setBatchField("");
  }, [layerName]);

  const applyFilter = () => {
    setAppliedFilters(filters.filter((f) => f.field));
    setAppliedQuery(textQuery);
    setOffset(0);
  };

  const clearFilter = () => {
    setFilters([]);
    setTextQuery("");
    setAppliedFilters([]);
    setAppliedQuery("");
    setOffset(0);
    if (bbox) {
      const next = Object.fromEntries(params);
      delete next.bbox;
      setParams(next, { replace: true });
    }
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

  const batchDelete = async () => {
    if (!layer || selectedIds.size === 0) return;
    if (!window.confirm(`删除选中的 ${selectedIds.size} 个要素？`)) return;
    try {
      const n = await batchFeatures(layer.name, {
        action: "delete",
        ids: [...selectedIds],
      });
      toast.success(`已删除 ${n} 个要素`);
      setSelectedIds(new Set());
      void loadFeatures();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const batchUpdate = async () => {
    if (!layer || selectedIds.size === 0 || !batchField) return;
    try {
      const n = await batchFeatures(layer.name, {
        action: "update",
        ids: [...selectedIds],
        properties: { [batchField]: batchValue },
      });
      toast.success(`已更新 ${n} 个要素`);
      setBatchOpen(false);
      setBatchValue("");
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
      let mapping: Record<string, string> | undefined;
      if (importMapping.trim()) {
        try {
          mapping = JSON.parse(importMapping) as Record<string, string>;
        } catch {
          toast.error("字段映射必须是 JSON 对象，例如 {\"src\":\"dest\"}");
          return;
        }
      }
      const res = await importLayer(layer.name, importFile, importSrid, mapping);
      toast.success(`已导入 ${res.imported} 个要素（${res.format}）`);
      if (res.failed) toast.warning(`${res.failed} 行失败`);
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

  const runAnalyze = async () => {
    if (!layer) return;
    setAnalyzing(true);
    try {
      const bboxArr = bbox ? (bbox.split(",").map(Number) as [number, number, number, number]) : undefined;
      const data = await analyzeFeatures(layer.name, {
        op: analyzeOp,
        distance: analyzeDistance,
        tolerance: analyzeTolerance,
        ids: selectedIds.size ? [...selectedIds] : undefined,
        filters: selectedIds.size ? undefined : filterMap,
        bbox: selectedIds.size ? undefined : bboxArr,
        outSrid: 4326,
        limit: 5000,
      });
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/geo+json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${layer.name}-${analyzeOp}.geojson`;
      a.click();
      URL.revokeObjectURL(url);
      toast.success("分析结果已下载");
      setAnalyzeOpen(false);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setAnalyzing(false);
    }
  };

  const toggleSelect = (id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const allSelected = features.length > 0 && features.every((f) => selectedIds.has(Number(f.id)));

  const page = Math.floor(offset / pageSize) + 1;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-7xl space-y-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">数据浏览</h1>
            <p className="text-sm text-muted-foreground">
              查询、筛选、批量编辑与导出图层要素；可与地图双向定位。
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
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
            <Button disabled={!layer} variant="outline" onClick={() => setAnalyzeOpen(true)}>
              空间分析
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
            <CardHeader className="space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  {layer.title || layer.name}
                  <Badge variant="secondary">{layer.geometryType}</Badge>
                  <span className="text-sm font-normal text-muted-foreground">
                    共 {total} 个要素
                  </span>
                  {bbox && (
                    <Badge variant="outline" className="font-mono text-[10px]">
                      框选 {bbox}
                    </Badge>
                  )}
                </CardTitle>
                <div className="flex flex-wrap items-center gap-2">
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                    <Input
                      className="w-48 pl-8"
                      placeholder="全文搜索属性…"
                      value={textQuery}
                      onChange={(e) => setTextQuery(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && applyFilter()}
                    />
                  </div>
                  <Select
                    value={orderBy || "__fid__"}
                    onValueChange={(v) => setOrderBy(v === "__fid__" ? "" : v)}
                  >
                    <SelectTrigger className="w-36">
                      <SelectValue placeholder="排序" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__fid__">按 fid</SelectItem>
                      {layer.fields.map((f) => (
                        <SelectItem key={f.name} value={f.name}>
                          {f.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setOrder((o) => (o === "asc" ? "desc" : "asc"))}
                  >
                    {order === "asc" ? "升序" : "降序"}
                  </Button>
                </div>
              </div>

              <div className="space-y-2 rounded-md border bg-muted/30 p-3">
                <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                  <Filter className="h-3.5 w-3.5" /> 筛选条件（可组合）
                </div>
                {filters.map((row) => (
                  <div key={row.id} className="flex flex-wrap items-center gap-2">
                    <Select
                      value={row.field}
                      onValueChange={(v) =>
                        setFilters((rows) =>
                          rows.map((r) => (r.id === row.id ? { ...r, field: v } : r))
                        )
                      }
                    >
                      <SelectTrigger className="w-36">
                        <SelectValue placeholder="字段" />
                      </SelectTrigger>
                      <SelectContent>
                        {layer.fields.map((f) => (
                          <SelectItem key={f.name} value={f.name}>
                            {f.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Select
                      value={row.op}
                      onValueChange={(v) =>
                        setFilters((rows) =>
                          rows.map((r) => (r.id === row.id ? { ...r, op: v } : r))
                        )
                      }
                    >
                      <SelectTrigger className="w-32">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {OPERATORS.map((o) => (
                          <SelectItem key={o.value} value={o.value}>
                            {o.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Input
                      className="w-44"
                      placeholder="值"
                      disabled={row.op === "isnull" || row.op === "notnull"}
                      value={row.value}
                      onChange={(e) =>
                        setFilters((rows) =>
                          rows.map((r) => (r.id === row.id ? { ...r, value: e.target.value } : r))
                        )
                      }
                      onKeyDown={(e) => e.key === "Enter" && applyFilter()}
                    />
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setFilters((rows) => rows.filter((r) => r.id !== row.id))}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      setFilters((rows) => [
                        ...rows,
                        {
                          id: filterSeq++,
                          field: layer.fields[0]?.name ?? "",
                          op: "eq",
                          value: "",
                        },
                      ])
                    }
                  >
                    <Plus className="h-3.5 w-3.5" /> 添加条件
                  </Button>
                  <Button size="sm" onClick={applyFilter}>
                    应用筛选
                  </Button>
                  {(appliedFilters.length > 0 || appliedQuery || bbox) && (
                    <Button size="sm" variant="ghost" onClick={clearFilter}>
                      清除
                    </Button>
                  )}
                </div>
              </div>
            </CardHeader>

            {selectedIds.size > 0 && editable && (
              <div className="flex flex-wrap items-center gap-2 border-y bg-amber-50 px-4 py-2 text-sm dark:bg-amber-950/30">
                <span className="font-medium">已选 {selectedIds.size} 项</span>
                <Button size="sm" variant="outline" onClick={() => setBatchOpen(true)}>
                  批量修改字段
                </Button>
                <Button size="sm" variant="ghost" onClick={() => void batchDelete()}>
                  <Trash2 className="h-4 w-4 text-destructive" /> 批量删除
                </Button>
                <Button size="sm" variant="outline" onClick={() => setAnalyzeOpen(true)}>
                  分析选中要素
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setSelectedIds(new Set())}>
                  取消选择
                </Button>
              </div>
            )}

            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10">
                        <input
                          type="checkbox"
                          checked={allSelected}
                          onChange={(e) =>
                            setSelectedIds(
                              e.target.checked
                                ? new Set(features.map((f) => Number(f.id)))
                                : new Set()
                            )
                          }
                        />
                      </TableHead>
                      <TableHead className="w-20">fid</TableHead>
                      {layer.fields.map((f) => (
                        <TableHead key={f.name}>{f.name}</TableHead>
                      ))}
                      <TableHead className="w-40 text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {features.map((f) => (
                      <TableRow key={String(f.id)} className="hover:bg-accent/50">
                        <TableCell>
                          <input
                            type="checkbox"
                            checked={selectedIds.has(Number(f.id))}
                            onChange={() => toggleSelect(Number(f.id))}
                          />
                        </TableCell>
                        <TableCell className="font-mono text-xs">{String(f.id)}</TableCell>
                        {layer.fields.map((field) => (
                          <TableCell key={field.name} className="max-w-[220px] truncate">
                            {formatValue(f.properties?.[field.name])}
                          </TableCell>
                        ))}
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              title="在地图中查看"
                              onClick={() =>
                                navigate(
                                  `/map?layer=${encodeURIComponent(layer.name)}&fid=${f.id}`
                                )
                              }
                            >
                              <MapPin className="h-4 w-4" />
                            </Button>
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
                          colSpan={layer.fields.length + 3}
                          className="py-10 text-center text-sm text-muted-foreground"
                        >
                          没有符合条件的要素
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
            <div className="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-3 text-sm">
              <span className="text-muted-foreground">
                第 {page} / {pageCount} 页 · 每页
                <Select
                  value={String(pageSize)}
                  onValueChange={(v) => {
                    setPageSize(Number(v));
                    setOffset(0);
                  }}
                >
                  <SelectTrigger className="ml-1 inline-flex h-7 w-20">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PAGE_SIZES.map((s) => (
                      <SelectItem key={s} value={String(s)}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset === 0}
                  onClick={() => setOffset((o) => Math.max(0, o - pageSize))}
                >
                  <ChevronLeft className="h-4 w-4" /> 上一页
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset + pageSize >= total}
                  onClick={() => setOffset((o) => o + pageSize)}
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

      {/* 导出（带当前筛选条件） */}
      <Dialog open={exportOpen} onOpenChange={setExportOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>导出「{layer?.title || layer?.name}」</DialogTitle>
          </DialogHeader>
          <div className="grid gap-2 py-1">
            <p className="text-sm text-muted-foreground">
              导出会应用当前筛选条件{bbox ? "与框选范围" : ""}（坐标系为 EPSG:4326）。
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
                href={
                  layer
                    ? exportUrl(layer.name, o.fmt, {
                        filters: filterMap,
                        bbox: bbox || undefined,
                        query: appliedQuery || undefined,
                      })
                    : "#"
                }
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
            <div className="grid gap-1.5">
              <Label>字段映射（可选，JSON）</Label>
              <Textarea
                rows={2}
                className="font-mono text-xs"
                placeholder={'{"源字段":"目标字段"}'}
                value={importMapping}
                onChange={(e) => setImportMapping(e.target.value)}
              />
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

      {/* 批量修改 */}
      <Dialog open={batchOpen} onOpenChange={setBatchOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>批量修改 {selectedIds.size} 个要素</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 py-1">
            <div className="grid gap-1.5">
              <Label>字段</Label>
              <Select value={batchField} onValueChange={setBatchField}>
                <SelectTrigger>
                  <SelectValue placeholder="选择字段" />
                </SelectTrigger>
                <SelectContent>
                  {layer?.fields.map((f) => (
                    <SelectItem key={f.name} value={f.name}>
                      {f.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label>新值</Label>
              <Input value={batchValue} onChange={(e) => setBatchValue(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBatchOpen(false)}>
              取消
            </Button>
            <Button onClick={() => void batchUpdate()} disabled={!batchField}>
              更新
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 空间分析 */}
      <Dialog open={analyzeOpen} onOpenChange={setAnalyzeOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>空间分析</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 py-1">
            <p className="text-xs text-muted-foreground">
              {selectedIds.size > 0
                ? `对选中的 ${selectedIds.size} 个要素进行分析`
                : "对当前筛选 / 框选结果进行分析"}
              ，结果为 GeoJSON 文件（EPSG:4326，不写入数据库）。
            </p>
            <div className="grid gap-1.5">
              <Label>操作</Label>
              <Select
                value={analyzeOp}
                onValueChange={(v) => setAnalyzeOp(v as typeof analyzeOp)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="buffer">缓冲区（Buffer）</SelectItem>
                  <SelectItem value="centroid">质心（Centroid）</SelectItem>
                  <SelectItem value="convexhull">凸包（Convex Hull）</SelectItem>
                  <SelectItem value="envelope">外包矩形（Envelope）</SelectItem>
                  <SelectItem value="simplify">简化（Simplify）</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {analyzeOp === "buffer" && (
              <div className="grid gap-1.5">
                <Label>缓冲距离（图层坐标系单位）</Label>
                <Input
                  type="number"
                  step="0.001"
                  value={analyzeDistance}
                  onChange={(e) => setAnalyzeDistance(Number(e.target.value))}
                />
              </div>
            )}
            {analyzeOp === "simplify" && (
              <div className="grid gap-1.5">
                <Label>简化容差（图层坐标系单位）</Label>
                <Input
                  type="number"
                  step="0.0001"
                  value={analyzeTolerance}
                  onChange={(e) => setAnalyzeTolerance(Number(e.target.value))}
                />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAnalyzeOpen(false)}>
              取消
            </Button>
            <Button onClick={() => void runAnalyze()} disabled={analyzing}>
              {analyzing ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              生成并下载
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

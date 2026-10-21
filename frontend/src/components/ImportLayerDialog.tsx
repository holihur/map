import { useState } from "react";
import { FileUp, Loader2, Upload } from "lucide-react";
import { toast } from "sonner";

import { CRSInput } from "@/components/CRSInput";
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
import { importNewLayer, previewImportLayer, type ImportPreview } from "@/lib/api";
import type { Layer } from "@/types";

export function ImportLayerDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onCreated: (layer: Layer) => void;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [srid, setSrid] = useState(4326);
  const [name, setName] = useState("");
  const [title, setTitle] = useState("");
  const [table, setTable] = useState("");
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [dragOver, setDragOver] = useState(false);

  const pickFile = async (f: File | null) => {
    setFile(f);
    setPreview(null);
    if (!f) return;
    setLoading(true);
    try {
      const p = await previewImportLayer(f, srid, name);
      setPreview(p);
      if (!name) setName(p.name);
      if (!title) setTitle(p.name);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const rePreview = async (nextSrid: number) => {
    setSrid(nextSrid);
    if (!file) return;
    setLoading(true);
    try {
      setPreview(await previewImportLayer(file, nextSrid, name));
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const submit = async () => {
    if (!file) return;
    setImporting(true);
    try {
      const res = await importNewLayer(file, { srid, name, title, table: table || undefined });
      toast.success(`已创建图层并导入 ${res.imported} 个要素`);
      if (res.failed > 0) toast.warning(`${res.failed} 行导入失败`);
      onCreated(res.layer);
      onOpenChange(false);
      setFile(null);
      setPreview(null);
      setName("");
      setTitle("");
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setImporting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>上传数据并自动建层</DialogTitle>
          <DialogDescription>
            支持 GeoJSON / CSV / Shapefile(zip) / GeoPackage；自动推断几何类型与字段类型，确认后一次完成建层和导入。
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-1">
          <div
            className={`flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed p-6 text-center transition ${
              dragOver ? "border-primary bg-primary/5" : "border-border"
            }`}
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDragOver(false);
              void pickFile(e.dataTransfer.files?.[0] ?? null);
            }}
          >
            <FileUp className="h-8 w-8 text-muted-foreground" />
            <p className="text-sm">拖拽文件到这里，或</p>
            <label className="cursor-pointer text-sm text-primary underline">
              选择文件
              <input
                type="file"
                className="hidden"
                accept=".geojson,.json,.csv,.zip,.gpkg"
                onChange={(e) => void pickFile(e.target.files?.[0] ?? null)}
              />
            </label>
            {file && (
              <p className="font-mono text-xs text-muted-foreground">
                {file.name} · {(file.size / 1024).toFixed(1)} KB
              </p>
            )}
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label>图层名称</Label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="自动生成"
              />
            </div>
            <div className="grid gap-1.5">
              <Label>标题</Label>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} />
            </div>
            <div className="grid gap-1.5">
              <Label>文件坐标系 (SRID)</Label>
              <CRSInput value={srid} onChange={(n) => void rePreview(n)} />
            </div>
            <div className="grid gap-1.5">
              <Label>GeoPackage 表名（可选）</Label>
              <Input
                value={table}
                onChange={(e) => setTable(e.target.value)}
                placeholder="默认第一张表"
              />
            </div>
          </div>

          {loading && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" /> 正在解析文件并推断字段…
            </div>
          )}

          {preview && (
            <div className="space-y-3 rounded-md border p-3">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <Badge variant="secondary">{preview.format}</Badge>
                <Badge variant="outline">{preview.geometryType}</Badge>
                <Badge variant="outline">EPSG:{preview.srid}</Badge>
                <span className="text-muted-foreground">{preview.count} 个要素</span>
              </div>
              <div>
                <div className="mb-1 text-xs font-medium text-muted-foreground">推断字段</div>
                <div className="flex flex-wrap gap-1">
                  {preview.fields.map((f) => (
                    <Badge key={f.name} variant="outline" className="font-mono text-[10px]">
                      {f.name}:{f.type}
                    </Badge>
                  ))}
                </div>
              </div>
              {preview.sample.length > 0 && (
                <div>
                  <div className="mb-1 text-xs font-medium text-muted-foreground">样例</div>
                  <pre className="max-h-32 overflow-auto rounded bg-muted p-2 text-[11px]">
                    {JSON.stringify(preview.sample, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => void submit()} disabled={!file || !preview || importing}>
            {importing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
            创建并导入
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

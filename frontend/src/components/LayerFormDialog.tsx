import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { CRSInput } from "@/components/CRSInput";
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
import { Textarea } from "@/components/ui/textarea";
import { createLayer } from "@/lib/api";
import type { FieldType, Layer } from "@/types";

const FIELD_TYPES: FieldType[] = [
  "text",
  "integer",
  "bigint",
  "double",
  "boolean",
  "timestamptz",
  "date",
];

const GEOMETRY_TYPES = [
  "POINT",
  "LINESTRING",
  "POLYGON",
  "MULTIPOINT",
  "MULTILINESTRING",
  "MULTIPOLYGON",
  "GEOMETRY",
];

interface FieldRow {
  name: string;
  type: FieldType;
}

export function LayerFormDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onCreated: (layer: Layer) => void;
}) {
  const [name, setName] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [geometryType, setGeometryType] = useState("POINT");
  const [srid, setSrid] = useState(4326);
  const [color, setColor] = useState("#2563eb");
  const [fields, setFields] = useState<FieldRow[]>([{ name: "name", type: "text" }]);
  const [saving, setSaving] = useState(false);

  const reset = () => {
    setName("");
    setTitle("");
    setDescription("");
    setGeometryType("POINT");
    setSrid(4326);
    setColor("#2563eb");
    setFields([{ name: "name", type: "text" }]);
  };

  const submit = async () => {
    if (!name.trim()) {
      toast.error("请填写图层名称");
      return;
    }
    setSaving(true);
    try {
      const style =
        geometryType.includes("POINT")
          ? {
              type: "circle" as const,
              paint: {
                "circle-radius": 5,
                "circle-color": color,
                "circle-stroke-color": "#ffffff",
                "circle-stroke-width": 1.5,
              },
            }
          : geometryType.includes("LINE")
            ? { type: "line" as const, paint: { "line-color": color, "line-width": 2 } }
            : {
                type: "fill" as const,
                paint: {
                  "fill-color": color,
                  "fill-opacity": 0.5,
                  "fill-outline-color": color,
                },
              };

      const layer = await createLayer({
        name: name.trim(),
        title: title.trim() || name.trim(),
        description: description.trim(),
        geometryType,
        srid: srid || 4326,
        fields: fields.filter((f) => f.name.trim()),
        style,
      });
      toast.success(`图层 ${layer.name} 已创建`);
      onCreated(layer);
      reset();
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
          <DialogTitle>新建图层</DialogTitle>
          <DialogDescription>
            将创建元数据记录与物理空间表（layer_&lt;name&gt;）并建立 GiST 索引。
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-2">
              <Label htmlFor="layer-name">名称（英文标识）</Label>
              <Input
                id="layer-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="roads"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="layer-title">标题</Label>
              <Input
                id="layer-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="道路"
              />
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="layer-desc">描述</Label>
            <Textarea
              id="layer-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
            />
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div className="grid gap-2">
              <Label>几何类型</Label>
              <Select value={geometryType} onValueChange={setGeometryType}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {GEOMETRY_TYPES.map((g) => (
                    <SelectItem key={g} value={g}>
                      {g}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="layer-srid">坐标系 (SRID)</Label>
              <CRSInput id="layer-srid" value={srid} onChange={setSrid} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="layer-color">颜色</Label>
              <Input
                id="layer-color"
                type="color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                className="h-9 p-1"
              />
            </div>
          </div>

          <div className="grid gap-2">
            <div className="flex items-center justify-between">
              <Label>属性字段</Label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setFields((f) => [...f, { name: "", type: "text" }])}
              >
                <Plus className="h-3.5 w-3.5" /> 添加字段
              </Button>
            </div>
            <div className="space-y-2">
              {fields.map((f, i) => (
                <div key={i} className="flex items-center gap-2">
                  <Input
                    value={f.name}
                    placeholder="字段名"
                    onChange={(e) =>
                      setFields((rows) =>
                        rows.map((r, ri) =>
                          ri === i ? { ...r, name: e.target.value } : r
                        )
                      )
                    }
                  />
                  <Select
                    value={f.type}
                    onValueChange={(v) =>
                      setFields((rows) =>
                        rows.map((r, ri) =>
                          ri === i ? { ...r, type: v as FieldType } : r
                        )
                      )
                    }
                  >
                    <SelectTrigger className="w-40">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {FIELD_TYPES.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={() => setFields((rows) => rows.filter((_, ri) => ri !== i))}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={submit} disabled={saving}>
            {saving ? "创建中…" : "创建图层"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

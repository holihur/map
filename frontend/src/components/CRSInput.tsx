import { useEffect, useId, useState } from "react";

import { Input } from "@/components/ui/input";
import { listCRS } from "@/lib/api";
import type { CRS } from "@/types";

/**
 * 坐标参考系选择输入框：可直接输入 EPSG 代号，或从 PostGIS 的
 * spatial_ref_sys 列表中搜索选择（使用原生 datalist 支持输入过滤）。
 */
export function CRSInput({
  value,
  onChange,
  id,
}: {
  value: number;
  onChange: (srid: number) => void;
  id?: string;
}) {
  const [list, setList] = useState<CRS[]>([]);
  const [text, setText] = useState(String(value || ""));
  const reactId = useId();
  const listId = id ?? `crs-list-${reactId}`;

  useEffect(() => {
    listCRS("", 300)
      .then(setList)
      .catch(() => {
        /* 忽略：可手动输入 SRID */
      });
  }, []);

  useEffect(() => {
    setText(String(value || ""));
  }, [value]);

  const handleChange = (v: string) => {
    setText(v);
    const n = Number(v);
    if (Number.isFinite(n) && n > 0) onChange(n);
  };

  const current = list.find((c) => c.srid === value);

  return (
    <>
      <Input
        list={listId}
        inputMode="numeric"
        value={text}
        onChange={(e) => handleChange(e.target.value)}
        placeholder="4326"
      />
      <datalist id={listId}>
        {list.map((c) => (
          <option key={c.srid} value={c.srid}>
            {`${c.authName}:${c.authSrid} ${c.name}`}
          </option>
        ))}
      </datalist>
      <p className="text-[11px] text-muted-foreground">
        {current
          ? `${current.authName}:${current.authSrid} · ${current.name}`
          : "EPSG:4326 · WGS 84（默认）"}
      </p>
    </>
  );
}

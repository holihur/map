import { API_BASE, getAuthToken } from "@/lib/api";
import type { ExternalLayer, Layer, LayerStyle } from "@/types";

export const RASTER_SOURCE_TYPES = ["xyz", "wms", "wmts", "arcgis"] as const;

export function isRasterSource(type: string | undefined): boolean {
  return !!type && (RASTER_SOURCE_TYPES as readonly string[]).includes(type);
}

export function sourceLayerOf(l: Layer): string {
  const v = (l.sourceOptions ?? {}).sourceLayer;
  return typeof v === "string" && v ? v : l.name;
}

/** 把第三方图层转换成地图渲染使用的统一 Layer 结构。 */
export function externalToLayer(e: ExternalLayer): Layer {
  const style = (e.style ?? {}) as LayerStyle;
  const type = String((style as any).type ?? "fill");
  const geometryType = isRasterSource(e.sourceType)
    ? "RASTER"
    : type === "line"
      ? "LINESTRING"
      : type === "circle" || type === "heatmap" || type === "symbol"
        ? "POINT"
        : "POLYGON";
  const bounds = Array.isArray((e.metadata as any)?.bounds)
    ? ((e.metadata as any).bounds as [number, number, number, number])
    : undefined;
  return {
    id: -e.id,
    name: e.name,
    title: e.title || e.name,
    description: e.description,
    geometryType,
    srid: 4326,
    tableName: "",
    fields: [],
    minZoom: e.minZoom,
    maxZoom: e.maxZoom,
    bounds,
    style,
    metadata: e.metadata ?? {},
    sortOrder: e.sortOrder,
    visibility: e.visibility,
    featureCount: 0,
    createdAt: e.createdAt,
    updatedAt: e.updatedAt,
    external: true,
    sourceType: e.sourceType,
    sourceUrl: e.sourceUrl,
    sourceOptions: e.sourceOptions ?? {},
  };
}

export function mergeLayers(base: Layer[], external: ExternalLayer[]): Layer[] {
  const out = [...base, ...external.map(externalToLayer)];
  out.sort((a, b) => {
    if (a.sortOrder !== b.sortOrder) return a.sortOrder - b.sortOrder;
    return a.name.localeCompare(b.name);
  });
  return out;
}

/** 第三方瓦片地址：默认走服务端代理，解决浏览器 CORS 限制。 */
export function externalTileUrl(l: Layer): string {
  const opts = (l.sourceOptions ?? {}) as Record<string, unknown>;
  if (opts.proxy === false) return l.sourceUrl ?? "";
  const token = getAuthToken();
  const suffix = token ? `?access_token=${encodeURIComponent(token)}` : "";
  return `${API_BASE}/api/external-layers/${encodeURIComponent(l.name)}/tile/{z}/{x}/{y}${suffix}`;
}

/** 第三方 GeoJSON 数据地址。 */
export function externalDataUrl(l: Layer): string {
  const opts = (l.sourceOptions ?? {}) as Record<string, unknown>;
  if (opts.proxy === false) return l.sourceUrl ?? "";
  const token = getAuthToken();
  const suffix = token ? `?access_token=${encodeURIComponent(token)}` : "";
  return `${API_BASE}/api/external-layers/${encodeURIComponent(l.name)}/data${suffix}`;
}

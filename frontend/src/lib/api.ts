import type {
  CRS,
  Capabilities,
  CreateLayerInput,
  FeatureCollection,
  FeatureQuery,
  GeoJSONFeature,
  Layer,
  MapStyle,
  TileMatrixSet,
} from "@/types";

export const API_BASE = import.meta.env.VITE_API_BASE ?? "";

/** 用于界面展示的 API 基地址（空表示同源 / 开发代理）。 */
export function apiBaseLabel(): string {
  return API_BASE;
}

// ── Token 管理 ──────────────────────────────────────────────────

const TOKEN_KEY = "mapserver_token";

let authToken: string | null =
  typeof localStorage !== "undefined" ? localStorage.getItem(TOKEN_KEY) : null;

/** 设置（或清除）当前访问令牌，并持久化到 localStorage。 */
export function setAuthToken(token: string | null): void {
  authToken = token;
  if (typeof localStorage === "undefined") return;
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

export function getAuthToken(): string | null {
  return authToken;
}

/** 当收到 401 时触发，供 AuthProvider 监听并登出。 */
export const UNAUTHORIZED_EVENT = "mapserver:unauthorized";

export type Role = "admin" | "editor" | "viewer";

export interface AuthUser {
  id: number;
  username: string;
  role: Role;
  createdAt: string;
  updatedAt: string;
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    ...((init?.headers as Record<string, string>) || {}),
  };
  // FormData 由浏览器自动设置 multipart 边界
  if (!(init?.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }
  if (authToken) headers.Authorization = `Bearer ${authToken}`;

  const res = await fetch(`${API_BASE}${path}`, { ...init, headers });
  if (res.status === 401 && !path.startsWith("/api/auth/login")) {
    if (typeof window !== "undefined") {
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
    }
  }
  if (!res.ok) {
    let message = `请求失败 (${res.status})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  return text ? (JSON.parse(text) as T) : (undefined as T);
}

// ── 鉴权与用户 ──────────────────────────────────────────────────

export async function login(
  username: string,
  password: string
): Promise<{ token: string; user: AuthUser }> {
  const data = await request<{ token: string; user: AuthUser }>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
  setAuthToken(data.token);
  return data;
}

export function logout(): void {
  setAuthToken(null);
}

export async function fetchMe(): Promise<AuthUser> {
  return request<AuthUser>("/api/auth/me");
}

export async function listUsers(): Promise<AuthUser[]> {
  const data = await request<{ users: AuthUser[] }>("/api/users");
  return data.users ?? [];
}

export async function createUser(input: {
  username: string;
  password: string;
  role: Role;
}): Promise<AuthUser> {
  return request<AuthUser>("/api/users", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateUser(
  id: number,
  patch: { password?: string; role?: Role }
): Promise<AuthUser> {
  return request<AuthUser>(`/api/users/${id}`, {
    method: "PUT",
    body: JSON.stringify(patch),
  });
}

export async function deleteUser(id: number): Promise<void> {
  await request<void>(`/api/users/${id}`, { method: "DELETE" });
}

// ── 运行统计 ────────────────────────────────────────────────────

export interface CacheStats {
  hits: number;
  misses: number;
  size: number;
  evicted: number;
  max?: number;
}

export interface ServerStats {
  tileCache: CacheStats;
  metaCache: CacheStats;
  diskCache: { enabled: boolean; dir: string; files: number; bytes: number };
}

export async function getStats(): Promise<ServerStats> {
  return request<ServerStats>("/api/stats");
}

export async function purgeCache(): Promise<void> {
  await request<void>("/api/cache/purge", { method: "POST" });
}

/** 矢量瓦片 URL 模板（供 MapLibre 使用）。 */
export function tileUrlTemplate(layerName: string): string {
  return `${API_BASE}/api/tiles/${layerName}/{z}/{x}/{y}.mvt`;
}

/** 指定瓦片矩阵集的矢量瓦片 URL 模板。 */
export function tileUrlTemplateTMS(layerName: string, tms: string): string {
  return `${API_BASE}/api/tiles/${layerName}/${tms}/{z}/{x}/{y}.mvt`;
}

/** 栅格 PNG 瓦片 URL 模板（供 OpenLayers / XYZ 使用）。 */
export function rasterTileUrlTemplate(layerName: string, tms?: string): string {
  return tms
    ? `${API_BASE}/api/tiles/${layerName}/${tms}/{z}/{x}/{y}.png`
    : `${API_BASE}/api/tiles/${layerName}/{z}/{x}/{y}.png`;
}

/** WMTS GetCapabilities 地址。 */
export function wmtsCapabilitiesUrl(): string {
  return `${API_BASE}/api/wmts?SERVICE=WMTS&REQUEST=GetCapabilities`;
}

/** WFS GetCapabilities 地址。 */
export function wfsCapabilitiesUrl(): string {
  return `${API_BASE}/api/wfs?SERVICE=WFS&REQUEST=GetCapabilities`;
}

/** 图层数据导出地址。 */
export function exportUrl(layerName: string, format: string, outSrid?: number): string {
  const params = new URLSearchParams({ format });
  if (outSrid) params.set("outSrid", String(outSrid));
  return `${API_BASE}/api/layers/${encodeURIComponent(layerName)}/export?${params.toString()}`;
}

/** 全部图层导出为一个 GeoPackage。 */
export function exportAllUrl(format = "gpkg"): string {
  return `${API_BASE}/api/export?format=${format}`;
}

/** Prometheus 指标地址。 */
export function metricsUrl(): string {
  return `${API_BASE}/metrics`;
}

/** 上传 SLD 文档到样式库。 */
export async function uploadSLD(
  name: string,
  xml: string,
  isDefault = false
): Promise<MapStyle> {
  const params = new URLSearchParams({ name, default: String(isDefault) });
  return request<MapStyle>(`/api/styles/sld?${params.toString()}`, {
    method: "POST",
    body: xml,
    headers: { "Content-Type": "application/xml" },
  });
}

/** 上传文件导入图层要素。 */
export async function importLayer(
  layerName: string,
  file: File,
  srid = 4326
): Promise<{ imported: number; format: string; layer: string }> {
  const fd = new FormData();
  fd.append("file", file);
  fd.append("srid", String(srid));
  return request(`/api/layers/${encodeURIComponent(layerName)}/import`, {
    method: "POST",
    body: fd,
  });
}

/** WMS GetFeatureInfo 地址。 */
export function wmsGetFeatureInfoUrl(opts: {
  layer: string;
  srid: number;
  bbox: [number, number, number, number];
  width: number;
  height: number;
  i: number;
  j: number;
  swapAxes?: boolean;
}): string {
  const [minx, miny, maxx, maxy] = opts.bbox;
  const bbox = opts.swapAxes ? [miny, minx, maxy, maxx] : [minx, miny, maxx, maxy];
  const params = new URLSearchParams({
    SERVICE: "WMS",
    VERSION: "1.3.0",
    REQUEST: "GetFeatureInfo",
    QUERY_LAYERS: opts.layer,
    LAYERS: opts.layer,
    CRS: `EPSG:${opts.srid}`,
    BBOX: bbox.join(","),
    WIDTH: String(opts.width),
    HEIGHT: String(opts.height),
    I: String(opts.i),
    J: String(opts.j),
    INFO_FORMAT: "application/json",
  });
  return `${API_BASE}/api/wms?${params.toString()}`;
}

// ── 图层 ────────────────────────────────────────────────────────

export async function listLayers(): Promise<Layer[]> {
  const data = await request<{ layers: Layer[] }>("/api/layers");
  return data.layers ?? [];
}

export async function getLayer(name: string): Promise<Layer> {
  return request<Layer>(`/api/layers/${encodeURIComponent(name)}`);
}

export async function createLayer(input: CreateLayerInput): Promise<Layer> {
  return request<Layer>("/api/layers", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateLayer(
  name: string,
  patch: Partial<CreateLayerInput>
): Promise<Layer> {
  return request<Layer>(`/api/layers/${encodeURIComponent(name)}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export async function deleteLayer(name: string): Promise<void> {
  await request<void>(`/api/layers/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export async function layerBounds(
  name: string
): Promise<[number, number, number, number] | null> {
  const data = await request<{ bounds: [number, number, number, number] | null }>(
    `/api/layers/${encodeURIComponent(name)}/bounds`
  );
  return data.bounds;
}

// ── 要素 ────────────────────────────────────────────────────────

export async function listFeatures(
  layerName: string,
  query: FeatureQuery = {}
): Promise<FeatureCollection> {
  const params = new URLSearchParams();
  if (query.bbox) params.set("bbox", query.bbox);
  if (query.bboxSrid != null) params.set("bboxSrid", String(query.bboxSrid));
  if (query.limit != null) params.set("limit", String(query.limit));
  if (query.offset != null) params.set("offset", String(query.offset));
  if (query.outSrid != null) params.set("outSrid", String(query.outSrid));
  if (query.simplify != null) params.set("simplify", String(query.simplify));
  if (query.orderBy) params.set("orderBy", query.orderBy);
  if (query.order) params.set("order", query.order);
  for (const [k, v] of Object.entries(query.filters ?? {})) {
    params.set(`f.${k}`, v);
  }
  const qs = params.toString();
  return request<FeatureCollection>(
    `/api/layers/${encodeURIComponent(layerName)}/features${qs ? `?${qs}` : ""}`
  );
}

export async function getFeature(
  layerName: string,
  fid: number,
  outSrid?: number
): Promise<GeoJSONFeature> {
  const qs = outSrid ? `?outSrid=${outSrid}` : "";
  return request<GeoJSONFeature>(
    `/api/layers/${encodeURIComponent(layerName)}/features/${fid}${qs}`
  );
}

export async function createFeatures(
  layerName: string,
  payload: GeoJSONFeature | { type: "FeatureCollection"; features: GeoJSONFeature[] },
  inputSrid?: number
): Promise<{ ids: number[]; count: number }> {
  const qs = inputSrid ? `?inputSrid=${inputSrid}` : "";
  return request(`/api/layers/${encodeURIComponent(layerName)}/features${qs}`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateFeature(
  layerName: string,
  fid: number,
  feature: GeoJSONFeature,
  inputSrid?: number
): Promise<GeoJSONFeature> {
  const qs = inputSrid ? `?inputSrid=${inputSrid}` : "";
  return request<GeoJSONFeature>(
    `/api/layers/${encodeURIComponent(layerName)}/features/${fid}${qs}`,
    { method: "PUT", body: JSON.stringify(feature) }
  );
}

export async function deleteFeature(
  layerName: string,
  fid: number
): Promise<void> {
  await request<void>(
    `/api/layers/${encodeURIComponent(layerName)}/features/${fid}`,
    { method: "DELETE" }
  );
}

// ── 样式 ────────────────────────────────────────────────────────

export async function listStyles(): Promise<MapStyle[]> {
  const data = await request<{ styles: MapStyle[] }>("/api/styles");
  return data.styles ?? [];
}

export async function createStyle(input: {
  name: string;
  isDefault: boolean;
  style: Record<string, unknown>;
}): Promise<MapStyle> {
  return request<MapStyle>("/api/styles", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateStyle(
  id: number,
  input: { name: string; isDefault: boolean; style: Record<string, unknown> }
): Promise<MapStyle> {
  return request<MapStyle>(`/api/styles/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export async function deleteStyle(id: number): Promise<void> {
  await request<void>(`/api/styles/${id}`, { method: "DELETE" });
}

export async function capabilities(): Promise<Capabilities> {
  return request<Capabilities>("/api/capabilities");
}

// ── 坐标参考系（CRS）───────────────────────────────────────────

export async function listCRS(query = "", limit = 50): Promise<CRS[]> {
  const params = new URLSearchParams();
  if (query) params.set("q", query);
  params.set("limit", String(limit));
  const data = await request<{ crs: CRS[] }>(`/api/crs?${params.toString()}`);
  return data.crs ?? [];
}

/** 将图层所有几何转换到新的坐标系。 */
export async function transformLayer(layerName: string, srid: number): Promise<Layer> {
  return request<Layer>(`/api/layers/${encodeURIComponent(layerName)}/transform`, {
    method: "POST",
    body: JSON.stringify({ srid }),
  });
}

// ── 瓦片矩阵集 / WMS ────────────────────────────────────────────

export async function listTileMatrixSets(): Promise<TileMatrixSet[]> {
  const data = await request<{ tileMatrixSets: TileMatrixSet[] }>("/api/tilematrixsets");
  return data.tileMatrixSets ?? [];
}

/** 构造 WMS GetCapabilities 地址。 */
export function wmsCapabilitiesUrl(): string {
  return `${API_BASE}/api/wms?SERVICE=WMS&REQUEST=GetCapabilities`;
}

/** 构造 WMS GetMap 地址。 */
export function wmsGetMapUrl(opts: {
  layer: string;
  srid: number;
  bbox: [number, number, number, number];
  width: number;
  height: number;
  transparent?: boolean;
  /** WMS 1.3.0 中 EPSG:4326 使用 lat,lon 轴顺序 */
  swapAxes?: boolean;
}): string {
  const [minx, miny, maxx, maxy] = opts.bbox;
  const bbox = opts.swapAxes ? [miny, minx, maxy, maxx] : [minx, miny, maxx, maxy];
  const params = new URLSearchParams({
    SERVICE: "WMS",
    VERSION: "1.3.0",
    REQUEST: "GetMap",
    LAYERS: opts.layer,
    CRS: `EPSG:${opts.srid}`,
    BBOX: bbox.join(","),
    WIDTH: String(opts.width),
    HEIGHT: String(opts.height),
    FORMAT: "image/png",
    TRANSPARENT: opts.transparent === false ? "FALSE" : "TRUE",
  });
  return `${API_BASE}/api/wms?${params.toString()}`;
}

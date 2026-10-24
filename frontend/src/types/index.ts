export type FieldType =
  | "text"
  | "integer"
  | "bigint"
  | "double"
  | "boolean"
  | "timestamptz"
  | "date";

export interface LayerField {
  name: string;
  type: FieldType;
  nullable?: boolean;
}

export interface LayerStyle {
  type?: "fill" | "line" | "circle" | "symbol" | "raster" | "heatmap";
  paint?: Record<string, unknown>;
  layout?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface Layer {
  id: number;
  name: string;
  title: string;
  description: string;
  geometryType: string;
  srid: number;
  tableName: string;
  fields: LayerField[];
  minZoom: number;
  maxZoom: number;
  bounds?: [number, number, number, number];
  style: LayerStyle;
  metadata: Record<string, unknown>;
  sortOrder: number;
  visibility: "public" | "private";
  featureCount: number;
  createdAt: string;
  updatedAt: string;
  /** 第三方图层（不落 PostGIS）。 */
  external?: boolean;
  sourceType?: ExternalSourceType;
  sourceUrl?: string;
  sourceOptions?: Record<string, unknown>;
}

export type ExternalSourceType = "xyz" | "wms" | "wmts" | "mvt" | "geojson" | "arcgis";

export interface ExternalLayer {
  id: number;
  name: string;
  title: string;
  description: string;
  sourceType: ExternalSourceType;
  sourceUrl: string;
  sourceOptions: Record<string, unknown>;
  style: LayerStyle;
  metadata: Record<string, unknown>;
  minZoom: number;
  maxZoom: number;
  sortOrder: number;
  visibility: "public" | "private";
  createdAt: string;
  updatedAt: string;
}

export interface CreateExternalLayerInput {
  name: string;
  title?: string;
  description?: string;
  sourceType: ExternalSourceType;
  sourceUrl: string;
  sourceOptions?: Record<string, unknown>;
  style?: LayerStyle;
  metadata?: Record<string, unknown>;
  minZoom?: number;
  maxZoom?: number;
  sortOrder?: number;
  visibility?: "public" | "private";
}

export interface CreateLayerInput {
  name: string;
  title?: string;
  description?: string;
  geometryType: string;
  srid: number;
  fields: LayerField[];
  minZoom?: number;
  maxZoom?: number;
  style?: LayerStyle;
  metadata?: Record<string, unknown>;
  sortOrder?: number;
  visibility?: "public" | "private";
}

export interface GeoJSONGeometry {
  type: string;
  coordinates?: unknown;
  geometries?: unknown[];
}

export interface GeoJSONFeature {
  type: "Feature";
  id?: number | string;
  geometry: GeoJSONGeometry | null;
  properties: Record<string, unknown> | null;
}

export interface FeatureCollection {
  type: "FeatureCollection";
  features: GeoJSONFeature[];
  total: number;
}

export interface MapStyle {
  id: number;
  name: string;
  isDefault: boolean;
  style: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface Capabilities {
  service: string;
  version: string;
  description: string;
  formats: Record<string, string[]>;
  layers: Array<Record<string, unknown>>;
  styles: MapStyle[];
}

export interface FeatureQuery {
  bbox?: string;
  bboxSrid?: number;
  ids?: number[];
  q?: string;
  limit?: number;
  offset?: number;
  outSrid?: number;
  simplify?: number;
  orderBy?: string;
  order?: "asc" | "desc";
  filters?: Record<string, string>;
}

export interface CRS {
  srid: number;
  authName: string;
  authSrid: number;
  name: string;
}

export interface TileMatrixSet {
  id: string;
  crs: string;
  srid: number;
  tileSize: number;
  bounds: [number, number, number, number];
  scheme: string;
  topLeftOrigin: boolean;
  tileUrl: string;
  maxZoom: number;
}

export type BasemapId =
  | "solid"
  | "osm"
  | "carto-light"
  | "carto-dark"
  | "satellite"
  | "amap"
  | "amap-satellite"
  | "google"
  | "google-satellite"
  | "google-hybrid";

export interface LayerViewState {
  visible: boolean;
  opacity: number;
  /** 可选的临时样式覆盖（不写回后端）。 */
  styleOverride?: LayerStyle;
}

export interface MapWorkspace {
  version: 1;
  name?: string;
  basemap: BasemapId;
  backgroundColor: string;
  center: [number, number];
  zoom: number;
  bearing: number;
  pitch: number;
  layers: Record<string, LayerViewState>;
  order: string[];
  popupFields?: Record<string, string[]>;
}

export interface SharedMap {
  id: number;
  token: string;
  name: string;
  state: MapWorkspace;
  createdBy: number;
  isPublic: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface APIKey {
  id: number;
  name: string;
  prefix: string;
  role: string;
  layerName?: string;
  revoked: boolean;
  createdAt: string;
  lastUsedAt?: string;
}

export interface AuditLog {
  id: number;
  userId: number;
  username: string;
  role: string;
  method: string;
  path: string;
  status: number;
  layer: string;
  ip: string;
  detail: Record<string, unknown>;
  createdAt: string;
}

export interface FieldStat {
  field: string;
  type: string;
  count: number;
  nulls: number;
  distinct: number;
  min?: unknown;
  max?: unknown;
  top?: Array<{ value: string; count: number }>;
}

export interface SearchResponse {
  layers: Array<{
    name: string;
    title: string;
    geometryType: string;
    bounds?: [number, number, number, number];
    featureCount: number;
  }>;
  features: Array<{
    layer: string;
    layerTitle: string;
    feature: GeoJSONFeature;
  }>;
}

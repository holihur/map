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
  type?: "fill" | "line" | "circle" | "symbol";
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
  featureCount: number;
  createdAt: string;
  updatedAt: string;
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

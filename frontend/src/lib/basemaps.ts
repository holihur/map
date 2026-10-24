import { API_BASE } from "@/lib/api";
import type { BasemapId } from "@/types";

export interface BasemapDef {
  id: BasemapId;
  label: string;
  color: string;
  tiles?: string[];
  attribution?: string;
  maxzoom?: number;
}

export const BASEMAPS: BasemapDef[] = [
  {
    id: "solid",
    label: "纯色",
    color: "#f8fafc",
  },
  {
    id: "osm",
    label: "OpenStreetMap",
    color: "#f2efe9",
    tiles: ["https://tile.openstreetmap.org/{z}/{x}/{y}.png"],
    attribution: '© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    maxzoom: 19,
  },
  {
    id: "carto-light",
    label: "CARTO 浅色",
    color: "#f5f5f5",
    tiles: [
      "https://a.basemaps.cartocdn.com/rastertiles/light_all/{z}/{x}/{y}@2x.png",
    ],
    attribution: '© <a href="https://carto.com/attributions">CARTO</a> · © OpenStreetMap',
    maxzoom: 19,
  },
  {
    id: "carto-dark",
    label: "CARTO 深色",
    color: "#0f172a",
    tiles: [
      "https://a.basemaps.cartocdn.com/rastertiles/dark_all/{z}/{x}/{y}@2x.png",
    ],
    attribution: '© <a href="https://carto.com/attributions">CARTO</a> · © OpenStreetMap',
    maxzoom: 19,
  },
  {
    id: "satellite",
    label: "Esri 卫星",
    color: "#1f2937",
    tiles: [
      "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}",
    ],
    attribution: "© Esri, Maxar, Earthstar Geographics",
    maxzoom: 19,
  },
  {
    id: "amap",
    label: "高德地图",
    color: "#f4f1ea",
    tiles: [`${API_BASE}/api/basemaps/amap/tile/{z}/{x}/{y}`],
    attribution: "© 高德地图",
    maxzoom: 18,
  },
  {
    id: "amap-satellite",
    label: "高德卫星",
    color: "#1f2937",
    tiles: [`${API_BASE}/api/basemaps/amap-satellite/tile/{z}/{x}/{y}`],
    attribution: "© 高德地图",
    maxzoom: 18,
  },
  {
    id: "google",
    label: "Google 地图",
    color: "#f2efe9",
    tiles: [`${API_BASE}/api/basemaps/google/tile/{z}/{x}/{y}`],
    attribution: "© Google",
    maxzoom: 20,
  },
  {
    id: "google-satellite",
    label: "Google 卫星",
    color: "#1f2937",
    tiles: [`${API_BASE}/api/basemaps/google-satellite/tile/{z}/{x}/{y}`],
    attribution: "© Google",
    maxzoom: 20,
  },
  {
    id: "google-hybrid",
    label: "Google 混合",
    color: "#1f2937",
    tiles: [`${API_BASE}/api/basemaps/google-hybrid/tile/{z}/{x}/{y}`],
    attribution: "© Google",
    maxzoom: 20,
  },
];

export function basemapById(id: BasemapId): BasemapDef {
  return BASEMAPS.find((b) => b.id === id) ?? BASEMAPS[0];
}

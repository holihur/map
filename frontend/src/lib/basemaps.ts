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
    label: "卫星影像",
    color: "#1f2937",
    tiles: [
      "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}",
    ],
    attribution: "© Esri, Maxar, Earthstar Geographics",
    maxzoom: 19,
  },
];

export function basemapById(id: BasemapId): BasemapDef {
  return BASEMAPS.find((b) => b.id === id) ?? BASEMAPS[0];
}

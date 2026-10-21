import type { GeoJSONGeometry, Layer, MapWorkspace } from "@/types";

const KEY = "mapserver_workspace_v1";

export function defaultWorkspace(layers: Layer[]): MapWorkspace {
  const view: MapWorkspace["layers"] = {};
  for (const l of layers) view[l.name] = { visible: true, opacity: 1 };
  return {
    version: 1,
    basemap: "osm",
    backgroundColor: "#f8fafc",
    center: [10, 25],
    zoom: 1.6,
    bearing: 0,
    pitch: 0,
    layers: view,
    order: layers.map((l) => l.name),
  };
}

export function normalizeWorkspace(ws: MapWorkspace | null, layers: Layer[]): MapWorkspace {
  const base = defaultWorkspace(layers);
  if (!ws) return base;
  const names = new Set(layers.map((l) => l.name));
  const order = (ws.order ?? []).filter((n) => names.has(n));
  for (const l of layers) {
    if (!order.includes(l.name)) order.push(l.name);
  }
  const view: MapWorkspace["layers"] = {};
  for (const l of layers) {
    const prev = ws.layers?.[l.name];
    view[l.name] = {
      visible: prev?.visible ?? true,
      opacity: typeof prev?.opacity === "number" ? prev.opacity : 1,
    };
  }
  return {
    version: 1,
    name: ws.name,
    basemap: ["solid", "osm", "carto-light", "carto-dark", "satellite"].includes(ws.basemap)
      ? ws.basemap
      : base.basemap,
    backgroundColor: ws.backgroundColor ?? base.backgroundColor,
    center: Array.isArray(ws.center) && ws.center.length === 2 ? ws.center : base.center,
    zoom: typeof ws.zoom === "number" ? ws.zoom : base.zoom,
    bearing: typeof ws.bearing === "number" ? ws.bearing : 0,
    pitch: typeof ws.pitch === "number" ? ws.pitch : 0,
    layers: view,
    order,
    popupFields: ws.popupFields,
  };
}

export function loadLocalWorkspace(): MapWorkspace | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    return JSON.parse(raw) as MapWorkspace;
  } catch {
    return null;
  }
}

export function saveLocalWorkspace(ws: MapWorkspace): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(ws));
  } catch {
    /* ignore quota errors */
  }
}

export function orderedLayers(layers: Layer[], ws: MapWorkspace): Layer[] {
  const byName = new Map(layers.map((l) => [l.name, l]));
  const out: Layer[] = [];
  for (const name of ws.order) {
    const l = byName.get(name);
    if (l) out.push(l);
  }
  for (const l of layers) if (!out.includes(l)) out.push(l);
  return out;
}

export function boundsOfGeometry(g: GeoJSONGeometry): [number, number, number, number] | null {
  let minx = Infinity;
  let miny = Infinity;
  let maxx = -Infinity;
  let maxy = -Infinity;
  const walk = (v: unknown) => {
    if (!Array.isArray(v)) return;
    if (typeof v[0] === "number" && typeof v[1] === "number") {
      minx = Math.min(minx, v[0]);
      miny = Math.min(miny, v[1]);
      maxx = Math.max(maxx, v[0]);
      maxy = Math.max(maxy, v[1]);
      return;
    }
    v.forEach(walk);
  };
  walk(g.coordinates);
  if (!Number.isFinite(minx)) return null;
  return [minx, miny, maxx, maxy];
}

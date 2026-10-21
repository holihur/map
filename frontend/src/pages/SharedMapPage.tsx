import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Layers as LayersIcon, Loader2, Map as MapIcon } from "lucide-react";
import { toast } from "sonner";

import { MapView } from "@/components/map/MapView";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Switch } from "@/components/ui/switch";
import { getMap, listLayers } from "@/lib/api";
import { normalizeWorkspace } from "@/lib/workspace";
import type { Layer, MapWorkspace, SharedMap } from "@/types";

export function SharedMapPage() {
  const { token = "" } = useParams();
  const [map, setMap] = useState<SharedMap | null>(null);
  const [layers, setLayers] = useState<Layer[]>([]);
  const [ws, setWs] = useState<MapWorkspace | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const [m, ls] = await Promise.all([getMap(token), listLayers()]);
        if (!alive) return;
        setMap(m);
        setLayers(ls);
        setWs(normalizeWorkspace(m.state, ls));
      } catch (err) {
        toast.error((err as Error).message);
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [token]);

  const initialView = useMemo(
    () =>
      ws
        ? { center: ws.center, zoom: ws.zoom, bearing: ws.bearing, pitch: ws.pitch }
        : undefined,
    [!!ws]
  );

  if (loading) {
    return (
      <div className="flex h-dvh items-center justify-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> 正在加载地图…
      </div>
    );
  }
  if (!ws || !map) {
    return (
      <div className="flex h-dvh flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
        <p>地图不存在或未公开</p>
        <Link to="/map" className="text-primary underline">
          返回 MapServer
        </Link>
      </div>
    );
  }

  const ordered = ws.order
    .map((n) => layers.find((l) => l.name === n))
    .filter((l): l is Layer => !!l);
  for (const l of layers) if (!ordered.includes(l)) ordered.push(l);

  return (
    <div className="relative h-dvh w-full">
      <MapView
        layers={ordered}
        view={ws.layers}
        basemap={ws.basemap}
        backgroundColor={ws.backgroundColor}
        initialView={initialView}
      />
      <Card className="absolute left-4 top-4 w-72 shadow-lg">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 p-3">
          <CardTitle className="flex items-center gap-2 text-sm">
            <MapIcon className="h-4 w-4" /> {map.name || "共享地图"}
          </CardTitle>
          <Link to="/map" className="text-xs text-primary underline">
            打开编辑器
          </Link>
        </CardHeader>
        <CardContent className="p-0">
          <ScrollArea className="max-h-[50vh]">
            <div className="space-y-1 p-2">
              {ordered.map((l) => (
                <div key={l.id} className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-accent">
                  <Switch
                    checked={ws.layers[l.name]?.visible !== false}
                    onCheckedChange={(v) =>
                      setWs((prev) =>
                        prev
                          ? {
                              ...prev,
                              layers: {
                                ...prev.layers,
                                [l.name]: {
                                  ...(prev.layers[l.name] ?? { visible: true, opacity: 1 }),
                                  visible: v,
                                },
                              },
                            }
                          : prev
                      )
                    }
                  />
                  <span className="min-w-0 flex-1 truncate text-sm">{l.title || l.name}</span>
                  <Badge variant="outline" className="text-[10px]">
                    {l.geometryType}
                  </Badge>
                </div>
              ))}
              {ordered.length === 0 && (
                <p className="px-2 py-4 text-center text-xs text-muted-foreground">
                  该地图没有图层
                </p>
              )}
            </div>
          </ScrollArea>
          <div className="flex items-center gap-1.5 border-t px-3 py-2 text-[11px] text-muted-foreground">
            <LayersIcon className="h-3.5 w-3.5" />
            {ordered.length} 个图层 · {map.isPublic ? "公开链接" : "私有链接"}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

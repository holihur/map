import { useCallback, useEffect, useState } from "react";
import { Copy, KeyRound, Loader2, RefreshCw, Share2, ShieldAlert, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  createAPIKey,
  deleteAPIKey,
  deleteMap,
  listAPIKeys,
  listAuditLogs,
  listLayers,
  listMaps,
  revokeAPIKey,
} from "@/lib/api";
import type { APIKey, AuditLog, Layer, SharedMap } from "@/types";

export function PublishPage() {
  const [tab, setTab] = useState("keys");
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [maps, setMaps] = useState<SharedMap[]>([]);
  const [layers, setLayers] = useState<Layer[]>([]);
  const [loading, setLoading] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [role, setRole] = useState("viewer");
  const [layerName, setLayerName] = useState("");
  const [creating, setCreating] = useState(false);
  const [plaintext, setPlaintext] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [k, l, m, ls] = await Promise.all([
        listAPIKeys(),
        listAuditLogs({ limit: 200 }),
        listMaps(),
        listLayers(),
      ]);
      setKeys(k);
      setLogs(l);
      setMaps(m);
      setLayers(ls);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const submit = async () => {
    if (!name.trim()) {
      toast.error("请填写名称");
      return;
    }
    setCreating(true);
    try {
      const res = await createAPIKey({ name: name.trim(), role, layerName: layerName || undefined });
      setPlaintext(res.plaintext);
      setName("");
      setLayerName("");
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setCreating(false);
    }
  };

  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success("已复制");
    } catch {
      window.prompt("复制链接", text);
    }
  };

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">发布与治理</h1>
            <p className="text-sm text-muted-foreground">
              API Key、地图分享与写操作审计。API Key 仅用于第三方只读消费，避免分发登录 Token。
            </p>
          </div>
          <Button variant="outline" onClick={() => void refresh()}>
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            刷新
          </Button>
        </div>

        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="keys">
              <KeyRound className="mr-1 h-4 w-4" /> API Key
            </TabsTrigger>
            <TabsTrigger value="maps">
              <Share2 className="mr-1 h-4 w-4" /> 分享地图
            </TabsTrigger>
            <TabsTrigger value="audit">
              <ShieldAlert className="mr-1 h-4 w-4" /> 审计日志
            </TabsTrigger>
          </TabsList>

          <TabsContent value="keys" className="space-y-4">
            <div className="flex justify-end">
              <Button onClick={() => setCreateOpen(true)}>
                <KeyRound className="h-4 w-4" /> 新建 Key
              </Button>
            </div>
            <Card>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>名称</TableHead>
                      <TableHead>前缀</TableHead>
                      <TableHead>角色</TableHead>
                      <TableHead>限定图层</TableHead>
                      <TableHead>最后使用</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {keys.map((k) => (
                      <TableRow key={k.id}>
                        <TableCell className="font-medium">{k.name}</TableCell>
                        <TableCell className="font-mono text-xs">{k.prefix}…</TableCell>
                        <TableCell>
                          <Badge variant="outline">{k.role}</Badge>
                        </TableCell>
                        <TableCell className="font-mono text-xs">{k.layerName || "全部"}</TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {k.lastUsedAt ? new Date(k.lastUsedAt).toLocaleString() : "从未"}
                        </TableCell>
                        <TableCell>
                          {k.revoked ? (
                            <Badge variant="destructive">已吊销</Badge>
                          ) : (
                            <Badge variant="secondary">有效</Badge>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          {!k.revoked && (
                            <Button variant="ghost" size="sm" onClick={() => void revokeAPIKey(k.id).then(refresh)}>
                              吊销
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => void deleteAPIKey(k.id).then(refresh)}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                    {keys.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={7} className="py-10 text-center text-sm text-muted-foreground">
                          暂无 API Key
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
            <p className="text-xs text-muted-foreground">
              使用方式：请求头 <code className="rounded bg-muted px-1">X-API-Key: ms_xxx</code>，或查询参数{" "}
              <code className="rounded bg-muted px-1">?api_key=ms_xxx</code>。
            </p>
          </TabsContent>

          <TabsContent value="maps" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">已保存的分享地图</CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>名称</TableHead>
                      <TableHead>公开</TableHead>
                      <TableHead>更新时间</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {maps.map((m) => {
                      const url = `${window.location.origin}/share/${m.token}`;
                      return (
                        <TableRow key={m.id}>
                          <TableCell className="font-medium">{m.name || "未命名"}</TableCell>
                          <TableCell>{m.isPublic ? "是" : "否"}</TableCell>
                          <TableCell className="text-xs text-muted-foreground">
                            {new Date(m.updatedAt).toLocaleString()}
                          </TableCell>
                          <TableCell className="text-right">
                            <Button variant="ghost" size="icon" onClick={() => void copy(url)}>
                              <Copy className="h-4 w-4" />
                            </Button>
                            <Button asChild variant="ghost" size="sm">
                              <a href={url} target="_blank" rel="noreferrer">
                                打开
                              </a>
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => {
                                if (window.confirm("删除这张分享地图？")) {
                                  void deleteMap(m.token).then(refresh);
                                }
                              }}
                            >
                              <Trash2 className="h-4 w-4 text-destructive" />
                            </Button>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                    {maps.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={4} className="py-10 text-center text-sm text-muted-foreground">
                          还没有分享地图，去地图页点击「分享」
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="audit">
            <Card>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>时间</TableHead>
                      <TableHead>用户</TableHead>
                      <TableHead>操作</TableHead>
                      <TableHead>图层</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead>IP</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {logs.map((l) => (
                      <TableRow key={l.id}>
                        <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                          {new Date(l.createdAt).toLocaleString()}
                        </TableCell>
                        <TableCell>
                          <div className="text-sm">{l.username || "匿名"}</div>
                          <div className="text-[11px] text-muted-foreground">{l.role}</div>
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {l.method} {l.path}
                        </TableCell>
                        <TableCell className="font-mono text-xs">{l.layer || "—"}</TableCell>
                        <TableCell>
                          <Badge variant={l.status < 400 ? "secondary" : "destructive"}>{l.status}</Badge>
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">{l.ip}</TableCell>
                      </TableRow>
                    ))}
                    {logs.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                          暂无审计记录
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>新建 API Key</DialogTitle>
            <DialogDescription>明文 Key 只在创建成功时显示一次。</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 py-1">
            <div className="grid gap-1.5">
              <Label>名称</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="QGIS 只读" />
            </div>
            <div className="grid gap-1.5">
              <Label>角色</Label>
              <Select value={role} onValueChange={setRole}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="viewer">viewer（只读）</SelectItem>
                  <SelectItem value="editor">editor（可写）</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label>限定图层（可选）</Label>
              <Select value={layerName || "__all__"} onValueChange={(v) => setLayerName(v === "__all__" ? "" : v)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">全部图层</SelectItem>
                  {layers.map((l) => (
                    <SelectItem key={l.id} value={l.name}>
                      {l.title || l.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              关闭
            </Button>
            <Button onClick={() => void submit()} disabled={creating}>
              {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              创建
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!plaintext} onOpenChange={(v) => !v && setPlaintext("")}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>API Key 已创建</DialogTitle>
            <DialogDescription>请立即复制保存，关闭后无法再次查看明文。</DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2">
            <Input readOnly value={plaintext} className="font-mono text-xs" />
            <Button variant="outline" size="icon" onClick={() => void copy(plaintext)}>
              <Copy className="h-4 w-4" />
            </Button>
          </div>
          <DialogFooter>
            <Button onClick={() => setPlaintext("")}>我已保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

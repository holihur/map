import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, RefreshCw, ShieldAlert, Trash2 } from "lucide-react";
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
import { createUser, deleteUser, listUsers, updateUser, type AuthUser, type Role } from "@/lib/api";
import { useAuth } from "@/lib/auth";

const ROLE_LABEL: Record<Role, string> = {
  admin: "管理员",
  editor: "编辑者",
  viewer: "查看者",
};

export function UsersPage() {
  const { can, user: current } = useAuth();
  const [users, setUsers] = useState<AuthUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<AuthUser | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>("viewer");

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setUsers(await listUsers());
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (can("admin")) void refresh();
  }, [can, refresh]);

  if (!can("admin")) {
    return (
      <div className="flex h-full items-center justify-center">
        <Card className="w-96">
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <ShieldAlert className="h-8 w-8 text-destructive" />
            <p className="text-sm text-muted-foreground">仅管理员可访问用户管理</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  const openCreate = () => {
    setEditing(null);
    setUsername("");
    setPassword("");
    setRole("viewer");
    setDialogOpen(true);
  };

  const openEdit = (u: AuthUser) => {
    setEditing(u);
    setUsername(u.username);
    setPassword("");
    setRole(u.role);
    setDialogOpen(true);
  };

  const submit = async () => {
    try {
      if (editing) {
        await updateUser(editing.id, {
          role,
          password: password ? password : undefined,
        });
        toast.success("用户已更新");
      } else {
        if (!username.trim() || password.length < 6) {
          toast.error("用户名不能为空，密码至少 6 位");
          return;
        }
        await createUser({ username: username.trim(), password, role });
        toast.success("用户已创建");
      }
      setDialogOpen(false);
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const remove = async (u: AuthUser) => {
    if (!window.confirm(`删除用户「${u.username}」？`)) return;
    try {
      await deleteUser(u.id);
      toast.success("用户已删除");
      void refresh();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mx-auto max-w-4xl space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">用户管理</h1>
            <p className="text-sm text-muted-foreground">
              角色权限：admin &gt; editor &gt; viewer。
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => void refresh()}>
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              刷新
            </Button>
            <Button onClick={openCreate}>
              <Plus className="h-4 w-4" /> 新建用户
            </Button>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">全部用户（{users.length}）</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>用户名</TableHead>
                  <TableHead>角色</TableHead>
                  <TableHead className="hidden sm:table-cell">创建时间</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((u) => (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">
                      {u.username}
                      {current?.id === u.id && (
                        <Badge variant="secondary" className="ml-2 text-[10px]">
                          我
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge variant={u.role === "admin" ? "default" : "outline"}>
                        {ROLE_LABEL[u.role]}
                      </Badge>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell text-xs text-muted-foreground">
                      {new Date(u.createdAt).toLocaleString("zh-CN")}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(u)}>
                        编辑
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        disabled={current?.id === u.id}
                        onClick={() => void remove(u)}
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editing ? "编辑用户" : "新建用户"}</DialogTitle>
            <DialogDescription>
              {editing ? "留空密码表示不修改。" : "密码至少 6 位。"}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label>用户名</Label>
              <Input
                value={username}
                disabled={!!editing}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label>密码</Label>
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label>角色</Label>
              <Select value={role} onValueChange={(v) => setRole(v as Role)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="viewer">查看者 viewer</SelectItem>
                  <SelectItem value="editor">编辑者 editor</SelectItem>
                  <SelectItem value="admin">管理员 admin</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={submit}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

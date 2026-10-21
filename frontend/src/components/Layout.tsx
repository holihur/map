import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import {
  Database,
  FileJson,
  Globe2,
  Layers,
  LogOut,
  Map as MapIcon,
  Menu,
  Palette,
  Share2,
  Users,
  Workflow,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  adminOnly?: boolean;
}

const NAV: NavItem[] = [
  { to: "/map", label: "地图", icon: MapIcon },
  { to: "/projection", label: "投影", icon: Globe2 },
  { to: "/layers", label: "图层", icon: Layers },
  { to: "/data", label: "数据", icon: Database },
  { to: "/styles", label: "样式", icon: Palette },
  { to: "/capabilities", label: "能力", icon: FileJson },
  { to: "/publish", label: "发布", icon: Share2, adminOnly: true },
  { to: "/users", label: "用户", icon: Users, adminOnly: true },
];

const ROLE_LABEL: Record<string, string> = {
  admin: "管理员",
  editor: "编辑者",
  viewer: "查看者",
};

export function Layout() {
  const { user, logout, can } = useAuth();
  const [menuOpen, setMenuOpen] = useState(false);
  const items = NAV.filter((item) => !item.adminOnly || can("admin"));

  const navLinkClass = ({ isActive }: { isActive: boolean }) =>
    cn(
      "flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors",
      isActive
        ? "bg-primary text-primary-foreground"
        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
    );

  return (
    <div className="flex h-dvh w-full overflow-hidden bg-background">
      {/* 桌面端侧边栏 */}
      <aside className="hidden w-60 shrink-0 flex-col border-r bg-card md:flex">
        <div className="flex h-14 items-center gap-2 border-b px-4">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Workflow className="h-4 w-4" />
          </div>
          <div className="leading-tight">
            <div className="text-sm font-semibold">MapServer</div>
            <div className="text-[11px] text-muted-foreground">矢量瓦片地图服务</div>
          </div>
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto p-3">
          {items.map((item) => (
            <NavLink key={item.to} to={item.to} className={navLinkClass}>
              <item.icon className="h-4 w-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="space-y-2 border-t p-3">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0">
              <div className="truncate text-sm font-medium">{user?.username}</div>
              <Badge variant="outline" className="mt-0.5 text-[10px]">
                {ROLE_LABEL[user?.role ?? ""] ?? user?.role}
              </Badge>
            </div>
            <Button variant="ghost" size="icon" title="退出登录" onClick={logout}>
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
          <div className="text-[11px] text-muted-foreground">React · Go · PostGIS · MVT</div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        {/* 移动端顶栏 */}
        <header className="flex h-14 shrink-0 items-center gap-2 border-b bg-card px-3 pt-safe md:hidden">
          <Button
            variant="ghost"
            size="icon"
            aria-label="打开菜单"
            onClick={() => setMenuOpen(true)}
          >
            <Menu className="h-5 w-5" />
          </Button>
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
              <Workflow className="h-3.5 w-3.5" />
            </div>
            <span className="text-sm font-semibold">MapServer</span>
          </div>
          <div className="ml-auto flex items-center gap-1">
            <Badge variant="outline" className="text-[10px]">
              {ROLE_LABEL[user?.role ?? ""] ?? user?.role}
            </Badge>
            <Button variant="ghost" size="icon" aria-label="退出登录" onClick={logout}>
              <LogOut className="h-5 w-5" />
            </Button>
          </div>
        </header>

        <main className="min-h-0 flex-1 overflow-hidden">
          <Outlet />
        </main>

        {/* 移动端底部导航 */}
        <nav className="flex shrink-0 items-stretch border-t bg-card md:hidden">
          {items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                cn(
                  "flex flex-1 flex-col items-center gap-0.5 py-2 text-[10px] font-medium transition-colors",
                  isActive ? "text-primary" : "text-muted-foreground"
                )
              }
            >
              <item.icon className="h-5 w-5" />
              {item.label}
            </NavLink>
          ))}
        </nav>
      </div>

      {/* 移动端抽屉菜单 */}
      <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
        <SheetContent side="left" className="w-64 p-0">
          <SheetHeader className="border-b p-4 pt-safe">
            <SheetTitle className="flex items-center gap-2">
              <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
                <Workflow className="h-3.5 w-3.5" />
              </div>
              MapServer
            </SheetTitle>
          </SheetHeader>
          <nav className="space-y-1 p-3">
            {items.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={navLinkClass}
                onClick={() => setMenuOpen(false)}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </NavLink>
            ))}
          </nav>
          <div className="absolute inset-x-0 bottom-0 border-t p-4 pb-safe">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">{user?.username}</div>
                <Badge variant="outline" className="mt-0.5 text-[10px]">
                  {ROLE_LABEL[user?.role ?? ""] ?? user?.role}
                </Badge>
              </div>
              <Button variant="outline" size="sm" onClick={logout}>
                <LogOut className="h-4 w-4" /> 退出
              </Button>
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}

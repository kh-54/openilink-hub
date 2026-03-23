import { Outlet, useNavigate, Link, useLocation } from "react-router-dom";
import { useEffect, useState } from "react";
import { LogOut, Github, Puzzle, Bot, LayoutDashboard, User, Shield, Bug } from "lucide-react";
import { api } from "../lib/api";

type NavItem = { path: string; icon: any; label: string; adminOnly?: boolean };

const navItems: NavItem[] = [
  { path: "/dashboard", icon: Bot, label: "Bot 管理" },
  { path: "/dashboard/webhook-plugins", icon: Puzzle, label: "Webhook 插件" },
  { path: "/dashboard/webhook-plugins/debug", icon: Bug, label: "插件调试" },
];

const bottomItems: NavItem[] = [
  { path: "/dashboard/settings", icon: User, label: "账号设置" },
  { path: "/dashboard/admin", icon: Shield, label: "系统管理", adminOnly: true },
];

export function Layout() {
  const navigate = useNavigate();
  const location = useLocation();
  const [user, setUser] = useState<any>(null);

  useEffect(() => {
    api.me().then(setUser).catch(() => navigate("/login", { replace: true }));
  }, []);

  if (!user) return null;

  async function handleLogout() {
    await api.logout();
    navigate("/login", { replace: true });
  }

  function isActive(path: string) {
    if (path === "/dashboard") return location.pathname === "/dashboard" || location.pathname.startsWith("/dashboard/bot/");
    if (path === "/dashboard/webhook-plugins") return location.pathname === "/dashboard/webhook-plugins";
    return location.pathname.startsWith(path);
  }

  function renderNav(items: NavItem[]) {
    return items.map((item) => {
      if (item.adminOnly && user.role !== "admin" && user.role !== "superadmin") return null;
      const active = isActive(item.path);
      return (
        <Link key={item.path} to={item.path}
          className={`flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-colors ${
            active ? "bg-secondary text-foreground font-medium" : "text-muted-foreground hover:text-foreground hover:bg-secondary/50"
          }`}>
          <item.icon className="w-4 h-4" />
          {item.label}
        </Link>
      );
    });
  }

  return (
    <div className="h-screen flex">
      {/* Sidebar — fixed height, independent scroll */}
      <aside className="w-52 border-r flex flex-col shrink-0 h-screen sticky top-0">
        {/* Logo */}
        <div className="px-4 py-4 border-b shrink-0">
          <Link to="/dashboard" className="flex items-center gap-2 hover:opacity-80">
            <LayoutDashboard className="w-5 h-5 text-primary" />
            <span className="font-semibold text-sm">OpenILink Hub</span>
          </Link>
        </div>

        {/* Primary nav */}
        <nav className="flex-1 px-2 py-3 space-y-0.5 overflow-y-auto">
          {renderNav(navItems)}
        </nav>

        {/* Secondary nav + user */}
        <div className="border-t px-2 py-2 space-y-0.5 shrink-0">
          {renderNav(bottomItems)}
        </div>

        {/* User footer */}
        <div className="border-t px-3 py-3 space-y-2 shrink-0">
          <div className="flex items-center gap-2 px-1">
            <div className="w-7 h-7 rounded-full bg-secondary flex items-center justify-center text-xs font-medium">
              {user.username.charAt(0).toUpperCase()}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-xs font-medium truncate">{user.username}</p>
              <p className="text-[10px] text-muted-foreground">{user.role === "superadmin" ? "超级管理员" : user.role === "admin" ? "管理员" : "成员"}</p>
            </div>
          </div>
          <div className="flex items-center gap-1">
            <a href="https://github.com/openilink/openilink-hub" target="_blank" rel="noopener"
              className="flex-1 flex items-center justify-center gap-1 text-[10px] text-muted-foreground hover:text-foreground py-1 rounded hover:bg-secondary/50 transition-colors">
              <Github className="w-3 h-3" /> GitHub
            </a>
            <button onClick={handleLogout}
              className="flex-1 flex items-center justify-center gap-1 text-[10px] text-muted-foreground hover:text-foreground py-1 rounded hover:bg-secondary/50 transition-colors cursor-pointer">
              <LogOut className="w-3 h-3" /> 退出
            </button>
          </div>
        </div>
      </aside>

      {/* Main content — scrolls independently */}
      <main className="flex-1 overflow-auto h-screen">
        <div className="max-w-4xl mx-auto p-6">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

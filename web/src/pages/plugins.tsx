import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Card } from "../components/ui/card";
import { Badge } from "../components/ui/badge";
import { api } from "../lib/api";
import {
  Github,
  Download,
  Check,
  X,
  Trash2,
  Send,
  ArrowLeft,
  ExternalLink,
  BookOpen,
  Bot,
  Puzzle,
  Shield,
} from "lucide-react";
import { SubmitForm } from "./plugin-submit";
import { ReviewCard } from "./plugin-review";

const statusMap: Record<string, { label: string; variant: "default" | "outline" | "destructive" }> =
  {
    approved: { label: "已通过", variant: "default" },
    pending: { label: "待审核", variant: "outline" },
    rejected: { label: "已拒绝", variant: "destructive" },
  };

export function PluginsPage({
  embedded,
  tab: initialTab,
}: {
  embedded?: boolean;
  tab?: "marketplace" | "my" | "submit" | "review";
}) {
  const [plugins, setPlugins] = useState<any[]>([]);
  const [tab, setTab] = useState<"marketplace" | "my" | "submit" | "review">(
    initialTab || "marketplace",
  );
  const [user, setUser] = useState<any>(null);
  const [myPlugins, setMyPlugins] = useState<any[]>([]);

  async function load() {
    try {
      setUser(await api.me());
    } catch {}
    if (tab === "my") {
      try {
        setMyPlugins((await api.myPlugins()) || []);
      } catch {
        setMyPlugins([]);
      }
    } else {
      try {
        setPlugins((await api.listPlugins(tab === "review" ? "pending" : "approved")) || []);
      } catch {
        setPlugins([]);
      }
    }
  }

  useEffect(() => {
    load();
  }, [tab]);
  useEffect(() => {
    if (initialTab) setTab(initialTab);
  }, [initialTab]);

  const isLoggedIn = !!user;
  const isAdmin = user?.role === "admin" || user?.role === "superadmin";

  const content = (
    <div className="space-y-8">
      {/* Hero banner */}
      <div className="space-y-5 rounded-2xl border bg-card p-6 sm:p-8">
        <div className="flex items-start justify-between gap-5">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">社区驱动的 Webhook 插件</h1>
            <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground sm:text-base">
              一键安装到你的渠道，自动转发消息到飞书、Slack、钉钉等服务。所有插件代码公开审核，在安全沙箱中执行。
            </p>
          </div>
          <div className="flex gap-2 shrink-0">
            {isLoggedIn && (
              <Button
                variant="outline"
                size="sm"
                className="px-4 text-sm"
                onClick={() => setTab("submit")}
              >
                <Send className="w-3 h-3 mr-1" /> 提交插件
              </Button>
            )}
          </div>
        </div>

        {/* AI development callout */}
        <div className="flex items-start gap-4 rounded-xl border border-primary/20 bg-primary/5 p-4 sm:p-5">
          <Bot className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <div>
            <p className="text-sm font-medium">使用 AI 编写插件</p>
            <p className="mt-1.5 text-sm leading-6 text-muted-foreground">
              将以下链接发送给你的 AI 助手（Claude、ChatGPT
              等），它可以直接阅读并为你生成符合规范的插件代码：
            </p>
            <div className="mt-3 flex items-center gap-2">
              <code className="select-all rounded border bg-background px-2.5 py-1.5 text-xs font-mono">
                {location.origin}/api/webhook-plugins/skill.md
              </code>
              <CopyButton value={`${location.origin}/api/webhook-plugins/skill.md`} />
              <a
                href="/api/webhook-plugins/skill.md"
                target="_blank"
                rel="noopener"
                className="flex items-center gap-0.5 text-xs text-primary hover:underline"
              >
                <BookOpen className="w-3 h-3" /> 预览文档
              </a>
            </div>
          </div>
        </div>
      </div>

      {/* Tabs — only show when not driven by sidebar routes */}
      {!embedded && (
        <div className="flex border rounded-lg overflow-hidden w-fit">
          <button
            className={`px-3 py-1 text-xs cursor-pointer ${tab === "marketplace" ? "bg-secondary" : "text-muted-foreground"}`}
            onClick={() => setTab("marketplace")}
          >
            市场 {plugins.length > 0 && tab === "marketplace" ? `(${plugins.length})` : ""}
          </button>
          {isLoggedIn && (
            <button
              className={`px-3 py-1 text-xs cursor-pointer ${tab === "my" ? "bg-secondary" : "text-muted-foreground"}`}
              onClick={() => setTab("my")}
            >
              我的插件
            </button>
          )}
          {isLoggedIn && (
            <button
              className={`px-3 py-1 text-xs cursor-pointer ${tab === "submit" ? "bg-secondary" : "text-muted-foreground"}`}
              onClick={() => setTab("submit")}
            >
              提交
            </button>
          )}
          {isAdmin && (
            <button
              className={`px-3 py-1 text-xs cursor-pointer ${tab === "review" ? "bg-secondary" : "text-muted-foreground"}`}
              onClick={() => setTab("review")}
            >
              审核 {plugins.length > 0 && tab === "review" ? `(${plugins.length})` : ""}
            </button>
          )}
        </div>
      )}

      {/* Content */}
      {tab === "marketplace" && (
        <div className="space-y-3">
          {plugins.length === 0 && (
            <div className="space-y-4 py-20 text-center">
              <Puzzle className="w-10 h-10 mx-auto text-muted-foreground/50" />
              <p className="text-sm text-muted-foreground">暂无已审核的插件</p>
              {isLoggedIn && (
                <Button
                  variant="outline"
                  size="sm"
                  className="px-4 text-sm"
                  onClick={() => setTab("submit")}
                >
                  成为第一个贡献者
                </Button>
              )}
            </div>
          )}
          {plugins.map((p) => (
            <PluginCard
              key={p.id}
              plugin={p}
              onRefresh={load}
              isAdmin={isAdmin}
              isLoggedIn={isLoggedIn}
              mode="marketplace"
            />
          ))}
        </div>
      )}

      {tab === "my" && <MyPluginsTab plugins={myPlugins} onRefresh={load} />}

      {tab === "submit" && (
        <SubmitForm
          onSubmitted={() => {
            setTab("marketplace");
            load();
          }}
        />
      )}

      {tab === "review" && (
        <div className="space-y-4">
          {plugins.length === 0 && (
            <div className="py-16 text-center">
              <Shield className="w-8 h-8 mx-auto text-muted-foreground/50 mb-2" />
              <p className="text-sm text-muted-foreground">没有待审核的插件</p>
            </div>
          )}
          {plugins.map((p) => (
            <ReviewCard key={p.id} plugin={p} onRefresh={load} />
          ))}
        </div>
      )}
    </div>
  );

  if (embedded) return content;

  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b px-6 py-4 sm:px-8 lg:px-12 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-2">
          <Link to="/" className="text-muted-foreground hover:text-foreground">
            <ArrowLeft className="w-4 h-4" />
          </Link>
          <Puzzle className="w-4 h-4 text-primary" />
          <span className="text-base font-semibold tracking-tight">Webhook 插件市场</span>
        </div>
        <div className="flex items-center gap-2">
          {!isLoggedIn && (
            <Link to="/login">
              <Button size="sm" className="px-3 text-sm">
                登录
              </Button>
            </Link>
          )}
          {isLoggedIn && <span className="text-sm text-muted-foreground">{user.username}</span>}
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-6 py-8 sm:px-8 sm:py-10">{content}</main>
      <footer className="border-t px-6 py-6 text-center text-sm text-muted-foreground sm:px-8">
        <a
          href="https://github.com/openilink/openilink-hub"
          target="_blank"
          rel="noopener"
          className="hover:text-primary"
        >
          OpeniLink Hub
        </a>
        {" · "}Webhook 插件运行在安全沙箱中（5s 超时 · 禁止系统访问 · 管理员审核）
      </footer>
    </div>
  );
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      onClick={() => {
        navigator.clipboard.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }}
      className="cursor-pointer text-muted-foreground hover:text-foreground"
    >
      {copied ? (
        <Check className="w-3 h-3 text-primary" />
      ) : (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
        </svg>
      )}
    </button>
  );
}

function PluginCard({
  plugin,
  onRefresh,
  isAdmin,
  isLoggedIn,
  mode,
}: {
  plugin: any;
  onRefresh: () => void;
  isAdmin: boolean;
  isLoggedIn: boolean;
  mode: string;
}) {
  const [detail, setDetail] = useState<any>(null);
  const [showScript, setShowScript] = useState(false);
  const [versions, setVersions] = useState<any[] | null>(null);
  const [showVersions, setShowVersions] = useState(false);

  async function toggleVersions() {
    if (!showVersions && !versions) {
      try {
        setVersions((await api.pluginVersions(plugin.id)) || []);
      } catch {
        setVersions([]);
      }
    }
    setShowVersions(!showVersions);
  }

  async function handleInstall() {
    const data = await api.installPlugin(plugin.id);
    await navigator.clipboard.writeText(data.script);
    alert("脚本已复制到剪贴板！\n\n推荐方式：进入渠道 → Webhook → 插件市场 → 选择此插件一键安装");
    onRefresh();
  }

  async function handleReview(status: string) {
    let reason = "";
    if (status === "rejected") {
      reason = prompt("请输入拒绝原因：") || "";
      if (!reason) return;
    }
    await api.reviewPlugin(plugin.id, status, reason);
    onRefresh();
  }

  async function handleDelete() {
    if (!confirm("确认删除此插件？")) return;
    await api.deletePlugin(plugin.id);
    onRefresh();
  }

  async function toggleScript() {
    if (!detail) {
      try {
        const d = await api.getPlugin(plugin.id);
        setDetail(d.latest_version || d); // version has script
      } catch {}
    }
    setShowScript(!showScript);
  }

  const s = statusMap[plugin.status || "approved"];
  const config = plugin.config_schema || [];
  const grants = (plugin.grant_perms || "").split(",").filter(Boolean);
  const matchTypes = plugin.match_types || "*";
  const connectDomains = plugin.connect_domains || "*";
  const riskLevel =
    connectDomains === "*" && grants.includes("reply")
      ? "high"
      : connectDomains === "*" || grants.includes("reply")
        ? "medium"
        : "low";
  const riskColors: Record<string, string> = {
    low: "text-primary",
    medium: "text-yellow-500",
    high: "text-destructive",
  };
  const riskLabels: Record<string, string> = { low: "低风险", medium: "中风险", high: "高风险" };

  return (
    <Card className="space-y-4 rounded-2xl p-6 sm:p-7">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {plugin.icon && <span className="text-base">{plugin.icon}</span>}
            <span className="text-base font-medium">{plugin.name}</span>
            <Badge variant={s.variant} className="text-xs">
              {s.label}
            </Badge>
            <span className="text-xs text-muted-foreground">v{plugin.version}</span>
            {plugin.license && (
              <span className="text-xs text-muted-foreground">{plugin.license}</span>
            )}
          </div>
          <p className="mt-1.5 text-sm leading-7 text-muted-foreground">{plugin.description}</p>
          <div className="flex items-center gap-3 mt-1 text-xs text-muted-foreground flex-wrap">
            <span>作者: {plugin.author || "anonymous"}</span>
            {(plugin.owner_name || plugin.submitter_name) && (
              <span>拥有者: {plugin.owner_name || plugin.submitter_name}</span>
            )}
            <span>{plugin.install_count} 次安装</span>
            <span>{new Date(plugin.created_at * 1000).toLocaleDateString()}</span>
            {(plugin.github_url || plugin.homepage) && (
              <a
                href={plugin.homepage || plugin.github_url}
                target="_blank"
                rel="noopener"
                className="flex items-center gap-0.5 hover:text-primary"
              >
                <Github className="w-3 h-3" /> 源码
              </a>
            )}
            {plugin.commit_hash && (
              <span className="font-mono">{plugin.commit_hash.slice(0, 7)}</span>
            )}
          </div>
          {/* Security summary */}
          <div className="flex items-center gap-2 mt-1 text-xs flex-wrap">
            <span className={riskColors[riskLevel]}>
              <Shield className="w-3 h-3 inline mr-0.5" />
              {riskLabels[riskLevel]}
            </span>
            <span className="text-muted-foreground">
              权限: {grants.length > 0 ? grants.join(", ") : "none"}
            </span>
            <span className="text-muted-foreground">消息: {matchTypes}</span>
            {connectDomains !== "*" && (
              <span className="text-muted-foreground">域名: {connectDomains}</span>
            )}
          </div>
          {plugin.reject_reason && (
            <p className="text-xs text-destructive mt-0.5">拒绝原因：{plugin.reject_reason}</p>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <Button size="sm" variant="ghost" className="h-7 text-xs" onClick={toggleVersions}>
            {showVersions ? "收起" : "版本"}
          </Button>
          <Button size="sm" variant="ghost" className="h-7 text-xs" onClick={toggleScript}>
            {showScript ? "收起" : "源码"}
          </Button>
          {mode === "marketplace" && plugin.status === "approved" && isLoggedIn && (
            <Button size="sm" variant="outline" className="h-7 text-xs" onClick={handleInstall}>
              <Download className="w-3 h-3 mr-1" /> 安装
            </Button>
          )}
          {mode === "review" && (
            <>
              <Button size="sm" className="h-7 text-xs" onClick={() => handleReview("approved")}>
                <Check className="w-3 h-3 mr-1" /> 通过
              </Button>
              <Button
                size="sm"
                variant="destructive"
                className="h-7 text-xs"
                onClick={() => handleReview("rejected")}
              >
                <X className="w-3 h-3 mr-1" /> 拒绝
              </Button>
            </>
          )}
          {isAdmin && (
            <Button size="sm" variant="ghost" className="h-7" onClick={handleDelete}>
              <Trash2 className="w-3 h-3 text-destructive" />
            </Button>
          )}
        </div>
      </div>

      {config.length > 0 && (
        <div className="text-xs text-muted-foreground">
          配置项：{config.map((c: any) => `${c.name} (${c.description || c.type})`).join("、")}
        </div>
      )}

      {showVersions && versions && (
        <div className="space-y-1">
          <p className="text-xs font-medium">发版历史</p>
          {versions.map((v) => (
            <div
              key={v.id}
              className="flex items-center gap-2 text-xs p-1.5 rounded border bg-background"
            >
              <span className="font-mono font-medium">v{v.version}</span>
              <Badge
                variant={
                  v.status === "approved"
                    ? "default"
                    : v.status === "rejected" || v.status === "cancelled"
                      ? "destructive"
                      : "outline"
                }
                className="text-xs"
              >
                {v.status === "approved"
                  ? "✓"
                  : v.status === "rejected"
                    ? "✕"
                    : v.status === "superseded"
                      ? "⊘"
                      : v.status === "cancelled"
                        ? "✕"
                        : "⏳"}
              </Badge>
              <span className="text-muted-foreground">{v.status}</span>
              {v.changelog && (
                <span className="text-muted-foreground flex-1 truncate">{v.changelog}</span>
              )}
              {v.commit_hash && (
                <span className="font-mono text-muted-foreground">{v.commit_hash.slice(0, 7)}</span>
              )}
              <span className="text-muted-foreground">
                {new Date(v.created_at * 1000).toLocaleDateString()}
              </span>
              {v.status === "pending" && isLoggedIn && (
                <button
                  onClick={async (e) => {
                    e.stopPropagation();
                    await api.cancelVersion(plugin.id, v.id);
                    toggleVersions();
                    toggleVersions();
                  }}
                  className="text-destructive hover:underline cursor-pointer ml-auto"
                >
                  取消
                </button>
              )}
            </div>
          ))}
          {versions.length === 0 && <p className="text-xs text-muted-foreground">暂无历史版本</p>}
        </div>
      )}

      {showScript && (
        <div>
          {detail?.script ? (
            <pre className="text-xs bg-background border rounded p-3 overflow-x-auto max-h-64 overflow-y-auto whitespace-pre-wrap font-mono">
              {detail.script}
            </pre>
          ) : plugin.github_url ? (
            <a
              href={plugin.github_url}
              target="_blank"
              rel="noopener"
              className="text-xs text-primary flex items-center gap-1"
            >
              <ExternalLink className="w-3 h-3" /> 在 GitHub 查看源码
            </a>
          ) : (
            <p className="text-xs text-muted-foreground">登录后可查看脚本源码</p>
          )}
        </div>
      )}
    </Card>
  );
}

function MyPluginsTab({ plugins, onRefresh }: { plugins: any[]; onRefresh: () => void }) {
  const statusLabels: Record<
    string,
    { label: string; variant: "default" | "outline" | "destructive" }
  > = {
    approved: { label: "已通过", variant: "default" },
    pending: { label: "待审核", variant: "outline" },
    rejected: { label: "已拒绝", variant: "destructive" },
  };

  if (plugins.length === 0) {
    return (
      <div className="text-center py-16 space-y-3">
        <Puzzle className="w-10 h-10 mx-auto text-muted-foreground/50" />
        <p className="text-sm text-muted-foreground">你还没有提交任何插件</p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {plugins.map((p) => {
        const hasApproved = !!p.latest_version_id;
        const s = hasApproved ? statusLabels.approved : statusLabels.pending;
        return (
          <Card key={p.id} className="flex items-center justify-between">
            <div className="flex items-center gap-2 min-w-0">
              {p.icon && <span>{p.icon}</span>}
              <div className="min-w-0">
                <div className="flex items-center gap-1.5">
                  <span className="text-xs font-medium">{p.name}</span>
                  <span className="text-xs text-muted-foreground">v{p.version}</span>
                  <Badge variant={s.variant} className="text-xs">
                    {s.label}
                  </Badge>
                </div>
                <p className="text-xs text-muted-foreground truncate">{p.description}</p>
              </div>
            </div>
            <div className="text-xs text-muted-foreground shrink-0 ml-2 text-right">
              <p>{p.install_count} 安装</p>
              <p>{new Date(p.created_at * 1000).toLocaleDateString()}</p>
            </div>
          </Card>
        );
      })}
    </div>
  );
}

import { useEffect, useState } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { ArrowLeft, Blocks, Plus, Trash2, ShieldCheck } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Card } from "../components/ui/card";
import { Badge } from "../components/ui/badge";
import { api } from "../lib/api";
import { InstallationDetail } from "./app-installation";

const EVENT_TYPES = [
  { key: "message.text", label: "文本消息" },
  { key: "message.image", label: "图片消息" },
  { key: "message.video", label: "视频消息" },
  { key: "message.voice", label: "语音消息" },
  { key: "message.file", label: "文件消息" },
  { key: "message.location", label: "位置消息" },
  { key: "contact.added", label: "新增联系人" },
  { key: "group.join", label: "入群" },
  { key: "group.leave", label: "退群" },
];

const SCOPES = [
  { key: "messages.send", label: "发送消息" },
  { key: "messages.read", label: "读取消息" },
  { key: "contacts.read", label: "读取联系人" },
  { key: "bot.read", label: "读取 Bot 信息" },
];

export function AppDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [app, setApp] = useState<any>(null);
  const [tab, setTab] = useState<"settings" | "installations" | "events">("settings");

  async function loadApp() {
    try {
      const a = await api.getApp(id!);
      setApp(a);
    } catch {
      navigate("/dashboard/apps");
    }
  }

  useEffect(() => {
    loadApp();
  }, [id]);

  if (!app) return null;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link to="/dashboard/apps" className="text-muted-foreground hover:text-foreground">
          <ArrowLeft className="w-4 h-4" />
        </Link>
        {app.icon_url ? (
          <img src={app.icon_url} alt="" className="w-8 h-8 rounded-lg object-cover" />
        ) : app.icon ? (
          <div className="w-8 h-8 rounded-lg bg-secondary flex items-center justify-center text-lg">{app.icon}</div>
        ) : (
          <div className="w-8 h-8 rounded-lg bg-secondary flex items-center justify-center">
            <Blocks className="w-4 h-4 text-muted-foreground" />
          </div>
        )}
        <div className="flex-1 min-w-0">
          <h1 className="text-lg font-semibold">{app.name}</h1>
          <p className="text-xs text-muted-foreground font-mono">{app.slug}</p>
        </div>
        <Badge variant={app.status === "active" ? "default" : "outline"}>
          {app.status === "active" ? "启用" : app.status || "草稿"}
        </Badge>
        {app.listed ? (
          <Badge variant="default">已上架</Badge>
        ) : app.listing_status === "pending" ? (
          <Badge variant="outline">审核中</Badge>
        ) : app.listing_status === "rejected" ? (
          <Badge variant="destructive">已拒绝</Badge>
        ) : null}
      </div>

      {/* Listing status */}
      {!app.listed && app.listing_status !== "pending" && (
        <ListingSection app={app} onUpdate={loadApp} />
      )}
      {app.listing_status === "pending" && (
        <Card className="bg-primary/5 border-primary/20">
          <p className="text-xs">上架申请已提交，等待管理员审核。</p>
        </Card>
      )}
      {app.listing_status === "rejected" && app.listing_reject_reason && (
        <Card className="bg-destructive/5 border-destructive/20 space-y-2">
          <p className="text-xs text-destructive">上架被拒绝：{app.listing_reject_reason}</p>
          <Button size="sm" variant="outline" onClick={async () => { await api.requestListing(app.id); loadApp(); }}>重新申请</Button>
        </Card>
      )}

      {/* Tabs */}
      <div className="flex rounded-lg border overflow-hidden w-fit">
        <button
          className={`px-3 py-1.5 text-xs cursor-pointer ${tab === "settings" ? "bg-secondary font-medium" : "text-muted-foreground"}`}
          onClick={() => setTab("settings")}
        >
          设置
        </button>
        <button
          className={`px-3 py-1.5 text-xs cursor-pointer ${tab === "installations" ? "bg-secondary font-medium" : "text-muted-foreground"}`}
          onClick={() => setTab("installations")}
        >
          安装
        </button>
        <button
          className={`px-3 py-1.5 text-xs cursor-pointer ${tab === "events" ? "bg-secondary font-medium" : "text-muted-foreground"}`}
          onClick={() => setTab("events")}
        >
          事件与权限
        </button>
      </div>

      {tab === "settings" ? (
        <SettingsTab app={app} onUpdate={loadApp} />
      ) : tab === "installations" ? (
        <InstallationsTab appId={id!} />
      ) : (
        <EventsScopesTab app={app} onUpdate={loadApp} />
      )}
    </div>
  );
}

// ==================== Settings Tab ====================

function SettingsTab({ app, onUpdate }: { app: any; onUpdate: () => void }) {
  const navigate = useNavigate();
  const [form, setForm] = useState({
    name: app.name || "",
    description: app.description || "",
    icon: app.icon || "",
    homepage: app.homepage || "",
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSuccess("");
    setSaving(true);
    try {
      await api.updateApp(app.id, form);
      setSuccess("已保存");
      onUpdate();
    } catch (err: any) {
      setError(err.message);
    }
    setSaving(false);
  }

  async function handleDelete() {
    if (!confirm("确定删除此 App？所有安装也将被移除。")) return;
    try {
      await api.deleteApp(app.id);
      navigate("/dashboard/apps");
    } catch {}
  }

  return (
    <div className="space-y-4">
      <Card className="space-y-3">
        <h3 className="text-sm font-medium">基本信息</h3>
        <form onSubmit={handleSave} className="space-y-2">
          <Input
            placeholder="名称"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            className="h-8 text-xs"
          />
          <Input
            placeholder="描述"
            value={form.description}
            onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
            className="h-8 text-xs"
          />
          <Input
            placeholder="图标 URL"
            value={form.icon}
            onChange={(e) => setForm((f) => ({ ...f, icon: e.target.value }))}
            className="h-8 text-xs"
          />
          <Input
            placeholder="主页 URL"
            value={form.homepage}
            onChange={(e) => setForm((f) => ({ ...f, homepage: e.target.value }))}
            className="h-8 text-xs"
          />
          <div className="flex items-center justify-between">
            <div>
              {error && <span className="text-xs text-destructive">{error}</span>}
              {success && <span className="text-xs text-primary">{success}</span>}
            </div>
            <Button type="submit" size="sm" disabled={saving}>
              {saving ? "..." : "保存"}
            </Button>
          </div>
        </form>
      </Card>

      <ToolsEditor app={app} onUpdate={onUpdate} />

      <Card className="space-y-3">
        <h3 className="text-sm font-medium text-destructive">危险区域</h3>
        <Button variant="destructive" size="sm" onClick={handleDelete}>
          <Trash2 className="w-3.5 h-3.5 mr-1" /> 删除 App
        </Button>
      </Card>
    </div>
  );
}

// ==================== Tools Editor ====================

function ToolsEditor({ app, onUpdate }: { app: any; onUpdate: () => void }) {
  const [tools, setTools] = useState<{ name: string; description: string; command: string; parameters: string }[]>(
    (app.tools || []).map((t: any) => ({
      ...t,
      parameters: t.parameters ? JSON.stringify(t.parameters, null, 2) : "",
    })),
  );
  const [saving, setSaving] = useState(false);

  function addTool() {
    setTools([...tools, { name: "", description: "", command: "", parameters: "" }]);
  }

  function removeTool(index: number) {
    setTools(tools.filter((_, i) => i !== index));
  }

  function updateTool(index: number, field: string, value: string) {
    setTools(tools.map((t, i) => (i === index ? { ...t, [field]: value } : t)));
  }

  async function handleSave() {
    setSaving(true);
    try {
      const payload = tools.map((t) => {
        const tool: any = { name: t.name, description: t.description };
        if (t.command) tool.command = t.command.replace(/^\//, "");
        if (t.parameters?.trim()) tool.parameters = JSON.parse(t.parameters);
        return tool;
      });
      await api.updateApp(app.id, { tools: payload });
      onUpdate();
    } catch {}
    setSaving(false);
  }

  return (
    <Card className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">工具 (Tools)</h3>
        <Button variant="outline" size="sm" className="h-7 text-xs" onClick={addTool}>
          <Plus className="w-3 h-3 mr-1" /> 添加工具
        </Button>
      </div>
      {tools.length === 0 && (
        <p className="text-xs text-muted-foreground">暂无工具。工具定义了 App 的能力，可被 AI Agent 自动调用。</p>
      )}
      {tools.map((tool, i) => (
        <div key={i} className="flex items-start gap-2 p-2 rounded-lg border bg-background">
          <div className="flex-1 space-y-1">
            <div className="flex gap-1">
              <Input
                placeholder="工具名（如 list_prs）"
                value={tool.name}
                onChange={(e) => updateTool(i, "name", e.target.value)}
                className="h-7 text-xs font-mono flex-1"
              />
              <Input
                placeholder="命令触发（如 pr，可选）"
                value={tool.command}
                onChange={(e) => updateTool(i, "command", e.target.value)}
                className="h-7 text-xs font-mono w-36"
              />
            </div>
            <Input
              placeholder="描述（AI Agent 用来判断何时调用）"
              value={tool.description}
              onChange={(e) => updateTool(i, "description", e.target.value)}
              className="h-7 text-xs"
            />
            <textarea
              placeholder='参数 JSON Schema（可选）\n{"type":"object","properties":{"repo":{"type":"string"}}}'
              value={tool.parameters}
              onChange={(e) => updateTool(i, "parameters", e.target.value)}
              rows={2}
              className="w-full rounded-md border border-input bg-transparent px-2 py-1 text-[11px] font-mono placeholder:text-muted-foreground/40 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 resize-none"
            />
          </div>
          <button onClick={() => removeTool(i)} className="cursor-pointer mt-1">
            <Trash2 className="w-3.5 h-3.5 text-destructive" />
          </button>
        </div>
      ))}
      {tools.length > 0 && (
        <Button size="sm" onClick={handleSave} disabled={saving}>
          {saving ? "..." : "保存工具"}
        </Button>
      )}
    </Card>
  );
}

// ==================== Events & Scopes Tab ====================

function EventsScopesTab({ app, onUpdate }: { app: any; onUpdate: () => void }) {
  const [events, setEvents] = useState<string[]>(app.events || []);
  const [scopes, setScopes] = useState<string[]>(app.scopes || []);
  const [saving, setSaving] = useState(false);

  function toggleEvent(key: string) {
    setEvents((prev) => (prev.includes(key) ? prev.filter((e) => e !== key) : [...prev, key]));
  }
  function toggleScope(key: string) {
    setScopes((prev) => (prev.includes(key) ? prev.filter((s) => s !== key) : [...prev, key]));
  }

  async function handleSave() {
    setSaving(true);
    try {
      await api.updateApp(app.id, { events, scopes });
      onUpdate();
    } catch {}
    setSaving(false);
  }

  return (
    <div className="space-y-4">
      <Card className="space-y-3">
        <h3 className="text-sm font-medium">订阅事件</h3>
        <p className="text-xs text-muted-foreground">选择你的 App 需要接收的事件类型</p>
        <div className="grid grid-cols-2 gap-2">
          {EVENT_TYPES.map((et) => (
            <label key={et.key} className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={events.includes(et.key)}
                onChange={() => toggleEvent(et.key)}
                className="w-3.5 h-3.5 accent-primary"
              />
              <span className="text-xs">{et.label}</span>
              <span className="text-xs text-muted-foreground font-mono">{et.key}</span>
            </label>
          ))}
        </div>
      </Card>

      <Card className="space-y-3">
        <h3 className="text-sm font-medium">权限范围</h3>
        <p className="text-xs text-muted-foreground">选择你的 App 需要的 API 权限</p>
        <div className="grid grid-cols-2 gap-2">
          {SCOPES.map((s) => (
            <label key={s.key} className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={scopes.includes(s.key)}
                onChange={() => toggleScope(s.key)}
                className="w-3.5 h-3.5 accent-primary"
              />
              <span className="text-xs">{s.label}</span>
              <span className="text-xs text-muted-foreground font-mono">{s.key}</span>
            </label>
          ))}
        </div>
      </Card>

      <Button size="sm" onClick={handleSave} disabled={saving}>
        {saving ? "..." : "保存"}
      </Button>
    </div>
  );
}

// ==================== Installations Tab ====================

function InstallationsTab({ appId }: { appId: string }) {
  const [installations, setInstallations] = useState<any[]>([]);
  const [bots, setBots] = useState<any[]>([]);
  const [installing, setInstalling] = useState(false);
  const [selectedBot, setSelectedBot] = useState("");
  const [detail, setDetail] = useState<any>(null);

  async function load() {
    try {
      const [ins, bs] = await Promise.all([api.listInstallations(appId), api.listBots()]);
      setInstallations(ins || []);
      setBots(bs || []);
    } catch {}
  }

  useEffect(() => {
    load();
  }, [appId]);

  async function handleInstall() {
    if (!selectedBot) return;
    try {
      await api.installApp(appId, { bot_id: selectedBot });
      setInstalling(false);
      setSelectedBot("");
      load();
    } catch {}
  }

  if (detail) {
    return (
      <InstallationDetail
        appId={appId}
        installation={detail}
        onBack={() => {
          setDetail(null);
          load();
        }}
      />
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">已安装</h3>
        {!installing && (
          <Button
            variant="outline"
            size="sm"
            className="h-7 text-xs"
            onClick={() => setInstalling(true)}
          >
            <Plus className="w-3 h-3 mr-1" /> 安装到 Bot
          </Button>
        )}
      </div>

      {installing && (
        <Card className="space-y-3">
          <h3 className="text-sm font-medium">选择 Bot</h3>
          <select
            value={selectedBot}
            onChange={(e) => setSelectedBot(e.target.value)}
            className="w-full h-8 text-xs rounded-md border bg-background px-2"
          >
            <option value="">选择一个 Bot...</option>
            {bots.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
          <div className="flex gap-2">
            <Button size="sm" onClick={handleInstall} disabled={!selectedBot}>
              安装
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setInstalling(false)}>
              取消
            </Button>
          </div>
        </Card>
      )}

      {installations.map((ins) => (
        <Card
          key={ins.id}
          className="flex items-center justify-between cursor-pointer hover:border-primary/50 transition-colors"
          onClick={() => setDetail(ins)}
        >
          <div>
            <p className="text-sm font-medium">{ins.bot_name || ins.bot_id}</p>
            <p className="text-xs text-muted-foreground font-mono mt-0.5">{ins.id}</p>
          </div>
          <div className="flex items-center gap-2">
            {ins.url_verified && (
              <Badge variant="default">
                <ShieldCheck className="w-3 h-3 mr-1" /> URL 已验证
              </Badge>
            )}
            <Badge variant={ins.enabled ? "default" : "outline"}>
              {ins.enabled ? "启用" : "禁用"}
            </Badge>
          </div>
        </Card>
      ))}

      {installations.length === 0 && !installing && (
        <p className="text-center text-sm text-muted-foreground py-4">暂无安装</p>
      )}
    </div>
  );
}

function ListingSection({ app, onUpdate }: { app: any; onUpdate: () => void }) {
  const [loading, setLoading] = useState(false);
  return (
    <Card className="flex items-center justify-between">
      <div>
        <p className="text-xs font-medium">上架到 App 市场</p>
        <p className="text-[10px] text-muted-foreground">上架后其他用户可以搜索并安装你的 App</p>
      </div>
      <Button size="sm" variant="outline" disabled={loading} onClick={async () => {
        setLoading(true);
        try { await api.requestListing(app.id); onUpdate(); } catch {}
        setLoading(false);
      }}>{loading ? "..." : "申请上架"}</Button>
    </Card>
  );
}

package hermes

import (
	"encoding/json"

	"github.com/openilink/openilink-hub/internal/builtin"
)

func init() {
	builtin.Register(builtin.AppManifest{
		Slug:        "hermes",
		Name:        "Hermes Agent",
		Description: "通过 Hermes Agent 接入 Bot，让 AI 助手处理微信消息",
		Icon:        "🪽",
		Readme:      "通过 Nous Research 的 Hermes Agent 接入 Bot。Hermes Agent 是一个具有自我学习能力的 AI 助手，支持跨平台消息处理。安装此 App 后，Hermes 可以通过 Hub 管理的微信 Bot 收发消息。\n\n> **注意**：平台支持已提交至 Hermes Agent 上游（[PR #9038](https://github.com/NousResearch/hermes-agent/pull/9038)），合并前需使用包含此补丁的版本。",
		Guide: "## Hermes Agent 接入指南\n\n你已在 Hub 上安装了 Hermes Agent App。下面将 Hermes Agent 连接到这个 Bot。\n\n### 1. 复制上方的 Token\n\n页面上方显示的 Token 是 Hermes Agent 连接此 Bot 的凭证，请先复制。\n\n### 2. 配置环境变量\n\n将以下内容写入 Hermes 的 `~/.hermes/.env` 文件：\n\n```bash\nOPENILINK_HUB_URL={hub_url}\nOPENILINK_TOKEN={your_token}\nOPENILINK_ALLOW_ALL_USERS=true\n```\n\n### 3. 启动 Gateway\n\n```bash\nhermes gateway\n```\n\n完成！在微信中发送消息即可与 Hermes AI 对话。\n\n### 手动发送消息（API）\n\n```bash\ncurl -X POST {hub_url}/bot/v1/message/send \\\n  -H \"Authorization: Bearer {your_token}\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"content\":\"hello\"}'\n```\n\n### 更多信息\n\n- [Hermes Agent 文档](https://hermes-agent.nousresearch.com)\n- [GitHub](https://github.com/nousresearch/hermes-agent)",
		Scopes:      []string{"message:read", "message:write"},
		Events:      []string{"message"},
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, nil) // nil handler — events delivered via WS to Hermes gateway adapter
}

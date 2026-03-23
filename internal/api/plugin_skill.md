# OpenILink Hub — Webhook Plugin Development Skill

> This document is designed for AI agents and developers to understand how to write webhook plugins for the OpenILink Hub plugin marketplace.

## What is a Webhook Plugin?

A webhook plugin is a JavaScript file that transforms how messages are forwarded from WeChat to external services via HTTP webhooks. Plugins run in a sandboxed JavaScript runtime (goja) on the server.

## Plugin Format

Every plugin is a `.js` file with metadata comments and exported functions:

```javascript
// @name Plugin Name (required)
// @description What this plugin does
// @author Author name
// @version 1.0.0
// @config param_name type "Human-readable description"

function onRequest(ctx) {
  // Called BEFORE the HTTP request is sent
  // Modify ctx.req to transform the outgoing request
}

function onResponse(ctx) {
  // Called AFTER the HTTP response is received
  // Read ctx.res and optionally reply()
}
```

## Metadata Fields

| Field | Required | Description |
|---|---|---|
| `@name` | Yes | Plugin display name |
| `@description` | No | Short description |
| `@author` | No | Author name |
| `@version` | No | Semver version (default: 1.0.0) |
| `@config` | No | Configurable parameter (can have multiple) |

### @config Syntax

```
// @config <name> <type> "<description>"
```

Types: `string`, `string?` (optional), `number`, `bool`

Example:
```javascript
// @config webhook_url string "Target webhook URL"
// @config secret string? "Signing secret (optional)"
// @config max_retries number "Maximum retry count"
```

## Context Object (ctx)

### ctx.msg — Inbound Message (read-only)

| Field | Type | Description |
|---|---|---|
| `ctx.msg.event` | string | Always `"message"` |
| `ctx.msg.channel_id` | string | Channel ID |
| `ctx.msg.bot_id` | string | Bot ID |
| `ctx.msg.seq_id` | number | Message sequence ID |
| `ctx.msg.sender` | string | Sender ID (e.g. `user@im.wechat`) |
| `ctx.msg.msg_type` | string | `text`, `image`, `voice`, `video`, `file` |
| `ctx.msg.content` | string | Text content or media description |
| `ctx.msg.timestamp` | number | Unix timestamp in milliseconds |
| `ctx.msg.items` | array | Message items (see below) |

#### ctx.msg.items[]

Each item has:
| Field | Type | Description |
|---|---|---|
| `type` | string | `text`, `image`, `voice`, `video`, `file` |
| `text` | string | Text content or voice transcription |
| `file_name` | string | Original file name |
| `media_url` | string | Download URL (if available) |
| `file_size` | number | File size in bytes |
| `play_time` | number | Voice duration in seconds |
| `ref_title` | string | Quoted message title |

### ctx.req — HTTP Request (modifiable)

| Field | Type | Description |
|---|---|---|
| `ctx.req.url` | string | Target URL (from channel webhook config) |
| `ctx.req.method` | string | HTTP method (default: `POST`) |
| `ctx.req.headers` | object | Request headers (key-value) |
| `ctx.req.body` | string | Request body (default: JSON of ctx.msg) |

### ctx.res — HTTP Response (read-only, only in onResponse)

| Field | Type | Description |
|---|---|---|
| `ctx.res.status` | number | HTTP status code |
| `ctx.res.headers` | object | Response headers |
| `ctx.res.body` | string | Response body |

## Global Functions

| Function | Description |
|---|---|
| `reply(text)` | Send a text message back to the sender via the bot (max 10 per execution) |
| `skip()` | Cancel this webhook delivery (no HTTP request will be made) |
| `JSON.parse(str)` | Parse JSON string |
| `JSON.stringify(obj)` | Serialize to JSON string |

## Sandbox Restrictions

- **5-second timeout** — script is terminated if it runs too long
- **Max call stack depth: 64** — prevents stack overflow
- **No `eval()` or `new Function()`** — code injection prevention
- **No `require()`, `process`, `fs`, `net`** — no system access
- **`reply()` limited to 10 calls** — prevents message spam
- **HTTP sent by Hub** — scripts cannot make their own network requests; they only modify `ctx.req` which Hub sends

## Examples

### 1. Notification Forward (onRequest)

Transforms the request body to match the target service's format. Works for Feishu, DingTalk, Slack, Discord, WeCom — just change the JSON structure.

```javascript
// @name Feishu Notification
// @description Forward WeChat messages to Feishu group bot
// @author openilink
// @version 1.0.0

function onRequest(ctx) {
  ctx.req.body = JSON.stringify({
    msg_type: "text",
    content: {
      text: "[" + ctx.msg.msg_type + "] " + ctx.msg.sender + ": " + ctx.msg.content
    }
  });
}
```

### 2. AI Auto-Reply (onRequest + onResponse + reply)

Demonstrates the full two-phase flow: transform the request, then parse the response and reply back through the bot.

```javascript
// @name ChatGPT Auto-Reply
// @description Forward to OpenAI API and auto-reply
// @author openilink
// @version 1.0.0
// @config api_key string "OpenAI API Key"
// @config model string? "Model name (default: gpt-4o-mini)"

function onRequest(ctx) {
  ctx.req.url = "https://api.openai.com/v1/chat/completions";
  ctx.req.headers["Authorization"] = "Bearer YOUR_API_KEY";
  ctx.req.body = JSON.stringify({
    model: "gpt-4o-mini",
    messages: [
      { role: "system", content: "You are a helpful assistant." },
      { role: "user", content: ctx.msg.content }
    ]
  });
}

function onResponse(ctx) {
  var data = JSON.parse(ctx.res.body);
  if (data.choices && data.choices[0]) {
    reply(data.choices[0].message.content);
  }
}
```

### 3. Conditional Filter (skip)

Uses `skip()` to cancel delivery when conditions aren't met.

```javascript
// @name Keyword Filter
// @description Only forward messages containing specific keywords
// @author openilink
// @version 1.0.0
// @config keywords string "Keywords, comma-separated"

function onRequest(ctx) {
  var keywords = ["urgent", "bug", "error", "help"];
  var found = false;
  for (var i = 0; i < keywords.length; i++) {
    if (ctx.msg.content.toLowerCase().indexOf(keywords[i]) >= 0) {
      found = true;
      break;
    }
  }
  if (!found) {
    skip();
    return;
  }
  ctx.req.body = JSON.stringify({
    text: "[ALERT] " + ctx.msg.sender + ": " + ctx.msg.content
  });
}
```

## Submission Process

1. Host your plugin `.js` file in a public GitHub repository
2. Go to the OpenILink Hub plugin marketplace and click "Submit Plugin"
3. Paste the GitHub blob URL (e.g. `https://github.com/user/repo/blob/main/my-plugin.js`)
4. Or paste the script content directly
5. The system fetches the code and pins the commit hash
6. An admin reviews and approves/rejects the plugin
7. Once approved, other users can install it with one click

## API Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/webhook-plugins` | No | List approved plugins |
| GET | `/api/webhook-plugins?status=pending` | Admin | List pending plugins |
| GET | `/api/webhook-plugins/{id}` | No | Plugin detail |
| POST | `/api/webhook-plugins/submit` | Yes | Submit plugin (github_url or script) |
| POST | `/api/webhook-plugins/{id}/install` | Yes | Install (get script + increment count) |
| PUT | `/api/admin/webhook-plugins/{id}/review` | Admin | Approve or reject |
| DELETE | `/api/admin/webhook-plugins/{id}` | Admin | Delete plugin |

## Tips for AI Agents

When generating a plugin:

1. Always include `// @name` — submission will fail without it
2. Use `JSON.stringify()` to set `ctx.req.body` — it must be a string
3. Use `JSON.parse()` to read `ctx.res.body` in onResponse
4. Call `skip()` to conditionally cancel delivery
5. Call `reply(text)` to send a message back through the bot
6. Don't use ES6+ syntax (no arrow functions, no const/let, no template literals) — the runtime is ES5
7. Don't try to access external resources — the sandbox blocks all I/O
8. Keep the script simple and focused — complex logic should live in the webhook receiver

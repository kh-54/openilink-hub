# 三方服务接收与回复（极简）

## 1) 三方服务如何接收 Hub 消息

你的服务提供一个 HTTP 接口，例如：

- `POST /wechat/webhook`
- `Content-Type: application/json`

Hub 会 POST 这样的消息体：

```json
{
  "event": "message",
  "channel_id": "ch_xxx",
  "bot_id": "bot_xxx",
  "seq_id": 123,
  "sender": "wxid_xxx",
  "msg_type": "text",
  "content": "你好",
  "timestamp": 1710000000000,
  "items": [
    { "type": "text", "text": "你好" }
  ]
}
```

## 2) 三方服务如何返回消息

你在 webhook 响应里直接返回：

```json
{ "reply": "收到啦" }
```

Hub 会把这条文本消息回给当前发送者。

## 3) 最小 Node.js 示例

```js
import express from "express";

const app = express();
app.use(express.json());

app.post("/wechat/webhook", (req, res) => {
  const msg = req.body;
  console.log(msg.sender, msg.msg_type, msg.content);
  return res.json({ reply: "收到啦" });
});

app.listen(8080, () => console.log("listening on :8080"));
```


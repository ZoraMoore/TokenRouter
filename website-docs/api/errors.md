# 错误处理与排查

网关会尽量保持客户端协议的错误格式：OpenAI 返回 `error` 对象，Anthropic 返回 `type: "error"`，Gemini 返回 Google 风格的 `code/message/status`。

## 常见 HTTP 状态码

| 状态码 | 常见原因 | 建议 |
| --- | --- | --- |
| `400` | JSON、参数或模型名不合法 | 检查请求体，并从模型列表复制模型 ID |
| `401` | Key 缺失、错误、禁用或过期 | 检查鉴权 Header；在控制台确认 Key 状态 |
| `402` | 余额、订阅或 Key 配额不足 | 查看 `/v1/usage` 与控制台用量 |
| `403` | 分组未开放当前协议、IP 限制或策略拒绝 | 换用允许的协议/分组，检查 Key 限制 |
| `404` | 路径错误，或当前平台不支持该能力 | 检查 Base URL、路径和分组平台 |
| `429` | 请求速率、并发、滚动额度或上游限流 | 读取错误体与 `Retry-After`，退避后重试 |
| `5xx` | 临时上游或网关故障 | 有限重试；持续发生时保留 Request ID 反馈 |

## 先做这四个测试

### 1. 确认域名与 TLS

```bash
curl -I https://api.scienceedu.me/health
```

### 2. 确认 Key 和模型列表

```bash
curl -i https://api.scienceedu.me/v1/models \
  -H "Authorization: Bearer sk-YOUR_API_KEY"
```

### 3. 确认 Key 额度

```bash
curl -i https://api.scienceedu.me/v1/usage \
  -H "Authorization: Bearer sk-YOUR_API_KEY"
```

### 4. 用最小请求测试协议

去掉工具调用、图片、超长上下文和复杂参数，只保留一个短输入。如果最小请求成功，再逐项恢复参数。

## Base URL 典型错误

| 实际请求 | 问题 |
| --- | --- |
| `/v1/v1/responses` | SDK 已追加 `/v1`，配置里又重复添加 |
| `/responses` 返回异常 | 客户端期望 OpenAI Base URL 带 `/v1` |
| `/v1/messages` 变成 `/v1/v1/messages` | Anthropic 客户端应配置站点根地址 |
| 请求发到 `docs.scienceedu.me` | 文档域名不承载 API，请改用 `api.scienceedu.me` |

## 重试建议

仅对临时错误重试：网络超时、`429` 和部分 `5xx`。使用指数退避并加随机抖动，例如 1 秒、2 秒、4 秒；通常最多 2–3 次。不要无条件重试 `400`、`401`、`402`、`403`。

流式请求在收到部分输出后失败时，自动重试可能产生重复内容或重复扣费。应用需要记录请求状态并自行决定是否重新提交。

## 反馈问题时提供

- 发生时间与时区；
- 请求接口和模型 ID；
- HTTP 状态码；
- 脱敏后的错误响应；
- 响应 Header 中的 `X-Request-ID`；
- 客户端名称和版本。

不要提供完整 API Key、对话隐私内容或未脱敏的配置文件。

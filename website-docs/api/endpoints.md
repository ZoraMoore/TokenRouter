# API 端点

大多数客户端在配置时都需要填写一个 API 地址（Base URL）。云桥词元 API 使用统一的默认域名，根据客户端协议选择对应格式即可。

## 端点一览

| 用途 | OpenAI 格式 | Anthropic 格式 |
| --- | --- | --- |
| 默认 | `https://api.scienceedu.me/v1` | `https://api.scienceedu.me` |

新配置请使用以上默认端点。云桥词元 API 没有需要切换的备用或加速域名。

::: tip 请求量较大时也使用默认端点

无论请求量大小，都请使用 `api.scienceedu.me`。客户端侧可通过合理的超时、有限重试和错误记录提高稳定性，详见[错误处理与排查](/api/errors)。

:::

默认端点也可在 [API 密钥页面](https://api.scienceedu.me/keys) 中查看和复制。

## 该选哪个格式

看客户端要求，不是看模型。

- **OpenAI 格式**（以 `/v1` 结尾）：绝大多数客户端使用，例如 Cherry Studio、RikkaHub、OpenCode、WorkBuddy，以及任何标注“OpenAI 兼容”的工具。
- **Anthropic 格式**（不带 `/v1`）：`Claude Code` 等原生走 Anthropic 协议的客户端使用。
- **Gemini 原生格式**（不带 `/v1`）：Gemini SDK、Gemini CLI 等客户端使用 `https://api.scienceedu.me`，客户端会请求 `/v1beta/models/...`。

如果客户端配置项是 `ANTHROPIC_BASE_URL`，使用 Anthropic 格式；配置项是 `OPENAI_BASE_URL` 或 `Base URL` 时，一般使用 OpenAI 格式。各客户端教程均已注明具体填写方式。

::: warning 不要重复添加路径

OpenAI 兼容客户端通常会在 Base URL 后自动追加 `chat/completions`、`responses` 等路径；请不要手动添加。若最终请求变成 `/v1/v1/...`，请去掉其中一个 `/v1`。

:::

## 接口列表

| 方法与路径 | 协议 / 用途 |
| --- | --- |
| `GET /v1/models` | 查询当前 Key 可见模型 |
| `GET /v1/usage` | 查询 Key 额度与计费来源 |
| `POST /v1/responses` | OpenAI Responses API |
| `POST /v1/chat/completions` | OpenAI Chat Completions |
| `POST /v1/embeddings` | OpenAI Embeddings；仅在分组与上游支持时可用 |
| `POST /v1/images/generations` | OpenAI 风格图片生成 |
| `POST /v1/images/edits` | OpenAI 风格图片编辑 |
| `POST /v1/messages` | Anthropic Messages API |
| `POST /v1/messages/count_tokens` | 消息 token 预估 |
| `GET /v1beta/models` | Gemini 原生模型列表 |
| `POST /v1beta/models/{model}:generateContent` | Gemini 非流式生成 |
| `POST /v1beta/models/{model}:streamGenerateContent` | Gemini 流式生成 |
| `POST /v1beta/models/{model}:countTokens` | Gemini token 统计 |

图片、视频、实时语音等能力只对匹配的平台、模型与分组开放。返回 `404` 不一定表示地址拼错，也可能表示该分组不支持该能力。

## 鉴权

推荐统一使用 Bearer Token：

```http
Authorization: Bearer sk-YOUR_API_KEY
```

Anthropic SDK 常使用：

```http
x-api-key: sk-YOUR_API_KEY
anthropic-version: 2023-06-01
```

Gemini SDK 也可使用：

```http
x-goog-api-key: sk-YOUR_API_KEY
```

## 请求关联 ID

遇到问题时保存响应中的 `X-Request-ID`。应用也可以主动传入合法的 `X-Request-ID` 或 `X-Client-Request-ID`，便于把客户端日志与服务端请求对应起来。

```bash
curl https://api.scienceedu.me/v1/models \
  -H "Authorization: Bearer sk-YOUR_API_KEY" \
  -H "X-Request-ID: my-test-20260820-001" \
  -i
```

不要在工单或群聊中提供完整 API Key。排查通常只需要时间、接口、模型、HTTP 状态码、Request ID 和脱敏后的错误体。

## 相关入口

- [创建 API Key](/guide/api-key)：先获取密钥；
- [快速开始](/guide/quickstart)：按客户端选择接入方式；
- [常见问题](/help/faq)：连接与使用中的常见问题；
- [错误处理与排查](/api/errors)：HTTP 错误与请求失败处理。

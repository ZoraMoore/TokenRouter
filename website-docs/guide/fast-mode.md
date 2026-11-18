# Fast 模式

Fast 模式会向上游请求更高优先级的算力通道，通常能缩短排队时间并加快响应。它直接绑定在 API Key 上，客户端无需额外修改配置。

这对 `Codex` 桌面端尤其方便：即使客户端界面没有 Fast 开关，也可以把 API Key 的策略设为“强制开启 Fast”，无需安装插件或改写请求。

::: warning 费用通常约为常规模式的 2 倍

Fast 模式使用上游高优先级通道，实际扣费通常约为常规模式的 **2 倍**。不同模型、分组和渠道的价格可能不同；开启前请以[模型广场](https://api.scienceedu.me/models)和控制台显示的价格为准。

:::

## 设置方式

1. 打开 [API 密钥页面](https://api.scienceedu.me/keys)。
2. 创建新密钥，或在已有密钥上点击“编辑”。
3. 找到“Fast 模式策略”，选择所需选项。
4. 保存。

## 三个选项

| 选项 | 行为 |
| --- | --- |
| `跟随请求`（默认） | 完全按客户端请求中携带的优先级参数处理，网关不额外增加或移除 Fast 标记。 |
| `强制开启 Fast` | 客户端没有请求 Fast 时，也会按 Fast 模式向符合条件的上游发起请求。 |
| `强制关闭 Fast` | 移除客户端已携带的 Fast 标记，按常规优先级发送。 |

“强制关闭 Fast”只会移除 Fast，不会影响 `flex`、`auto`、`default`、`scale` 等其他优先级设置。

## 生效范围

策略对该 API Key 的所有兼容请求生效，不限于 Codex，包括 OpenAI Chat Completions、Responses，以及 Anthropic Messages 格式的请求。

保存后立即生效。对于 WebSocket 长连接，系统会在下一轮对话读取新策略，不需要重新连接或重启客户端。

## 强制开启未生效的情况

“强制开启 Fast”需要当前请求满足上游能力条件；不满足时会**静默跳过**，请求仍按常规模式发送：

- 当前 API Key 命中的不是 OpenAI 或 Anthropic 平台账号；
- Anthropic 账号使用 Bedrock、Vertex 或 OAuth 凭据；只有 Anthropic API Key 直连账号支持由 Key 策略注入 Fast；
- 当前模型或其定价配置不支持优先级通道；
- 平台管理员配置了全局 Fast 过滤或拦截策略。

::: tip 管理员策略优先级更高

平台的全局 Fast 策略高于单个 API Key 的设置。若管理员配置了过滤或拦截，Key 的“强制开启 Fast”无法绕过该限制。

:::

## 相关入口

- [创建 API Key](/guide/api-key)：创建、编辑和管理密钥；
- [Codex 使用指南](/clients/codex)：将 Codex 接入云桥词元 API；
- [模型、分组与计费](/guide/models-and-billing)：查看当前模型、分组和计费信息；
- [API 密钥页面](https://api.scienceedu.me/keys)：直接设置 Fast 模式策略。

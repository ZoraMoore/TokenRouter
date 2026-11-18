# 快速开始

云桥词元 API 是一个模型中转服务：使用一个 API Key，即可在熟悉的客户端中调用当前分组允许的模型。

本页帮你规划接入路线，三步即可开始使用。

::: tip 访问提示

不同地区和网络环境对主站、客户端及上游服务的可访问性可能不同。API 请求请使用云桥词元默认端点 `https://api.scienceedu.me`；OpenAI 兼容客户端通常填写 `https://api.scienceedu.me/v1`。

:::

## 三步接入

| 步骤 | 做什么 | 去哪 |
| --- | --- | --- |
| 1 | 充值或订阅，准备可用额度 | [充值／订阅](https://api.scienceedu.me/purchase) · [模型、分组与计费](/guide/models-and-billing) |
| 2 | 创建 API Key，并选择分组 | [创建 API Key](/guide/api-key) |
| 3 | 在客户端填写 Key 和 API 端点 | 见下方客户端选择 |

模型、分组和价格会随上游能力调整。创建 Key 后，以控制台中的“使用”弹窗及该 Key 调用 `/v1/models` 返回的模型列表为准。

## 选择你的客户端

::: details Agent

适合在终端工作、有编程需求，或希望使用 AI Agent 辅助开发的用户。

推荐先使用 [CC-Switch](/clients/cc-switch) 统一管理 API Key 和供应商配置，减少手动修改环境变量或本地配置文件的操作。随后可按使用的客户端继续配置：

| 客户端 | 适用场景 | 指南 |
| --- | --- | --- |
| Claude Code | Anthropic 官方 CLI，终端编程 | [教程](/clients/claude-code) |
| Codex | OpenAI 官方 CLI 与桌面端 | [教程](/clients/codex) |
| Codex++ | 为 Codex App 增加扩展能力 | [教程](/clients/codex-plus-plus) |
| OpenCode | 开源终端 Agent，多模型切换 | [教程](/clients/opencode) |
| Hermes | 轻量终端 Agent | [教程](/clients/hermes) |
| WorkBuddy | 桌面 AI Agent 工作台 | [教程](/clients/workbuddy) |

:::

::: details ChatBot

适合偏好图形界面、移动端，或以日常对话为主的用户。

| 客户端 | 适用场景 | 指南 |
| --- | --- | --- |
| Cherry Studio | Windows、macOS、Linux 桌面端，多模型切换 | [教程](/clients/cherry-studio) |
| RikkaHub | Android 手机端，OpenAI 兼容 | [教程](/clients/rikkahub) |

在客户端中添加“云桥词元”作为自定义服务商，填写 API Key 和 [API 端点](/api/endpoints)。刷新模型列表后，选择当前 Key 可见的模型即可开始对话。

:::

## 验证连接

如果尚未选择客户端，先用下列命令验证 API Key。将占位符替换为真实 Key：

```bash
curl https://api.scienceedu.me/v1/models \
  -H "Authorization: Bearer sk-YOUR_API_KEY"
```

成功时会返回该 Key 当前分组允许使用的模型。选择返回结果中的一个 `id`，即可调用对应协议。

### OpenAI Responses 示例

```bash
curl https://api.scienceedu.me/v1/responses \
  -H "Authorization: Bearer sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MODEL_ID_FROM_V1_MODELS",
    "input": "请用一句话介绍你自己"
  }'
```

### Anthropic Messages 示例

```bash
curl https://api.scienceedu.me/v1/messages \
  -H "x-api-key: sk-YOUR_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{
    "model": "MODEL_ID_FROM_V1_MODELS",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "请用一句话介绍你自己"}]
  }'
```

::: tip 协议由分组决定

API Key 所属分组必须允许对应协议。若收到 `403` 或 `protocol_not_allowed`，请改用该分组支持的客户端协议，或创建绑定正确分组的 Key。

:::

## 进阶用法

- [复合 Key](/guide/composite-key)：一个 Key 绑定多个分组，通过模型前缀切换；
- [Fast 模式](/guide/fast-mode)：在 Key 上强制开启或关闭高优先级通道；
- [模型、分组与计费](/guide/models-and-billing)：查看当前可用模型、分组和计费信息。

## 遇到问题

- [常见问题](/help/faq)：连接、模型检测和客户端配置等问题；
- [API 端点](/api/endpoints)：端点选择与鉴权格式。

## 商务合作

::: tip 联系方式

如有企业级接入、API 对接、渠道合作或定制服务需求，欢迎添加微信沟通。

- 微信：`scienceedu_me`
- 添加好友时请备注：`合作`
- 为便于快速对接，建议同时简要说明公司、业务场景与预计用量。

:::

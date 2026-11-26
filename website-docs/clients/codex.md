# Codex 使用指南

`Codex` 是一款专为代码生成、修改和审查设计的 AI Agent 工具。

## 安装

根据使用习惯选择安装方式。

::: details Codex App

`Codex App` 适合使用图形界面的用户。

按系统选择对应安装包：

- [Codex App for Windows](https://get.microsoft.com/installer/download/9PLM9XGG6VKS?cid=website_cta_psi)
- [Codex App for macOS（Apple Silicon）](https://persistent.oaistatic.com/codex-app-prod/Codex.dmg)
- [Codex App for macOS（Intel）](https://persistent.oaistatic.com/codex-app-prod/Codex-latest-x64.dmg)

下载完成后，按照系统提示完成安装并启动即可。

:::

::: details Codex CLI

推荐在终端中使用 `Codex CLI`。

安装方式：

```bash
npm install -g @openai/codex
```

验证安装：

```bash
codex --help
```

命令能正常输出帮助信息即表示安装成功。

:::

::: details npx 直接调用

无需全局安装，可直接使用 `npx` 按需调用 `Codex`：

```bash
npx @openai/codex
```

第一次运行时，`npx` 会自动下载并执行 `Codex`。适合以下场景：

- 不想污染全局环境
- 在某台机器上按需运行一次
- 验证 CLI 是否满足需求

如果后续经常使用，建议全局安装以获得更快的启动速度。

:::

## 导入

首次接入或更新 GPT 型号时，也可以使用[云桥词元配置 Skill](/clients/yunqiao-skill)，通过 CC Switch 查询模型、保存配置并检查生效情况。

安装完成后，选择以下两种方式之一将 `Codex` 接入云桥词元 API。

::: details 使用 CC-Switch

推荐使用 `CC-Switch` 统一管理配置。

操作步骤：

1. 按 [创建 API Key 教程](/guide/api-key) 生成 API Key。
2. 按 [CC-Switch](/clients/cc-switch) 完成统一供应商配置。
3. 配置完成后，重启 `Codex` 或 `Codex App`。

:::

::: details 手动填写

**第一步：确认配置目录**

`Codex` 的本地配置目录通常是：

- Windows：`%userprofile%\.codex`
- macOS / Linux：`~/.codex`

如果你在 VSCode 或 Zed 中使用 `Codex`，通常同样走 `Codex` 的全局配置；按本节方式写入配置后，重启编辑器即可生效。

建议先启动一次 `Codex` 或 `Codex App`，让程序自动初始化配置目录。

**第二步：写入 `config.toml`**

在配置目录中创建或编辑 `config.toml`，确保以下内容位于文件前部：

```toml
model_provider = "scienceedu"
model = "MODEL_ID_FROM_V1_MODELS"
review_model = "MODEL_ID_FROM_V1_MODELS"
model_reasoning_effort = "xhigh"
network_access = true

[model_providers.scienceedu]
name = "OpenAI"
base_url = "https://api.scienceedu.me/v1"
env_key = "SCIENCEEDU_API_KEY"
wire_api = "responses"
requires_openai_auth = false
```

将 `MODEL_ID_FROM_V1_MODELS` 替换为当前 API Key 调用 `/v1/models` 后返回的模型 ID。

**第三步：设置 API Key**

云桥词元 API 使用自定义 provider 的环境变量认证，不需要把 Key 写入 `auth.json`。

**macOS / Linux**

```bash
export SCIENCEEDU_API_KEY="sk-YOUR_API_KEY"
```

**Windows PowerShell**

```powershell
$env:SCIENCEEDU_API_KEY="sk-YOUR_API_KEY"
```

**Windows CMD**

```cmd
set SCIENCEEDU_API_KEY=sk-YOUR_API_KEY
```

将 `sk-YOUR_API_KEY` 替换为你的真实 API Key。

**WebSocket 版本（可选）**

只有当当前 Key 所属分组明确支持 Responses WebSocket 时，才在 provider 中额外配置：

```toml
supports_websockets = true
```

:::

## 关于远程压缩

上方配置把 `[model_providers.scienceedu]` 的 `name` 设为 `OpenAI`，以保持 Codex 对 OpenAI Responses 兼容 provider 的远程压缩识别。

`Codex` 在长对话接近上下文上限时会触发压缩。云桥词元 API 兼容 `/v1/responses/compact` 路径；保持 `name = "OpenAI"` 和 `wire_api = "responses"`，可让 Codex 使用该兼容链路。

说明：

- `name` 是用于客户端识别的显示名，保持 `OpenAI` 即可。
- provider 标识 `scienceedu`（即 `model_provider` 和 `[model_providers.scienceedu]`）不受影响，保持不变。
- 这个设置不会丢失已有聊天记录。

## 1M 上下文窗口

是否可使用 1M 上下文取决于当前 API Key 的分组和模型。请先以 `/v1/models`、模型广场和控制台显示的实际能力为准；未明确提供 1M 上下文时，不要强行修改窗口配置。

### 安装 Skill

如果当前分组已提供 1M 上下文模型，可以将第三方 Skill 克隆到 `Codex` 的用户 Skill 目录：

```bash
git clone https://github.com/smartcmd/codex-context-window.git ~/.codex/skills/codex-context-window
```

也可以把下面这段直接发给 `Codex`，让它根据当前可用模型完成配置：

```text
安装这个 Skill：https://github.com/smartcmd/codex-context-window

然后将当前 API Key 可用的模型上下文窗口调整为 1M，自动压缩阈值设置为 900k。
```

### 配置模型

新开一个任务让 `Codex` 发现 Skill，然后发送：

```text
将当前 API Key 可用的模型上下文窗口调整为 1M，自动压缩阈值设置为 900k。
```

Skill 会依次确认目标模型、原始窗口大小、有效窗口比例和自动压缩策略，确认后才写入配置。配置完成后重启 `Codex`。

::: tip 有效比例保持默认
有效比例保持默认的 `95%` 即可。此时 1M 原始窗口对应的可用上下文是 `950000` token，状态栏显示的也是这个折算后的数字。
:::

### 确认是否生效

- `Codex App`：在 **设置 → 常规 → 编辑器** 中开启 **显示上下文窗口使用情况**，新开一条消息即可看到窗口大小。
- `Codex CLI`：输入 `/status`，在输出中查看 **Context window**。

## 关于 codex-auto-review

`codex-auto-review` 是否可用以及最终路由到哪个模型，取决于当前 API Key 的分组配置。请以控制台“使用 Key”页面或 `/v1/models` 返回结果为准。

如需降低成本，可在 API Key 的模型路由配置中将其映射到该分组允许的其他模型；修改后重新启动 Codex 使配置生效。

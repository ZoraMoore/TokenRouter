# OpenCode 使用指南

`OpenCode` 是一款开源的 AI 编程助手框架，支持多种 AI 模型集成，可用于代码生成、修改和审查。

## 安装

根据使用习惯选择一种安装方式。

::: details 脚本安装

**macOS / Linux**

```bash
curl -fsSL https://opencode.ai/install | bash
```

**Windows PowerShell**

推荐在 WSL 环境中，按上述 macOS / Linux 方式安装。

:::

::: details npm 安装

全局安装 `OpenCode`：

```bash
npm install -g opencode-ai
```

安装完成后，直接在终端运行 `opencode` 即可启动。

:::

::: details Homebrew

**macOS / Linux**

```bash
brew install anomalyco/tap/opencode
```

:::

::: details Windows

除 WSL 外，还可使用以下包管理器：

**Chocolatey**

```cmd
choco install opencode
```

**Scoop**

```cmd
scoop install opencode
```

推荐优先使用 WSL 环境，以获得更好的兼容性。

:::

## 接入云桥词元 API

安装完成后，选择以下任一方式将 `OpenCode` 接入云桥词元 API。

::: details 使用 CC-Switch

推荐使用 `CC-Switch` 统一管理多个 AI 客户端的配置。

1. 在 [创建 API Key](/guide/api-key) 中生成云桥词元 API Key。
2. 按照 [CC-Switch 使用指南](/clients/cc-switch) 完成统一供应商配置。
3. 配置完成后，重启 `OpenCode`。

:::

::: details 手动填写

**第一步：创建配置文件**

进入需要使用 OpenCode 的项目目录，创建 `opencode.json` 文件。

**第二步：填写配置**

将以下内容复制到 `opencode.json`，再把 `MODEL_ID_FROM_V1_MODELS` 替换为[模型、分组与计费](/guide/models-and-billing)中实际可用的模型 ID：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "scienceedu": {
      "npm": "@ai-sdk/openai",
      "name": "云桥词元 API",
      "options": {
        "baseURL": "https://api.scienceedu.me/v1",
        "apiKey": "{env:SCIENCEEDU_API_KEY}"
      },
      "models": {
        "MODEL_ID_FROM_V1_MODELS": {
          "name": "云桥词元模型"
        }
      }
    }
  },
  "model": "scienceedu/MODEL_ID_FROM_V1_MODELS",
  "small_model": "scienceedu/MODEL_ID_FROM_V1_MODELS"
}
```

然后在终端中设置 API Key。请将 `sk-YOUR_API_KEY` 替换为你在控制台创建的真实 Key。

**macOS / Linux**

```bash
export SCIENCEEDU_API_KEY="sk-YOUR_API_KEY"
```

**Windows PowerShell**

```powershell
$env:SCIENCEEDU_API_KEY="sk-YOUR_API_KEY"
```

`OpenCode` 会通过配置中的 `scienceedu` provider 调用云桥词元 API。不要将真实 API Key 直接写入并提交到 `opencode.json`。

配置完成后运行以下命令，确认模型已经正确加载：

```bash
opencode models
```

**第三步：启动 OpenCode**

进入项目目录后运行：

```bash
opencode
```

然后执行：

```text
/init
```

`/init` 会在项目中创建 `AGENTS.md`，用于记录项目协作说明；如需团队共享，建议将其纳入版本控制。

:::

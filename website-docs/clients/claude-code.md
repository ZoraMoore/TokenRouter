# Claude Code 使用指南

`Claude Code` 是 Anthropic 推出的 AI 编程助手，支持代码生成、修改和审查。

## 安装

根据使用习惯选择安装方式。

::: details 脚本安装

**macOS / Linux**

```bash
curl -fsSL https://claude.ai/install.sh | bash
```

**Windows PowerShell**

```powershell
irm https://claude.ai/install.ps1 | iex
```

**Windows CMD**

推荐先安装 [Git for Windows](https://git-scm.com/download/win)，然后运行：

```cmd
curl -fsSL https://claude.ai/install.cmd -o install.cmd && install.cmd && del install.cmd
```

:::

::: details Homebrew

**macOS / Linux**

```bash
brew install --cask claude-code
```

:::

::: details WinGet

**Windows**

```powershell
winget install Anthropic.ClaudeCode
```

:::

::: details npm 安装

全局安装 `Claude Code`：

```bash
npm install -g @anthropic-ai/claude-code
```

安装完成后，直接在终端运行 `claude` 即可启动。

:::

::: details npx 启动

无需全局安装，可直接使用 `npx` 按需调用 `Claude Code`：

```bash
npx @anthropic-ai/claude-code
```

第一次运行时，`npx` 会自动下载并执行 `Claude Code`。

:::

## 导入

安装完成后，选择以下两种方式之一将 `Claude Code` 接入云桥词元 API。

::: details 使用 CC-Switch

推荐使用 `CC-Switch` 统一管理配置。

操作步骤：

1. 按 [创建 API Key 教程](/guide/api-key) 生成 API Key。
2. 按 [CC-Switch](/clients/cc-switch) 完成统一供应商配置。
3. 配置完成后，重启 `Claude Code`。

:::

::: details 手动填写

**第一步：设置环境变量**

根据操作系统选择对应方式设置环境变量：

如果你在 VSCode 或 Zed 中使用 `Claude Code`，通常同样走 `Claude Code` 的全局配置；按本节方式配置后，重启编辑器即可生效。

**macOS / Linux**

在终端中运行：

```bash
export ANTHROPIC_BASE_URL="https://api.scienceedu.me"
export ANTHROPIC_AUTH_TOKEN="sk-YOUR_API_KEY"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

将 `sk-YOUR_API_KEY` 替换为你的真实 API Key。

**Windows PowerShell**

```powershell
$env:ANTHROPIC_BASE_URL="https://api.scienceedu.me"
$env:ANTHROPIC_AUTH_TOKEN="sk-YOUR_API_KEY"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

**Windows CMD**

```cmd
set ANTHROPIC_BASE_URL=https://api.scienceedu.me
set ANTHROPIC_AUTH_TOKEN=sk-YOUR_API_KEY
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

**第二步：配置 VSCode Claude Code（可选）**

如果 VSCode 中的 Claude Code 扩展未读取到 shell 环境变量，还可以通过 `settings.json` 显式配置：

**macOS / Linux 路径**

```text
~/.claude/settings.json
```

**Windows 路径**

```text
%userprofile%\.claude\settings.json
```

**settings.json 内容**

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.scienceedu.me",
    "ANTHROPIC_AUTH_TOKEN": "sk-YOUR_API_KEY",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}
```

将 `sk-YOUR_API_KEY` 替换为你的真实 API Key。

:::

## DOYO Claude 渠道

::: warning
DOYO 的 Claude 渠道暂时无法在 Claude 应用（Claude app）中使用，只能通过 `Claude Code` CLI 使用。
:::

如果你使用的是 DOYO 的 Claude 渠道，可以在完成上方基础配置后，额外开启 `ENABLE_PROMPT_CACHING_1H=1`。该选项会启用 1 小时 prompt cache，适合重复携带较长上下文的 Claude Code 会话。

该配置只对 DOYO Claude 渠道有效；其他 Claude 渠道无需设置。

如果你通过 `CC-Switch` 管理 Claude Code，请参考 [CC-Switch 的 DOYO Claude 渠道环境变量说明](/clients/cc-switch#doyo-claude-渠道环境变量)。

### 终端启动

按系统设置一次环境变量后，再启动 `Claude Code`。

**macOS / Linux**

```bash
export ENABLE_PROMPT_CACHING_1H=1
```

**Windows PowerShell**

```powershell
$env:ENABLE_PROMPT_CACHING_1H=1
```

**Windows CMD**

```cmd
set ENABLE_PROMPT_CACHING_1H=1
```

### VSCode / Zed

如果你通过 VSCode 或 Zed 启动 `Claude Code`，可以在 `~/.claude/settings.json` 或 `%userprofile%\.claude\settings.json` 的 `env` 中补充：

```json
{
  "env": {
    "ENABLE_PROMPT_CACHING_1H": "1"
  }
}
```

如果文件里已经有 `env`，只需要把 `ENABLE_PROMPT_CACHING_1H` 这一行合并进去，不要重复创建第二个 `env`。

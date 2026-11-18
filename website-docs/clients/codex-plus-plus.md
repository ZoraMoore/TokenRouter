# Codex++

::: danger 安全提示
`Codex++` 是第三方项目，不由云桥词元 API、OpenAI 或 Codex 官方维护。它会读取和管理本机 Codex 配置、会话数据，并通过 Chromium DevTools Protocol（CDP）向 Codex App 注入增强脚本。安装前请自行审查项目代码与发布包；不要把真实 API Key、登录凭据或敏感会话内容提交到 issue、截图或未知脚本。
:::

`Codex++` 是面向 Codex App 的外部增强启动器，通过 Chromium DevTools Protocol 注入增强脚本，**不修改 Codex 原始安装文件**。

项目地址：[BigPizzaV3/CodexPlusPlus](https://github.com/BigPizzaV3/CodexPlusPlus)

## 功能亮点

- **Codex++ 管理工具**：集中管理供应商、模型、工具插件、会话、增强功能、脚本、更新和诊断。
- **插件入口解锁**：API Key 模式下显示并启用插件入口。
- **特殊插件强制安装**：解除 App unavailable / 应用不可用导致的前端安装禁用。
- **会话删除**：悬停显示删除按钮，删除前确认并支持撤销。
- **Markdown 导出**：按本地 rollout 导出带时间戳的会话 Markdown。
- **会话项目移动**：把会话移动到普通对话或其他本地项目。
- **对话 Timeline**：右侧显示用户提问时间线，悬停摘要，点击跳转。
- **Provider 同步**：切换 model_provider 或供应商时不丢历史会话。
- **Windows 快捷方式、卸载项**，可选 watcher 自动接管。
- **macOS `/Applications/Codex++.app`** 与管理工具应用。

## 如何配置

你可以将项目地址直接交给 Agent 协助阅读安装说明，也可以手动操作。

### 交给 Agent

将以下项目链接交给你的 AI Agent，并告诉它“请你按这个项目的说明帮我安装并配置 Codex++”：

```text
https://github.com/BigPizzaV3/CodexPlusPlus
```

Agent 可以拉取项目并执行安装步骤。执行前请先确认它展示的命令和目标路径，不要授权它上传 `~/.codex/`、API Key 或本地会话数据。

### 手动配置

从 [Codex++ Releases](https://github.com/BigPizzaV3/CodexPlusPlus/releases) 下载与系统匹配的最新安装包：

- Windows：`CodexPlusPlus-*-windows-x64-setup.exe`
- macOS Intel：`CodexPlusPlus-*-macos-x64.dmg`
- macOS Apple Silicon：`CodexPlusPlus-*-macos-arm64.dmg`

安装后会出现两个入口：

- `Codex++`：启动官方 Codex App，并加载已保存的供应商配置与增强功能。
- `Codex++ 管理工具`：管理供应商、模型、插件、会话、增强功能、更新和诊断。

首次使用建议先打开“Codex++ 管理工具”，确认 Codex App 路径和运行状态，再配置供应商与增强功能，最后从 `Codex++` 入口启动。

## 配置云桥词元 API

1. 在云桥词元主站创建一个允许 OpenAI Responses 协议的 API Key。
2. 在 Codex++ 管理工具中新增“纯 API”或自定义供应商。
3. 按下列信息填写：

| 字段 | 填写内容 |
| --- | --- |
| 供应商名 | `云桥词元 API` |
| Base URL | `https://api.scienceedu.me/v1` |
| 协议 | `Responses` |
| API Key | 你的 `sk-...` API Key |
| 测试 / 默认模型 | 当前 Key 的 `/v1/models` 返回的模型 ID |

4. 在管理工具中运行模型测试或 Provider Doctor，确认地址、协议、Key 和模型都可用。
5. 保存后从 `Codex++` 入口启动 Codex App。

::: tip API 地址
Codex++ 的纯 API 配置使用带 `/v1` 的地址：`https://api.scienceedu.me/v1`。模型必须以当前 Key 的 `/v1/models` 返回结果为准，不要直接复制其他用户的模型名称。
:::

## 工作方式

1. 外部启动 Codex App，并附加 CDP 参数。
2. 启动本地 helper 服务，用于健康检查、设置、导出、移动、删除等操作。
3. 通过 CDP 注入渲染端增强脚本。
4. 渲染端通过 CDP bridge 调用本地服务。
5. 启动时继承现有代理环境变量；若未设置，会探测常见本地代理端口。

这种方式**不会修改 Codex 的 `app.asar`**，也不需要往 Codex 安装目录写 DLL。

## 常用操作

| 目标 | 操作 |
| --- | --- |
| 启动 Codex++ | 从 `Codex++` 快捷方式或应用入口启动。 |
| 管理供应商和模型 | 打开 `Codex++ 管理工具`，编辑对应供应商后保存。 |
| 检查连接 | 在供应商详情中运行模型测试或 Provider Doctor。 |
| 管理会话 | 使用管理工具中的会话删除、导出、项目移动和 Timeline 功能。 |
| 检查更新 | 在管理工具的“关于”页面检查并启动更新。 |
| 卸载 | 使用系统的卸载入口移除 Codex++；移除前如需保留配置，请先备份 `~/.codex/`。 |

## 相关内容

- [Codex 使用指南](/clients/codex)
- [CC-Switch](/clients/cc-switch)
- [创建 API Key](/guide/api-key)

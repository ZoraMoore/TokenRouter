# CC-Switch

需要配置或更新 GPT 模型时，可以安装[云桥词元配置 Skill](/clients/yunqiao-skill)，让 AI 助手查询当前 Key 的可用模型并指导你完成 CC Switch 配置。

`CC-Switch` 是一款专为管理 AI 接口配置而设计的图形化工具，可统一管理多个 Agent 客户端的供应商配置与 API Key，支持 `Claude Code`、`Codex`、`OpenCode` 等主流 AI 编程助手，降低配置错误风险。

适用场景包括：

- 统一管理多个 Agent 客户端（如 `Claude Code`、`Codex`、`OpenCode`）的供应商配置与 API Key。
- 快速切换不同 AI 模型接口，无需手动修改环境变量或本地配置文件。
- 减少重复配置，提升多客户端场景下的效率与可维护性。

本文将重点介绍安装流程，更多配置详情可参阅相关客户端页面。

## 安装说明

请根据您所使用的操作系统，参考以下步骤完成安装。

::: details Windows

1. 访问 [CC-Switch Releases](https://github.com/farion1231/cc-switch/releases/latest)，进入 Releases 页面。在页面中找到适用于 Windows 的安装包（推荐 `.msi` 格式），如图所示：

   <div style="text-align: center;">
     <img src="https://docs.tokenflux.dev/images/cc-switch/windows-installer-selection.png" alt="如何在 Releases 页面选择适合 Windows 的安装包" />
   </div>

2. 下载完成后，双击安装包。
3. 按照安装向导的提示完成安装。
4. 安装完成后，您可以在开始菜单中找到并启动 `CC-Switch`。

:::

::: details macOS

建议使用 Homebrew 进行安装：

```bash
brew tap farion1231/ccswitch
brew install --cask cc-switch
```

安装完成后，可以从“应用程序”或启动台找到并运行 `CC-Switch`。

:::

::: details Linux

对于 Linux 用户，推荐通过 Releases 页面下载 AppImage 文件：

1. 前往 [发布页面](https://github.com/farion1231/cc-switch/releases/latest)。
2. 找到适合您操作系统架构的 AppImage 文件（例如 `CC-Switch-v3.13.0-Linux-x86_64.AppImage`）。请根据实际的系统架构（如 x86_64 或 arm64）选择正确的版本。如果对如何选择文件有疑问，可以参考下图：

   <div style="text-align: center;">
     <img src="https://docs.tokenflux.dev/images/cc-switch/linux-installer-selection.png" alt="如何在 Releases 页面选择适合 Linux 的 AppImage 文件" />
   </div>

3. 下载完成后，赋予文件可执行权限。

   ```bash
   chmod +x CC-Switch-v3.13.0-Linux-x86_64.AppImage
   ```

4. 双击运行文件，或在终端中使用以下命令启动：

   ```bash
   ./CC-Switch-v3.13.0-Linux-x86_64.AppImage
   ```

   如果您使用的是图形界面（例如 KDE 或 GNOME），还可以通过右键点击文件，选择“属性”或“权限”选项卡，并启用“作为可执行程序”的权限。

AppImage 文件集成了所有必要的依赖，适用于大多数 Linux 发行版。

:::

## 安装后怎么开始？

1. 按照 [创建 API Key 教程](/guide/api-key)，获取一个新的 API Key。

   在 API 密钥列表中找到需要导入的 Key，点击“更多”，再选择“导入到 CCS”。

   <div style="text-align: center;">
     <img src="/images/cc-switch/import-from-api-key.png" width="900" alt="云桥词元 API 密钥列表中通过更多菜单导入到 CCS 的入口" />
   </div>

2. 打开 `CC-Switch`，点击右上角的“添加”按钮，选择“添加统一供应商”。
3. 在弹出的配置窗口中，准确填写以下字段：

   ```text
   供应商名: 云桥词元 API
   API 地址: https://api.scienceedu.me
   API Key: 你的云桥词元 API Key
   ```

   示例界面如下图所示：

   <div style="text-align: center;">
     <img src="/images/cc-switch/manual-provider-fields.png" alt="云桥词元 API 在 CC-Switch 中填写统一供应商字段示意图" />
   </div>

4. 保存配置，`CC-Switch` 会自动为所有相关的 Agent 启用该供应商。

## DOYO Claude 渠道环境变量

::: warning
DOYO 的 Claude 渠道暂时无法在 Claude 应用（Claude app）中使用，只能通过 `Claude Code` CLI 使用。
:::

如果你通过 `CC-Switch` 管理 Claude Code，可以把 `ENABLE_PROMPT_CACHING_1H` 写进 Claude 供应商的 JSON 配置。`CC-Switch` 在启用供应商时会把该配置写入 Claude Code 的 `settings.json`。

该变量用于开启 1 小时 prompt cache，适合重复携带较长上下文的 Claude Code 会话。它只对 DOYO 的 Claude 渠道有效，其他 Claude 渠道无需设置。

### 应用专属供应商

1. 在 `CC-Switch` 顶部切换到 `Claude Code`。
2. 找到云桥词元 API 对应的 Claude 供应商卡片，点击“编辑”。
3. 在“配置 JSON”里的 `env` 中追加：

```json
{
  "env": {
    "ENABLE_PROMPT_CACHING_1H": "1"
  }
}
```

### 统一供应商

统一供应商的基础表单只填写“名称”、“API Key”、“API 地址”和模型配置，没有单独的 DOYO 环境变量字段。使用统一供应商时：

1. 先按上方流程创建并同步统一供应商。
2. 回到 `Claude Code` 应用下，编辑同步生成的 Claude 供应商。
3. 在该供应商的 JSON `env` 中追加 `"ENABLE_PROMPT_CACHING_1H": "1"`。
4. 保存后启用该 Claude 供应商。

## 下一步该怎么做？

恭喜你成功安装并配置了 `CC-Switch`！接下来，只需重启你的客户端，即可开始使用。

更多详情，请参阅：

- [Claude Code 使用指南](/clients/claude-code)
- [Codex 使用指南](/clients/codex)
- [OpenCode 使用指南](/clients/opencode)

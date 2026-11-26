# 用 Skill 更新 GPT 模型

安装 `yunqiao-api-cc-switch` 后，可以让 AI 助手按你的云桥词元 Key 查询模型，再通过 CC Switch 配置 Codex。技能涵盖首次接入、更新默认模型、添加模型菜单选项和配置排查，附有 Claude Code、OpenCode 说明。

[下载技能 ZIP](/downloads/yunqiao-api-cc-switch.zip)

## 安装到 CC Switch

1. 下载上面的 ZIP 文件。
2. 打开 CC Switch 的“Skills”页面，在“添加 Skill”中选择“从 ZIP 安装”。不同版本的菜单名称可能略有差异。
3. 选择下载的 ZIP，安装后为 Codex 开启该技能。
4. 重新启动 Codex 并新建会话，让客户端发现技能。

如果当前版本没有 ZIP 安装入口，可以更新 CC Switch，或解压后将整个 `yunqiao-api-cc-switch` 文件夹放到 Codex 的用户技能目录。CC Switch 常用目录为：

| 系统 | 技能目录 |
| --- | --- |
| Windows | `%USERPROFILE%\.codex\skills\` |
| macOS / Linux | `~/.codex/skills/` |

使用自定义配置目录时，以 CC Switch 设置中的 Codex 路径和客户端实际扫描目录为准。目录内应能直接看到 `yunqiao-api-cc-switch/SKILL.md`，并带有 `references` 文件夹。

安装技能后，API Key 仍需在你本机的 CC Switch 中填写。首次获取 Key 请参考[创建 API Key](/guide/api-key)。

## 直接发给 Codex

首次接入可以发送：

```text
使用 $yunqiao-api-cc-switch 帮我通过 CC Switch 接入云桥词元 API。
先查询我的 Key 可以使用哪些 GPT 模型，再让我选择默认模型。
API Key 我会在本机 CC Switch 中填写。
```

更新新上架的模型可以发送：

```text
使用 $yunqiao-api-cc-switch 检查云桥词元当前可用的 GPT 模型。
结合网站公告列出我的 Key 可以使用的新型号，让我选择后，
更新 CC Switch 中 Codex 的默认模型，并检查新会话是否生效。
```

已知道完整 ID 时，把下面的 `目标模型ID` 换成网站或接口返回的值：

```text
使用 $yunqiao-api-cc-switch，把 Codex 的云桥词元默认模型改为“目标模型ID”。
先核对这个 Key 是否能调用，再通过 CC Switch 保存配置并验证。
```

只想在菜单中增加一个模型时可以发送：

```text
使用 $yunqiao-api-cc-switch，把“目标模型ID”添加到云桥词元的 Codex 模型列表。
默认模型继续使用当前选择。
```

## 技能如何完成配置

技能查询当前 Key 的模型目录，使用返回的完整 ID。它会区分默认模型、菜单列表和当前会话的选择，并检查 CC Switch 是否把设置保存到正确的供应商。

AI 助手拥有桌面操作工具时，可以操作 CC Switch。没有这种工具时，它会给出具体点击步骤和配置片段，由你在 CC Switch 保存。它会说明哪些检查已经完成、哪些需要你重启客户端或实测。

不要把完整 Key 粘贴到群聊、工单或公开教程中。连通性测试会发起一次简短模型请求，产生少量用量。

## 常见情况

| 情况 | 处理方式 |
| --- | --- |
| 网站上架了，当前 Key 没有该模型 | 检查 Key 所属分组及可用协议。 |
| 改完默认模型，菜单里还没有 | 检查 CC Switch 的模型列表，重新启动 Codex，并核对客户端版本。 |
| 改完仍然请求旧模型 | 使用新会话，检查启用的供应商、profile、项目配置和启动参数。 |
| 切换供应商后型号恢复了 | 在 CC Switch 中保存型号，检查统一供应商的同步配置。 |
| CLI 正常，App 不正常 | 检查两者配置目录、环境变量、登录方式和 App 重启情况。 |

技能通过查询模型目录适配后续上架的型号。模型的上下文长度、思考等级和协议能力仍以当前分组说明为准。

## 参考文档

- [CC Switch 使用指南](/clients/cc-switch)
- [Codex 使用指南](/clients/codex)
- [模型、分组与计费](/guide/models-and-billing)
- [CC Switch Skills 管理](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/zh/3-extensions/3.3-skills.md)

# WorkBuddy 使用指南

`WorkBuddy` 是腾讯推出的桌面 AI Agent 工作台，支持本地文件处理、Skills、MCP、自动化任务和自定义模型。它可通过 OpenAI 兼容接口接入云桥词元 API。

## 下载与安装

1. 打开 [WorkBuddy 官方下载页](https://www.codebuddy.cn/work/)，也可以先访问 [国内官网](https://www.workbuddy.cn/) 或 [海外官网](https://www.workbuddy.ai/)。
2. 选择与设备匹配的安装包：
   - Apple Silicon Mac：选择 `arm64`；
   - Intel Mac：选择 `x64`；
   - Windows：选择 Windows 安装程序。
3. macOS 打开下载的 DMG，将 `WorkBuddy.app` 拖入“应用程序”；Windows 按安装程序提示完成安装。
4. 第一次启动时，WorkBuddy 会准备本地运行环境。请保持网络连接，等待应用自动进入欢迎页，不要在此阶段强制退出。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/01-preparing-environment.png" width="800" alt="WorkBuddy 第一次启动时正在准备运行环境" />
</div>

环境准备完成后会显示欢迎页，点击 **登录** 继续。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/02-welcome-login.png" width="987" alt="WorkBuddy 欢迎页和登录按钮" />
</div>

## 登录

WorkBuddy 会在浏览器中打开腾讯 CodeBuddy 登录页。

1. 阅读用户服务协议和隐私政策。
2. 由你本人勾选同意选项并点击 **同意**。
3. 选择微信、手机号、邮箱或 SSO 完成认证；可用方式可能因地区和账号类型而异。
4. 浏览器提示成功后返回 WorkBuddy。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/03-browser-sign-in.png" width="1274" alt="WorkBuddy 的腾讯 CodeBuddy 登录页" />
</div>

## 接入云桥词元 API

### 1. 准备 API Key

在[创建 API Key](/guide/api-key)中创建一张 Key，并在[模型、分组与计费](/guide/models-and-billing)页面确认该 Key 当前可用的模型 ID。

### 2. 打开自定义模型设置

登录 WorkBuddy 后，依次进入：

1. 头像或账户菜单；
2. **系统设置**；
3. **模型**；
4. 点击 **添加模型**。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/04-system-settings.png" width="925" alt="WorkBuddy 系统设置中的模型入口" />
</div>

进入 **模型** 后可以看到本地配置文件路径和 **添加模型** 按钮。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/05-model-settings.png" width="925" alt="WorkBuddy 模型设置页面和添加模型按钮" />
</div>

WorkBuddy 会在模型页面显示实际的本地配置文件路径。桌面版通常使用 `~/.workbuddy/models.json`；官方模型配置说明也提到已有 `~/.codebuddy/models.json` 的兼容迁移。

### 3. 填写模型配置

在提供商列表底部选择 **自定义 / Custom**。WorkBuddy 的自定义入口使用 OpenAI 兼容 API，云桥词元 API 可直接接入。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/06-provider-selection.png" width="538" alt="WorkBuddy 添加模型时选择自定义 Custom" />
</div>

然后填写：

| 配置项 | 建议值 |
| --- | --- |
| 提供商 | `自定义 / Custom` |
| 接口地址 | `https://api.scienceedu.me/v1/chat/completions` |
| API Key | 你的云桥词元 API Key |
| 模型名称 | 当前 Key 的模型列表中显示的精确模型 ID |
| 工具调用 | 仅当所选模型支持 Tool Calling 时开启 |
| 图片输入 | 仅当所选模型和分组支持图片输入时开启 |
| 思考模式 | 仅当所选模型支持 reasoning 时开启 |
| 自定义协议 | 保持关闭，使用 OpenAI Chat Completions 兼容协议 |

::: tip 使用完整接口地址

为避免不同 WorkBuddy 版本对路径补全的差异，直接填写以上以 `/v1/chat/completions` 结尾的完整接口地址。

:::

::: warning 不要开启“自定义协议”

该选项用于非标准网关或代理路径。云桥词元 API 使用标准 OpenAI Chat Completions 兼容接口，无需开启。

:::

如果使用[复合 Key](/guide/api-key)，模型名称还需要带上控制台提供的分组前缀，例如 `GPT/模型_ID`。请直接复制“使用 Key”弹窗中的示例，前缀和模型 ID 必须与映射完全一致。

### 4. 保存并验证

1. 点击 **保存**。模型会写入 WorkBuddy 模型页面显示的本地配置文件。

<div style="text-align: center;">
  <img src="/images/workbuddy/save-model.png" width="538" alt="WorkBuddy 中填写云桥词元 API 配置并保存模型" />
</div>

2. 保存后，确认刚填写的模型出现在 **已保存模型** 中。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/09-saved-model.png" width="925" alt="WorkBuddy 已保存模型列表" />
</div>

3. 返回新任务，打开模型选择器，在 **自定义模型** 中选择刚添加的模型。

<div style="text-align: center;">
  <img src="https://docs.tokenflux.dev/images/workbuddy/10-select-custom-model.png" width="268" alt="WorkBuddy 模型选择器中的自定义模型" />
</div>

4. 发送一条最小测试消息，例如“你好”。收到正常回复后，再查看模型名称与用量是否符合预期。
5. 基础对话验证成功后，再测试需要工具调用的任务，例如让 WorkBuddy 在临时目录中创建文本文件并读取回来。

## 常见问题

### 保存后找不到模型

重新打开模型选择器或新建任务。如果使用企业账号，管理员可能禁止成员添加个人自定义模型，需要联系企业管理员确认策略。

### 返回 401 或认证失败

检查 API Key 是否复制完整、是否含有空格或换行，以及 Key 是否已失效。不要把 Key 填到模型名称或接口地址字段。

### 返回 `model not found`

模型名称必须与当前 Key 可见的模型 ID 完全一致。请从 `/v1/models` 返回结果或[模型、分组与计费](/guide/models-and-billing)页面获取；复合 Key 还需要正确的分组前缀。

### Agent 不会调用工具

编辑该模型并确认 **工具调用** 已开启，同时确认上游模型本身支持 Tool Calling。只开启客户端选项，不能让不支持工具调用的模型获得该能力。

## 相关入口

- [API 端点](/api/endpoints)
- [创建 API Key](/guide/api-key)
- [模型、分组与计费](/guide/models-and-billing)
- [WorkBuddy 官方下载页](https://www.codebuddy.cn/work/)
- [WorkBuddy 官方模型配置文档](https://www.workbuddy.ai/docs/zh/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Model)

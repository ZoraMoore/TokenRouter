# Codex 字段与排查

## 默认模型与模型菜单

Codex 的 `model` 决定默认请求模型。供应商的模型列表决定它向客户端提供哪些候选，两者需要分别核对。

CC Switch 编辑页提供“模型列表”时，将要使用的完整 ID 添加到列表。核实当前版本如何指定默认项：已核对的上游实现以列表第一行为默认模型。添加选项后复查默认值，防止插入或拖动一行时顺带切换默认型号。

Codex App 的下拉菜单需要单独核对候选列表。更新 `model` 后菜单仍缺少该型号时，按顺序检查：

1. 供应商的模型列表是否已添加该 ID，修改是否保存。
2. 该供应商是否正在使用；聚合模式下，模型属于哪家供应商。
3. Codex 是否重新启动并创建了新会话。
4. CC Switch 与 Codex 是否读取同一个用户目录、同一个 `CODEX_HOME` 和同一个 Windows/WSL 环境。
5. 启动参数、profile 或项目配置是否指定了另一个模型。
6. 当前 CC Switch 与 Codex App 的版本是否存在菜单显示限制。

上游另有 [Codex 桌面自定义模型显示说明](https://github.com/farion1231/cc-switch/blob/main/docs/guides/codex-desktop-custom-model-visibility-zh.md)。只有核对当前版本确实遇到该问题时，才按该文档检查“保留官方登录”等设置。它是版本相关的处理办法，云桥词元 API Key 的创建不以购买 ChatGPT 订阅为前提。处理过程中保护用户已有登录态。

部分版本提供 `codex debug models`。先检查本机 `codex debug --help` 是否支持，再用它诊断模型目录。其输出和 `/model` 菜单仍需结合实际请求验证。

## Base URL 与 TOML

| 项目 | 值 |
| --- | --- |
| Base URL | `https://api.scienceedu.me/v1` |
| 协议 | `responses` |
| 模型 | 当前 Key 可用且支持 Responses 的完整 ID |
| provider 标识 | 已有的 `custom`、`scienceedu` 等标识 |

Base URL 包含一段 `/v1`，客户端会在后面追加 `/responses`。`/v1/models` 是查询目录的地址，不能填入 Base URL。

下面展示首次配置的字段结构。`MODEL_ID_FROM_V1_MODELS` 是占位符，保存前换成查询得到的 ID。编辑已有供应商时，修改匹配的字段，provider 标识与对应表名保持一致。

```toml
model_provider = "scienceedu"
model = "MODEL_ID_FROM_V1_MODELS"

[model_providers.scienceedu]
name = "OpenAI"
base_url = "https://api.scienceedu.me/v1"
wire_api = "responses"
```

该片段需要配合下面一种认证方式才能使用。`model`、`model_provider` 是 TOML 顶层字段，应在 `[model_providers.scienceedu]` 之前。粘贴到文件末尾的某个表中会改变它们所属的位置。

云桥词元 Codex 教程使用 `name = "OpenAI"` 适配部分 Codex 版本的远程压缩识别。这是 provider 表内的名称，CC Switch 卡片的显示名称仍可使用“云桥词元 API”。只升级模型时先沿用能工作的 provider 配置。

## 认证方式

CC Switch 与网站手动教程可以采用不同的凭据存储方式。先检查现有配置，使用其中已经生效的一种。

### CC Switch 管理 API Key

在供应商的 API Key 输入框中填写云桥词元 Key。常见版本由 CC Switch 写入 Codex 的 `auth.json`，并在 provider 表中设置：

```toml
[model_providers.scienceedu]
name = "OpenAI"
base_url = "https://api.scienceedu.me/v1"
wire_api = "responses"
requires_openai_auth = true
```

对应的 `auth.json` 可能包含：

```json
{
  "OPENAI_API_KEY": "YOUR_YUNQIAO_API_KEY"
}
```

这里的 `OPENAI_API_KEY` 是客户端字段名，填写的是云桥词元 Key。该方式可用于中转服务的 API Key，`requires_openai_auth` 的名字不表示必须购买官方订阅。

启用保留官方登录、路由接管等功能时，CC Switch 可能采用另一种凭据写入方式。让当前版本的应用完成保存，按其生成结果核对。手工覆盖 `auth.json` 会损坏用户已有登录信息。

### 环境变量认证

用户已经使用环境变量认证时，在 provider 表中使用：

```toml
[model_providers.scienceedu]
name = "OpenAI"
base_url = "https://api.scienceedu.me/v1"
wire_api = "responses"
env_key = "SCIENCEEDU_API_KEY"
requires_openai_auth = false
```

`env_key` 的值是环境变量名称。实际 Key 放在启动 Codex 的进程环境中。终端中的临时变量通常只传给该终端启动的子进程；从桌面图标打开的 Codex App 未必能读取它。使用图形界面的用户可选择让 CC Switch 管理 Key。

单纯修改型号时继续使用现有认证方式。认证排查需要核对 `env_key`、`requires_openai_auth` 和进程收到的变量，打印时隐去值。

## 配置覆盖与特殊选项

- Codex 默认用户配置常见于 Windows 的 `%USERPROFILE%\.codex\config.toml` 或 macOS/Linux 的 `~/.codex/config.toml`。设置了 `CODEX_HOME` 或 CC Switch 自定义目录时，以实际路径为准。
- 项目中的 `.codex/config.toml`、选中的 `[profiles.<name>]`、`codex -m`、`codex -c` 和客户端会话选择都可能影响模型。更新 CC Switch 后仍是旧模型时，逐项核对，不批量替换整个文件中的型号。
- `review_model` 是单独的配置。用户需要代码审查一起升级时再核对它。CC Switch 的自动审批模型和 `codex-auto-review` 也需要按各自用途确认，不能通过全文替换强行跟随主模型。
- `model_reasoning_effort` 采用该模型支持的等级。用户未要求改变时，核对已有值的兼容性；不支持时选用文档支持的默认值或省略该项。
- 上下文窗口、自动压缩阈值和模型能力由当前分组信息确定。普通 `/v1/models` 的 ID 列表不能证明模型支持 1M 上下文。
- Fast、优先级服务、WebSocket 和本地格式转换按用户需求配置。已有正常的 Responses 连接可以直接用于 GPT 模型更新。
- 路由接管时，Codex 的 `base_url` 可能指向本地 CC Switch。查看 CC Switch 供应商内的上游地址和凭据；直接改 Codex 文件会与接管状态冲突。

## 排查表

| 现象 | 检查与处理 |
| --- | --- |
| 401 | Key 是否缺失、失效或被撤销；认证方式是否匹配；App 是否读取到环境变量。停止重复发送无效凭据。 |
| 403 | 当前 Key 的分组、模型权限、IP 限制或客户端准入规则。查看站点返回的具体错误，换客户端模型名不能解除权限限制。 |
| 404 / model not found | 先区分路径错误与模型错误，核对 `/v1`、完整模型 ID、分组支持的协议及目录更新情况。 |
| 400 / unsupported parameter | 查看被拒绝的参数，检查思考等级、请求格式或客户端版本。修正报错指出的配置后再验证。 |
| 429 / quota exceeded | 区分速率限制、并发、余额和订阅额度；查看 Key 用量及错误信息，按服务提示等待或处理额度。 |
| 5xx / 暂无可用账号 | 记录时间、模型和 Request ID，查看站点状态或联系支持。限制重试次数。 |
| 获取模型为空 | 检查是否读到正确 JSON、同一 Key 的分组和目录。网站“使用 Key”也无候选时，请站点支持检查上架情况。 |
| 模型可调用，菜单里没有 | 检查供应商模型列表、客户端重启、实际配置目录和当前版本菜单限制。 |
| 改完仍用旧模型 | 核对当前供应商、连接模式、profile、项目配置、启动参数以及会话里的模型选择。 |
| 切换一次又恢复旧模型 | 在 CC Switch 供应商记录中保存更改，并检查统一供应商的同步设置。 |
| CLI 能用，App 不能用 | 检查进程环境、配置目录、认证方式、App 版本和是否完成退出重启。 |
| 模型返回自述与名称不同 | 使用请求模型字段、配置和站点记录核对；自述文本无法证明上游型号。 |

排查输出保留 HTTP 状态、错误类型、脱敏信息和 Request ID。完整密钥、Authorization 头和含密钥的深链留在本机。

## 资料来源

2026-11-18 核对了云桥词元文档及 CC Switch 上游源码，后者提交为 `bdbc9604bc655320bfe4560ed6cf36b3cdf6b910`。上游主分支可能领先用户安装的版本，按钮和认证写入方式以当前版本为准。

- [云桥词元 Codex 教程](https://docs.scienceedu.me/clients/codex)
- [云桥词元模型与分组](https://docs.scienceedu.me/guide/models-and-billing)
- [CC Switch 编辑供应商](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/zh/2-providers/2.3-edit.md)
- [CC Switch 切换供应商](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/zh/2-providers/2.2-switch.md)
- [CC Switch Codex 配置生成](https://github.com/farion1231/cc-switch/blob/bdbc9604bc655320bfe4560ed6cf36b3cdf6b910/src/config/codexProviderPresets.ts)
- [Codex 配置参考](https://developers.openai.com/codex/config-reference/)

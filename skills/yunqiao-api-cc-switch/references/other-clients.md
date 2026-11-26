# 其他客户端接入

先按 `SKILL.md` 查询当前 Key 的模型和可用协议，再选择客户端。云桥词元的“使用 Key”页面按分组和协议生成配置，可以用来核对字段。

## Claude Code

在 CC Switch 的 Claude Code 页编辑云桥词元供应商。通过云桥词元的 Anthropic Messages 入口调用时，Base URL 为 `https://api.scienceedu.me`。

目标 GPT 模型需要在该分组支持 Messages 请求。只有 Responses 或 Chat Completions 能力时，先核对 CC Switch 当前版本是否支持对应本地转换；不能单靠替换名称改变协议。

在已有 `env` 中合并需要修改的字段。示例中的模型和 Key 都是占位符：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.scienceedu.me",
    "ANTHROPIC_AUTH_TOKEN": "YOUR_YUNQIAO_API_KEY",
    "ANTHROPIC_MODEL": "MODEL_ID_FROM_V1_MODELS"
  }
}
```

用户要求把 Sonnet、Opus、Haiku 档位也映射到 GPT 时，再逐项核对 `ANTHROPIC_DEFAULT_SONNET_MODEL`、`ANTHROPIC_DEFAULT_OPUS_MODEL`、`ANTHROPIC_DEFAULT_HAIKU_MODEL`。已存在的子任务模型设置单独确认。不同档位可以使用不同 ID。

保存并启用后，在新会话中检查模型并发一次短请求。遇到路由接管时，在 CC Switch 供应商中修改上游设置。

## OpenCode

在 CC Switch 的 OpenCode 页添加或编辑供应商，Base URL 为 `https://api.scienceedu.me/v1`，添加当前 Key 可用的模型。OpenCode 允许多个供应商共存，添加供应商后还需要选择模型。

配置结构可参考网站导出：

```json
{
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
  "model": "scienceedu/MODEL_ID_FROM_V1_MODELS"
}
```

`scienceedu` 是此例中的本地 provider ID，`model` 使用“provider ID/接口返回的完整模型 ID”。接口 ID 本身含斜杠时也完整保留。已有 provider ID 不需要跟着示例改名。

认证由 CC Switch 管理时使用它生成的配置。使用示例中的环境变量方式时，先在启动 OpenCode 的进程环境中配置 Key。用户需要修改轻量任务模型时，再单独设置 `small_model`。

保存后重新加载 OpenCode，用 `opencode models` 核对模型，再在客户端选择它并做一次短请求。上下文、工具调用和模态参数采用网站已提供的属性。

## 资料来源

- [云桥词元 Claude Code 教程](https://docs.scienceedu.me/clients/claude-code)
- [云桥词元 OpenCode 教程](https://docs.scienceedu.me/clients/opencode)
- [云桥词元 API 端点](https://docs.scienceedu.me/api/endpoints)
- [CC Switch 添加供应商](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/zh/2-providers/2.1-add.md)

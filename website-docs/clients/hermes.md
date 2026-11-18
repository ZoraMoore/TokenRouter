# Hermes 使用指南

`Hermes` 是一款 AI Agent 工具，支持通过自定义 OpenAI-compatible 接口接入云桥词元 API。

## 安装

可直接使用官方安装脚本：

```bash
curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash
```

安装完成后，默认配置目录通常位于 `~/.hermes`。

## 接入云桥词元 API

1. 按 [创建 API Key 教程](/guide/api-key) 生成一个 API Key，并确认其分组允许 OpenAI 兼容协议。

2. 编辑 `~/.hermes/config.yaml`，写入或确认以下配置：

   ```yaml
   model:
     default: "MODEL_ID_FROM_V1_MODELS"
     provider: "custom"
     base_url: "https://api.scienceedu.me/v1"
   ```

3. 编辑 `~/.hermes/.env`，写入：

   ```bash
   OPENAI_API_KEY=sk-YOUR_API_KEY
   OPENAI_BASE_URL=https://api.scienceedu.me/v1
   ```

   将 `MODEL_ID_FROM_V1_MODELS` 替换为当前 Key 的 `/v1/models` 返回的模型 ID，并将 `sk-YOUR_API_KEY` 替换为你的真实 API Key。

::: warning 密钥安全
`.env` 包含真实 API Key。不要提交到 Git、同步到公开网盘或发送到截图、工单和聊天群。
:::

## 验证配置

保存配置后，可运行以下命令验证：

```bash
hermes config check
hermes chat -Q -q '只回复 OK' --max-turns 3
```

如果命令能够正常返回，即表示 `Hermes` 已成功通过云桥词元 API 调用模型。

如果提示模型不可用，请重新查询：

```bash
curl https://api.scienceedu.me/v1/models \
  -H "Authorization: Bearer sk-YOUR_API_KEY"
```

并将配置中的 `model.default` 改为返回结果中的模型 ID。

## 更多相关内容

- [创建 API Key](/guide/api-key)
- [模型、分组与计费](/guide/models-and-billing)
- [OpenAI 兼容 API](/api/openai)

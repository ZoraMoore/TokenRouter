# 复合 Key

复合 Key 是一个同时绑定多个分组的 API Key。每个分组配一个前缀；调用时在模型 ID 前加上前缀，即可选择请求使用的分组。

这样无需为不同分组分别创建 Key，也不必在客户端中反复切换配置。

**前往创建：[API 密钥页面](https://api.scienceedu.me/keys)**

## 创建复合 Key

1. 在 [API 密钥页面](https://api.scienceedu.me/keys) 点击“创建密钥”。
2. 填写名称，然后打开“复合 Key”开关。
3. 在映射编辑器中，为每一行选择一个**分组**并填写一个**前缀**。
4. 需要更多分组时点击“添加分组映射”；可使用上移、下移按钮调整顺序。
5. 保存。

<div style="text-align: center;">
  <img src="/images/composite-key/editor.png" width="620" alt="云桥词元创建复合 Key 时配置分组与前缀映射" />
</div>

编辑已有 Key 时，同样可以打开或关闭复合 Key 开关。

例如按下列方式绑定：

| 前缀 | 分组 |
| --- | --- |
| `GPT` | OpenAI 分组 |
| `Claude` | Anthropic 分组 |

### 前缀规则

- 长度为 1–32 位，只能使用**字母、数字、下划线和连字符**；
- **不区分大小写**，`GPT` 和 `gpt` 视为同一个前缀，不能重复；
- 同一个分组在一个 Key 中只能添加一次；
- 每个复合 Key 最多支持 **20** 个分组映射。

填写不符合规则的前缀时，控制台会显示对应提示，例如“前缀仅允许 1 至 32 位字母、数字、下划线或连字符”或“前缀不能重复（不区分大小写）”。

## 调用方式

将模型 ID 写为 `前缀/模型 ID`：

```json
{
  "model": "GPT/gpt-5",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

按上面的绑定，两个分组分别这样调用：

- `GPT/gpt-5`：使用 OpenAI 分组；
- `Claude/claude-sonnet-4`：使用 Anthropic 分组。

::: warning 只按第一个斜杠拆分

如果模型 ID 本身带斜杠，只有**第一个**斜杠会被当作前缀分隔符。例如 `GPT/vendor/model` 的前缀是 `GPT`，实际模型 ID 是 `vendor/model`。

:::

Gemini 原生入口把前缀放在模型路径中：

```text
POST /v1beta/models/Gemini/gemini-2.5-pro:generateContent
```

## 模型列表与调用示例

`/v1/models` 等模型列表接口会按映射顺序聚合各分组可用的模型，并自动给模型 ID 加上前缀。客户端拉取模型列表后，可直接选择带前缀的模型，无需手动拼接。

在密钥列表中点击复合 Key 的“使用”按钮，弹窗会按每个前缀列出对应的调用示例，可直接复制后替换为当前可用模型。

## 不支持的接口

复合 Key 不支持实时或 WebSocket 类接口，例如 `/v1/live`、Codex Realtime、Responses WebSocket 和 Live sideband。这些连接可能在同一个会话内切换模型，服务端无法仅凭前缀确定一个固定分组，请求会返回 `COMPOSITE_KEY_ENDPOINT_UNSUPPORTED`。

**这些场景请使用普通 API Key。**

常规 API 请求仍可使用复合 Key，包括对话、图片生成与编辑、批量图片提交，以及不带模型的用量、账单和任务查询类接口。

## 常见错误

前缀有问题时会返回 HTTP 400；错误结构会保持所使用入口的协议格式。

| 错误码 | 含义 |
| --- | --- |
| `COMPOSITE_KEY_MODEL_PREFIX_REQUIRED` | 模型 ID 没有携带前缀 |
| `COMPOSITE_KEY_PREFIX_NOT_FOUND` | 前缀不在该 Key 的映射中 |
| `COMPOSITE_KEY_PREFIX_INVALID` | 前缀格式不合法 |
| `COMPOSITE_KEY_ENDPOINT_UNSUPPORTED` | 该接口不支持复合 Key |

## 注意事项

- 密钥列表的“分组”列会把复合 Key 显示为若干个“前缀 / 分组名”标签；点击可直接打开映射编辑弹窗；
- 将复合 Key 改回普通 Key 时，需要重新选择一个分组后才能保存；
- 计费、倍率、限流、额度和过期时间等设置仍按实际命中的分组与 Key 配置计算。

## 相关入口

- [创建 API Key](/guide/api-key)：普通 Key 的创建流程；
- [模型、分组与计费](/guide/models-and-billing)：查看当前可用模型与分组计费；
- [API 端点](/api/endpoints)：接口地址与协议格式；
- [API 密钥页面](https://api.scienceedu.me/keys)：创建或编辑复合 Key。

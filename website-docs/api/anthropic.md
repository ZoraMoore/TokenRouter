# Anthropic Messages API

Anthropic 兼容入口适用于官方 Anthropic SDK、Claude Code，以及支持 Messages 协议的其它客户端。

## cURL

```bash
curl https://api.scienceedu.me/v1/messages \
  -H "x-api-key: sk-YOUR_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{
    "model": "MODEL_ID_FROM_V1_MODELS",
    "max_tokens": 512,
    "messages": [
      {"role": "user", "content": "解释一下幂等性"}
    ]
  }'
```

## Python SDK

```bash
pip install -U anthropic
```

```python
import anthropic

client = anthropic.Anthropic(
    api_key="sk-YOUR_API_KEY",
    base_url="https://api.scienceedu.me",
)

message = client.messages.create(
    model="MODEL_ID_FROM_V1_MODELS",
    max_tokens=512,
    messages=[{"role": "user", "content": "给我一个 Python 重试示例"}],
)

print(message.content[0].text)
```

## Node.js SDK

```bash
npm install @anthropic-ai/sdk
```

```javascript
import Anthropic from '@anthropic-ai/sdk'

const client = new Anthropic({
  apiKey: 'sk-YOUR_API_KEY',
  baseURL: 'https://api.scienceedu.me'
})

const message = await client.messages.create({
  model: 'MODEL_ID_FROM_V1_MODELS',
  max_tokens: 512,
  messages: [{ role: 'user', content: '你好' }]
})

console.log(message.content)
```

## Token 统计

```bash
curl https://api.scienceedu.me/v1/messages/count_tokens \
  -H "x-api-key: sk-YOUR_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{
    "model": "MODEL_ID_FROM_V1_MODELS",
    "messages": [{"role": "user", "content": "需要统计的内容"}]
  }'
```

统计结果用于估算上下文规模，最终结算以实际请求的服务端用量记录为准。

## 协议兼容说明

`/v1/messages` 会根据 Key 的最终分组路由，但分组必须显式允许 `anthropic_messages`。如果 OpenAI 或其它平台分组开放了 Messages 兼容，模型名称映射和具体能力仍由该分组决定。

常见的 `403 permission_error` 表示协议未开放或本地策略拒绝；`404` 可能表示该平台不支持所请求的具体能力。

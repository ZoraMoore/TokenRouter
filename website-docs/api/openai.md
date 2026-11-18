# OpenAI 兼容 API

OpenAI 兼容入口支持 Responses API、Chat Completions，以及分组能力允许时的 Embeddings、Images 等接口。

## Responses API

### cURL

```bash
curl https://api.scienceedu.me/v1/responses \
  -H "Authorization: Bearer sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MODEL_ID_FROM_V1_MODELS",
    "input": "解释一下什么是 API 中转服务",
    "stream": false
  }'
```

### Python SDK

```bash
pip install -U openai
```

```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-YOUR_API_KEY",
    base_url="https://api.scienceedu.me/v1",
)

response = client.responses.create(
    model="MODEL_ID_FROM_V1_MODELS",
    input="用三点说明流式响应的优势",
)

print(response.output_text)
```

### Node.js SDK

```bash
npm install openai
```

```javascript
import OpenAI from 'openai'

const client = new OpenAI({
  apiKey: 'sk-YOUR_API_KEY',
  baseURL: 'https://api.scienceedu.me/v1'
})

const response = await client.responses.create({
  model: 'MODEL_ID_FROM_V1_MODELS',
  input: '写一句简短的欢迎语'
})

console.log(response.output_text)
```

生产环境请从环境变量读取 Key，不要像示例一样写在源码里。

## Chat Completions

某些旧客户端只支持 Chat Completions：

```bash
curl https://api.scienceedu.me/v1/chat/completions \
  -H "Authorization: Bearer sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MODEL_ID_FROM_V1_MODELS",
    "messages": [
      {"role": "system", "content": "你是一个简洁的助手。"},
      {"role": "user", "content": "你好"}
    ],
    "stream": false
  }'
```

新项目优先使用 Responses API；只有客户端或模型分组明确需要时再使用 Chat Completions。

## 流式输出

把 `stream` 设置为 `true`。cURL 使用 `-N` 关闭输出缓冲：

```bash
curl -N https://api.scienceedu.me/v1/responses \
  -H "Authorization: Bearer sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MODEL_ID_FROM_V1_MODELS",
    "input": "逐步解释快速排序",
    "stream": true
  }'
```

客户端必须按 SSE 事件逐段处理。流已经开始后，错误也可能以流事件而不是普通 JSON 返回。

## 图片生成

仅对支持图片能力的分组与模型开放：

```bash
curl https://api.scienceedu.me/v1/images/generations \
  -H "Authorization: Bearer sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "IMAGE_MODEL_ID_FROM_V1_MODELS",
    "prompt": "一座漂浮在云海上的未来图书馆"
  }'
```

图片请求的计费单位、返回格式和可选参数可能不同于文本模型。发起批量任务前先用单张请求核对模型广场和用量记录。

## Embeddings

Embeddings 只在 OpenAI 平台分组及其上游能力支持时提供：

```bash
curl https://api.scienceedu.me/v1/embeddings \
  -H "Authorization: Bearer sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "EMBEDDING_MODEL_ID_FROM_V1_MODELS",
    "input": "需要向量化的文本"
  }'
```

若模型列表中没有 Embedding 模型，不要强行使用其它文本模型 ID。

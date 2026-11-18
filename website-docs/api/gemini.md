# Gemini 原生 API

Gemini 原生兼容层使用 `/v1beta` 路径，适合 Google Gen AI SDK、Gemini CLI 及原生 `generateContent` 调用。

## 列出模型

```bash
curl https://api.scienceedu.me/v1beta/models \
  -H "x-goog-api-key: sk-YOUR_API_KEY"
```

## generateContent

```bash
curl "https://api.scienceedu.me/v1beta/models/MODEL_ID:generateContent" \
  -H "x-goog-api-key: sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{
      "role": "user",
      "parts": [{"text": "用一句话解释向量数据库"}]
    }]
  }'
```

## 流式生成

```bash
curl -N "https://api.scienceedu.me/v1beta/models/MODEL_ID:streamGenerateContent?alt=sse" \
  -H "x-goog-api-key: sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{"parts": [{"text": "写一个三段式短故事"}]}]
  }'
```

## Token 统计

```bash
curl "https://api.scienceedu.me/v1beta/models/MODEL_ID:countTokens" \
  -H "x-goog-api-key: sk-YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{"parts": [{"text": "需要统计的文本"}]}]
  }'
```

## Gemini CLI 环境变量

macOS / Linux：

```bash
export GOOGLE_GEMINI_BASE_URL="https://api.scienceedu.me"
export GEMINI_API_KEY="sk-YOUR_API_KEY"
export GEMINI_MODEL="MODEL_ID_FROM_V1BETA_MODELS"
```

PowerShell：

```powershell
$env:GOOGLE_GEMINI_BASE_URL="https://api.scienceedu.me"
$env:GEMINI_API_KEY="sk-YOUR_API_KEY"
$env:GEMINI_MODEL="MODEL_ID_FROM_V1BETA_MODELS"
```

分组必须允许 `gemini_generate_content`。如果 Key 绑定的是其它平台分组，即使模型名称看起来相似，也可能收到 `403 PERMISSION_DENIED`。

---
layout: home

hero:
  name: 云桥 token
  text: 使用教程文档站
  tagline: 从创建 API Key 到配置常用 AI 客户端，快速完成模型服务接入
  image:
    src: /site-logo.svg
    alt: 云桥词元
  actions:
    - theme: brand
      text: 三分钟快速开始
      link: /guide/quickstart
    - theme: alt
      text: 查看 API 端点
      link: /api/endpoints

features:
  - icon: 🔌
    title: 多协议，一套 API
    details: 同时兼容 OpenAI Responses、Chat Completions、Anthropic Messages 与 Gemini 原生协议，无需重写现有应用。
  - icon: 🔑
    title: 一个 Key，按需切换
    details: 在控制台按分组创建 Key，模型能力、价格、可用协议与使用限额一目了然。
  - icon: 🧰
    title: 热门客户端，直接照抄
    details: Codex、Claude Code、OpenCode 与 Cherry Studio 均提供可复制的完整配置。
  - icon: 📈
    title: 用量与费用，随时掌握
    details: 通过控制台或 /v1/usage 查询余额来源、Key 配额、滚动限额与使用情况。
---

## 统一入口

所有客户端都连接同一个服务域名：

```text
https://api.scienceedu.me
```

不同 SDK 对 Base URL 的约定不同：OpenAI 客户端通常填写 `https://api.scienceedu.me/v1`，Anthropic 与 Gemini CLI 通常填写不带 `/v1` 的站点根地址。请以各客户端章节的示例为准。

::: warning 密钥安全
文档中的 `sk-YOUR_API_KEY` 只是占位符。真实 API Key 不要发给他人，也不要写进前端网页、公开仓库、截图或日志。
:::

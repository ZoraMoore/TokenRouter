# RikkaHub 使用指南

`RikkaHub` 是一款 Android AI 聊天客户端，支持接入多种 OpenAI 兼容接口，适合在手机上进行日常 AI 对话。

## 安装

RikkaHub 目前仅支持 Android，**不支持 iOS**。

你可以通过以下任一方式安装：

- 前往 [RikkaHub 官网下载页](https://rikka-ai.com/download) 下载 APK 安装包，手动安装到设备；
- 在 Google Play 搜索 `RikkaHub` 并安装。

## 接入云桥词元 API

RikkaHub 支持以 OpenAI 兼容方式接入云桥词元 API。

1. 在[创建 API Key](/guide/api-key)中生成一个 API Key。
2. 打开 `RikkaHub`，点击左侧**侧边栏**，进入**设置**。
3. 选择**提供商**，点击添加新提供商，类型选择 `OpenAI`。
4. 填入以下配置：

   ```text
   提供商类型: OpenAI
   Base URL: https://api.scienceedu.me/v1
   API Key: 你的云桥词元 API Key
   ```

   > **注意：**Base URL 必须以 `/v1` 结尾。RikkaHub 会在此基础上自动拼接 `/chat/completions` 等路径，请勿在末尾重复添加。

5. 保存后，等待模型列表刷新，点击“添加模型”左侧的图标，选择需要使用的模型。
6. 返回主页，选择刚才添加的模型即可开始对话。

模型名称应以当前 API Key 的可见列表为准。你可以在[模型、分组与计费](/guide/models-and-billing)中查看查询方法；如果使用复合 Key，请同时填写控制台提供的模型分组前缀。

## 下一步该怎么做？

接入完成后，你可以直接在 `RikkaHub` 中与云桥词元 API 支持的模型对话。建议先发送一条简短消息确认连接、模型和用量均正常，再开始日常使用。

## 更多相关内容

- [模型、分组与计费](/guide/models-and-billing)
- [创建 API Key](/guide/api-key)
- [API 端点](/api/endpoints)

# 云桥词元

[访问站点](https://api.scienceedu.me/home) · [模型广场](https://api.scienceedu.me/models) · [接入文档](https://docs.scienceedu.me/)

云桥词元是一个多模型 API 服务。你可以在控制台创建 API Key，通过兼容 OpenAI、Anthropic 和 Gemini 的接口调用所选分组开放的模型，并在控制台查询用量和费用。

## 接入云桥词元

1. 登录站点，根据所选分组的计费方式准备余额或订阅。[模型、分组与计费](https://docs.scienceedu.me/guide/models-and-billing)
2. 在控制台创建 API Key，选择分组并保存完整密钥。密钥完整内容仅在创建时显示。[API 密钥页面](https://api.scienceedu.me/keys)
3. 在客户端填入 API Key，再按客户端要求选择 Base URL。分组需要允许客户端使用的协议。[客户端接入指南](https://docs.scienceedu.me/guide/quickstart)

| 客户端协议 | Base URL | 常用请求路径 |
| --- | --- | --- |
| OpenAI | `https://api.scienceedu.me/v1` | `POST /v1/responses`、`POST /v1/chat/completions` |
| Anthropic | `https://api.scienceedu.me` | `POST /v1/messages` |
| Gemini | `https://api.scienceedu.me` | `POST /v1beta/models/{model}:generateContent` |

把密钥存入本地环境变量 `YUNQIAO_API_KEY` 后，可以用下面的请求查询该 Key 可见的模型。发起生成请求时使用返回列表中的模型 ID。

```bash
curl -fsS https://api.scienceedu.me/v1/models \
  -H "Authorization: Bearer ${YUNQIAO_API_KEY}"
```

完整的请求路径和各协议的鉴权方式见 [API 端点](https://docs.scienceedu.me/api/endpoints)。

## 网关源码

本仓库维护云桥词元使用的 TokenRouter 定制版本。TokenRouter 是从 Sub2API 演进而来的开源网关，负责接入上游模型服务、转发请求和记录用量。[上游项目 TokenFlux/TokenRouter](https://github.com/TokenFlux/TokenRouter)

| 功能 | 网关如何处理 |
| --- | --- |
| 上游接入 | 可以接入 OpenAI、Anthropic、Gemini、Antigravity、Grok、Qoder、Kimi、智谱和 DeepSeek。按平台选择 OAuth、API Key、AWS Bedrock 或 Vertex AI 等接入方式。 |
| 请求调度 | 按模型、协议、限流状态和粘性会话选择上游。分组可以配置协议转换，符合重试条件时网关可切换上游。 |
| 账号与配额 | 按用户、团队、分组和 API Key 管理访问权限、额度、并发和每分钟请求数。登录支持邮箱密码、Passkey、两步验证，以及 GitHub、Google、LinuxDo、OIDC、微信和钉钉。 |
| 计费与支付 | 记录调用用量和费用，管理余额、订阅、额度包、兑换码和邀请返利。可配置易支付、支付宝、微信支付、Stripe 和 Airwallex。 |
| 运营管理 | 查询用量日志与统计，配置告警和邮件报表，管理升级和回滚。内容审核可以按关键词和哈希拦截请求，并自动封禁违规用户。控制台提供图片生成与编辑，支持简体中文和英语。 |

图片、视频、语音、向量、搜索和 WebSocket 的支持情况取决于所用平台、模型与分组。[上游提供商能力矩阵](docs/interfaces/upstream_provider_matrix.md#public_gateway_protocols)

TokenRouter 界面示例可查看[浅色仪表盘](assets/screenshots/dashboard-light.jpg)、[浅色模型广场](assets/screenshots/models-light.jpg)、[深色仪表盘](assets/screenshots/dashboard-dark.jpg)和[深色模型广场](assets/screenshots/models-dark.jpg)。

## 部署与本地开发

后端使用 Go，前端使用 Vue 3。Docker Compose 可以从当前检出的源码构建网关，并启动 PostgreSQL 和 Redis。

首次运行时，先复制 `deploy/.env.example` 为 `deploy/.env`，再设置 `POSTGRES_PASSWORD` 和 `ADMIN_PASSWORD`。启动命令从仓库根目录运行。

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.dev.yml up --build
```

使用默认配置时，在浏览器打开 `http://127.0.0.1:8080`。首次初始化时，如果 `ADMIN_PASSWORD` 留空，应用会随机生成管理员密码并写入启动日志。

- [部署指南](docs/guides/deployment/index.md)介绍镜像部署、初始化和升级方法。
- [开发、验证与上游同步](docs/operations/development_workflow.md)列出工具版本、前后端独立启动方式和测试命令。
- [工程文档](docs/index.md)记录网关架构、业务规则、接口和运维要求。[升级说明](docs/operations/upgrade_notes.md)记录版本升级时需要处理的事项。
- [使用与运维指南](docs/guides/index.md)包括部署、支付配置和外部系统集成。

## 许可证与上游资源

本项目依据 [GNU Lesser General Public License v3.0 或更高版本](LICENSE) 发布。

Copyright (c) 2026 Wesley Liddick & TokenFlux

[TokenRouter 英文说明](README_EN.md) · [上游发布](https://github.com/TokenFlux/TokenRouter/releases) · [上游 Docker 镜像](https://github.com/TokenFlux/TokenRouter/pkgs/container/tokenrouter) · [上游 Discord](https://discord.gg/4tPj7uk4)

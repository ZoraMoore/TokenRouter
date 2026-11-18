# 创建 API Key

使用云桥词元 API 前，需要先创建一个 API Key。API Key 是访问模型网关的凭据；每个 Key 都关联用户或团队、服务分组、计费策略和独立限制。

**直接前往：[创建 API Key](https://api.scienceedu.me/keys)**

## 创建步骤

1. 登录后进入[API 密钥页面](https://api.scienceedu.me/keys)。
2. 点击右上角的“创建密钥”按钮。
3. 输入一个便于识别的名称，例如 `cherry-studio`、`cc-switch` 或 `macbook-codex`。
4. 选择一个分组。下拉列表会显示每个分组的倍率和当前容量。
5. 点击创建，复制生成的密钥并妥善保存。

::: warning 请立即保存密钥

密钥完整内容仅在创建时显示一次。请立即复制保存；遗失后请创建替代 Key，而不是尝试找回旧 Key 的完整内容。

:::

<div style="text-align: center;">
  <img src="/images/api-key/group-pricing.png" width="680" alt="云桥词元 API Key 编辑界面中的分组倍率与价格列表" />
</div>

不同分组的倍率与价格可能不同，请查看[模型、分组与计费](/guide/models-and-billing)。

## 创建后做什么？

拿到 API Key 后，可以直接开始接入：

- [API 端点](/api/endpoints)：客户端需要填写的 API 地址；
- [快速开始](/guide/quickstart)：根据使用习惯选择接入方式；
- [Cherry Studio 使用指南](/clients/cherry-studio)：桌面端 AI 对话客户端；
- [RikkaHub 使用指南](/clients/rikkahub)：Android 端 AI 对话客户端；
- [CC-Switch 使用指南](/clients/cc-switch)：管理 Claude Code、Codex 等客户端的配置工具。

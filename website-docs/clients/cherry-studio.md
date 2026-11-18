# Cherry Studio 配置

Cherry Studio 可使用“OpenAI 兼容”服务商连接云桥词元。

## 添加服务商

在 Cherry Studio 设置中添加 OpenAI 兼容服务商：

| 配置项 | 填写内容 |
| --- | --- |
| 服务商名称 | 云桥词元 |
| API 地址 | `https://api.scienceedu.me/v1` |
| API Key | 你的 `sk-...` Key |
| 模型 | 从 `/v1/models` 获取的模型 ID |

不同版本可能把“API 地址”称为“Base URL”或“接口地址”。如果客户端自动追加 `/v1`，则填写 `https://api.scienceedu.me`；可在连接测试或错误信息里查看最终请求路径，确保没有 `/v1/v1`。

## 获取模型列表

如果客户端支持“获取模型”，可直接同步。否则先用命令查询：

```bash
curl https://api.scienceedu.me/v1/models \
  -H "Authorization: Bearer sk-YOUR_API_KEY"
```

把响应中的模型 `id` 添加到 Cherry Studio，不要填写模型的展示名称。

## 建议

- 为桌面客户端单独创建一个 Key，便于撤销和统计。
- 给该 Key 设置合理的总配额与滚动限额。
- 不要截图分享包含完整 Key 的设置页面。
- 连接测试失败时先检查最终 URL、HTTP 状态码和 Key 分组协议。

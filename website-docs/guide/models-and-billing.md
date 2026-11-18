# 模型、分组与计费

云桥词元以“分组”组织上游模型能力。创建 API Key 时选择分组，请求时再使用该分组公开的模型 ID。

## 获取当前模型

最直观的方式是打开[模型广场](https://api.scienceedu.me/models)。程序也可以查询：

```bash
curl https://api.scienceedu.me/v1/models \
  -H "Authorization: Bearer sk-YOUR_API_KEY"
```

只使用返回列表里的模型 ID。模型可能新增、改名或临时下线，不建议把文档中的示例 ID当作永久清单。

## 价格怎么理解

模型广场会展示分组倍率和模型价格。实际扣费可能受到以下因素影响：

- 输入、缓存输入、输出等不同 token 类型；
- 图片、音频、视频或搜索等非 token 计费单位；
- 分组倍率与明确配置的模型价格；
- 长上下文价格区间；
- 订阅额度、余额或 Key 指定的计费模式。

发起大批量任务前，建议先用少量请求核对控制台的用量明细。

## 查询 Key 用量

可在主站[用量查询](https://api.scienceedu.me/key-usage)页面粘贴 Key 查询，也可调用：

```bash
curl https://api.scienceedu.me/v1/usage \
  -H "Authorization: Bearer sk-YOUR_API_KEY"
```

响应会包含 Key 自身限额及 `billing` 对象。常见字段：

| 字段 | 含义 |
| --- | --- |
| `billing.mode` | Key 的计费策略：自动、订阅或余额 |
| `billing.source` | 本次当前选用的资金来源 |
| `billing.available` | 当前来源是否仍可使用 |
| `billing.unit` | 返回额度的单位 |

`auto` 模式下，资金来源可能随有效订阅与余额状态变化；指定订阅失效时不会自动改扣余额。

## 可用性与容量

模型广场的可用率与容量是观测信息，不是永久承诺。高峰期可能出现限流或暂时不可用。生产程序应实现超时、有限重试和错误记录，详见[错误处理与排查](/api/errors)。

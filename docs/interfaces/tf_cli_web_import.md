# tf CLI 网页导入

本文记录 TokenRouter 为接入 [tf-cli 网页导入协议](https://github.com/TokenFlux/tf-cli/blob/main/docs/integrations/web-import.md) 所做的工作。端口、字段、错误码和签名格式以上游文档为准。

导入完全在浏览器和本机之间完成，TokenRouter 后端没有导入接口，也没有专门的授权接口。Keys 页把当前用户已经加载到内存里的 API Key 直接发给本机的 `tf`；Key 不会出现在 URL、浏览器持久化存储、日志或分析事件里。

<a id="session_fragment"></a>
## 会话片段

`tf login --from-web` 打开 `/keys#tfcli=1.<port>.<base64url-secret>`。`frontend/src/router/index.ts` 在创建 Router、运行登录守卫之前，调用 `initializeTfCliImportSession()`：

- 任何 `#tfcli=` fragment 都立即用 `history.replaceState` 删掉，格式错误的值也一样。
- 只接受 Keys 路径、`43110` 到 `43119` 的端口，以及 16 字节、规范格式、不带 padding 的 base64url secret。
- session 只在模块内存里保存十分钟；过期、终端取消，或者请求被终端接受后清空。
- 同一个 SPA 内的登录跳转可以继续使用 session；刷新页面或外部认证的整页跳转会丢掉证明，之后只能走未验证的兼容路径。

<a id="session_proof"></a>
## 回环协议

页面只访问 `http://127.0.0.1:43110` 到 `43119`。发现服务使用 `GET /ping`：有 session 时带上随机的 challenge，并用 Web Crypto 校验 tf 返回的 HMAC。导入使用 `POST /import`，只有在发现阶段的证明有效时，才发送 `X-TF-Session-Proof`。

"已验证当前 tf 会话"的意思仅仅是会话证明校验通过。proof 不能证明网页、API Key、本机程序的来源，也不能证明之后的网关结果，同样不提供时效性和防重放。没有 session、proof 不匹配或 Web Crypto 不可用时，仍然可以导入，但发送前要显示未验证的警告。已经验证过、但导入时 proof 过期或计算失败的，这一次不发送 Key；页面先降为未验证状态，再要求用户确认。

回环请求固定使用 CORS、`credentials: omit`、`cache: no-store`、`redirect: error`、`referrerPolicy: no-referrer` 和 `targetAddressSpace: loopback`。发现阶段的总预算是 30 秒。`OPTIONS` 预检由浏览器自动完成，前端不需要手动发送。

## 字段映射

- `host` 使用页面的 Origin。tf 校验同源后，会恢复成 CLI 自己保存的完整服务地址。
- `key_name` 取 Keys 页上的名称。tf 会保存这个来源元数据；用户没有在本地指定名称时，合法的值还会成为终端里的命名候选。
- 普通 Key 发送它的 `group_id` 和 `group_name`。
- 复合 Key 不发送单个分组的元数据，实际能力由 tf 查询模型目录识别。

HTTP `202 Accepted` 只表示终端已经确认收到。之后 tf 才校验网关；用户没有指定名称时，tf 还会让用户在自动识别、网页名称和自定义名称之间选择。所以页面只能提示"最终结果以终端为准"，"导入成功"或"Key 已保存"这样的文案在这个时候都不准确。

<a id="user_confirmation"></a>
## 页面交互

Keys 行的更多菜单里有"导入 TF CLI"，使用当前行的 Key。弹窗先发现服务，再显示已验证状态或未验证的警告；用户点击"发送到 TF CLI"之后，才会发送 Key。POST 等待期间，提示用户在终端核对来源并确认。网页上的确认决定是否发送 Key，终端上的确认决定 tf 是否继续处理，两次确认缺一不可。

<a id="browser_security_headers"></a>
## 浏览器安全头

默认 CSP、对旧自定义 CSP 的运行时补全，以及 `deploy/config.example.yaml`，都在 `connect-src` 里精确列出十个回环 Origin。范围限定在这十个地址；端口通配符、`localhost` 和局域网地址都不在其中。

TokenRouter 默认不限制 `local-network-access`。如果反向代理自己设置了 `Permissions-Policy`，需要允许顶层页面访问本地网络。较新的 Chromium 第一次访问时可能请求本地网络权限；用户拒绝时，页面显示未找到本机会话，并允许重试。

## 验证

- `tfCliImport.spec.ts`：fragment、过期、固定的 HMAC 向量、验证降级、请求选项和 proof。
- `TfCliImportDialog.spec.ts`、`KeysView.spec.ts`、`KeyActionMenu.spec.ts`：发送确认、状态文案、字段映射和菜单入口。
- `security_headers_test.go`：十个精确的 CSP Origin，以及与默认策略的同步。

相关文档：[接口目录](index.md)、[配置](configuration.md)、[复合 API Key](../domains/composite_api_keys.md)。

# 边缘与 HTTP 入口安全

TokenRouter 支持长时间运行的 SSE 和 WebSocket 请求。入口保护不能依赖响应 `WriteTimeout`，因为写超时会终止正常的长耗时生成和流式响应。

本文说明应用与反向代理的入口限制、可信客户端 IP 解析、流式传输和 DDoS 防护分工。它不能替代云厂商防火墙或 CDN 策略，也不定义已认证提供商的并发和计费限速。

## 章节导航

- [应用默认值](#http_ingress_limits)：修改 HTTP/H2C 限制或请求体分类时读取。
- [固定窗口限流](#fixed_window_rate_limits)：修改 Redis 计数、TTL、故障策略或限流响应时读取。
- [可信客户端 IP](#可信客户端-ip)：修改代理信任、IP ACL 或运行时安全设置时读取。
- [Nginx 基线](#nginx-基线)：使用 Nginx 作为边缘入口时读取。
- [Caddy 与 CDN](#caddy-与-cdn)：修改仓库内 Caddyfile 或接入 CDN 前读取。
- [DDoS 防护分工](#ddos-防护分工)：判断防护应该放在应用还是网络边缘时读取。

<a id="http_ingress_limits"></a>
## 应用默认值

- `server.max_header_bytes: 65536` 把 HTTP/1 请求头限制为 64 KiB，Go 会把它映射为对应的 HTTP/2 Header List 限制。
- `server.read_header_timeout: 10` 限制慢请求头攻击，不限制请求处理时间或响应流时长。
- `server.max_request_body_size: 268435456` 是绝对的 256 MiB 安全上限。
- `gateway.max_body_size: 268435456` 继续供多模态、Gemini、图片、视频和批量图片端点使用。
- `gateway.text_max_body_size: 33554432` 把已知纯文本 `/embeddings` 和 `/alpha/search` 端点限制为 32 MiB。
- H2C 默认每条连接最多 50 个并发流，连接上传窗口为 2 MiB，单流上传窗口为 512 KiB。
- 进程内按可信客户端 IP 限制无效凭据滥用，IPv6 聚合到 `/64`：60 秒内允许 120 次失败，超过后封禁 60 秒。这只是单实例安全网，多实例强制限制仍应由负载均衡器、CDN 或 WAF 完成。

不要增加全应用共用的请求信号量，因为一个 SSE 请求可能正常占用它数分钟。连接和未认证请求限制应放在边缘；已认证用户或 API Key 的并发仍由应用负责。

HTTP/1 的连接空闲超时使用 `server.idle_timeout`，h2c 使用 `server.h2c.idle_timeout`。HTTP/2 配置通过 `x/net/http2.ConfigureServer` 传给标准库，分别应用两个协议的空闲超时；标准库公开的 `http.HTTP2Config` 暂无独立空闲超时字段。

<a id="fixed_window_rate_limits"></a>
## 固定窗口限流

`infra/redis.FixedWindowLimiter` 通过 Lua 原子递增，首次计数设置 TTL，并修复已有计数键缺失 TTL 的情况；后续请求不延长窗口。应用仍使用 `rate_limit:` 前缀。超限时尽力读取剩余 PTTL，查询失败时回退到完整窗口，HTTP 层继续向上取整输出 `Retry-After`。

`server/middleware.RateLimiter` 通过只返回标量结果的计数接口适配 HTTP，拥有客户端 IP 维度、故障策略和 429 格式。公开的高风险认证路由手动设为 fail-close；面板限流 fail-open，认证请求按用户 ID 分桶，公开请求只按可公开路由的客户端 IP 分桶。设置有缓存，管理员豁免限流，没有 Redis 时面板放行。计数器由路由装配创建，不由 HTTP 中间件构造数据库客户端。

<a id="trusted_client_ip"></a>
## 可信客户端 IP

地址解析和请求快照由 `server/clientip` 拥有；纯 IP/CIDR 编译与匹配由 `pkg/ipmatch` 提供。Key 的黑白名单由 apikey 的认证用例判断；IP 匹配成功只是其中一步，授权还要通过认证用例的其他检查。

为兼容升级，`security.trust_forwarded_ip_for_api_key_acl` 默认开启。开启后，原始转发请求头接管日志与安全敏感路径的客户端 IP 解析。`security.forwarded_client_ip_headers` 中的自定义请求头按配置顺序检查，优先于内置的 `CF-Connecting-IP`、`X-Real-IP` 和 `X-Forwarded-For` 回退。请求头名称不区分大小写，加载时会规范化并去重，最多允许 16 个唯一且有效的 HTTP 字段名。请求头值必须包含 IP 字面量；支持逗号分隔，跳过无效项，有公网地址时优先使用公网地址，私网地址作为回退。

该列表可以通过 YAML 或逗号分隔的 `SECURITY_FORWARDED_CLIENT_IP_HEADERS` 环境变量提供；把环境变量明确设为空，会清除 YAML 里的值。管理后台安全设置也可以编辑它，并在无需重启的情况下更新运行状态。每个请求同时取开关和请求头列表的快照，同一次请求里用的都是同一个版本的设置。开关关闭时完全忽略自定义请求头，此时以 Gin 的 `server.trusted_proxies` 链为准，只能配置直接连接 TokenRouter 的准确 CIDR 或 IP。配置为空列表时，所有转发来的客户端 IP 都不被信任。

首次升级到该模式时，只有在 `server.trusted_proxies` 没有配置的情况下，旧的 `false` 才会改成 `true`；已经明确配置了代理策略的环境，继续使用安全模式。新安装在数据库初始化期间持久化自定义请求头列表。旧安装会用 YAML 配置回填缺失的数据库值。隐藏迁移标记会阻止后续管理员修改被再次覆盖。设置读取失败或已持久化的自定义请求头列表格式错误时，进程会以无自定义请求头的可信代理模式安全关闭兼容路径。迁移写入失败时，本次进程仍使用计算后的模式，并在启动日志记录警告。

兼容接管模式会接受转发请求头而不校验直接对端，包括任何已配置的自定义请求头。开启时必须阻止外部直接访问源站。CDN 部署应通过防火墙只允许 CDN 或负载均衡器连接源站，并由这个代理整个覆盖每个可信客户端 IP 请求头；如果只是在客户端传来的值后面追加，客户端就能伪造 IP。

同主机代理示例：

```yaml
server:
  trusted_proxies:
    - 127.0.0.1/32
    - ::1/128
```

## Nginx 基线

在 `http` 块中定义共享区域。限速值要根据实际的合法流量调整；下面的值只是一个保守的起点。

### 已认证入站模板采集

项目已移除 Nginx/TShark 匿名旁路采集器及其安装、定时运行、报告和日志轮转文件。正常 API 流量的 TLS 模板采集使用下述 `authenticated_ingress` 链路。历史匿名样本没有同连接鉴权证据，不能导入、补标或提升为已认证模板。

`authenticated_ingress` 为正常访问 `api.scienceedu.me` 的客户端提供零额外配置采集：客户继续使用自己的 Base URL 和 API Key，不需要 HTTPS CONNECT 代理、额外 CA 或采集命令。Linux 主机上的 `tls-ingress-observer` 通过 AF_PACKET 只读观察发往 Nginx 443 的 TCP 流，有限重组完整 ClientHello，校验 SNI 后把去敏结构提交到主服务；它不进入转发链，不解密或转发 HTTP。observer 与 Nginx 必须看到同一入站五元组，并与 TLS 终止 Nginx 位于相同主机网络视图。若 CDN、L4 负载均衡、容器 NAT 或其它 TLS 终止层改写了这组地址/端口，关联会失败或观察到代理指纹，因此生产域名必须直接在该 Nginx 终止客户端 TLS，Cloudflare 等只能使用 DNS Only。

observer 使用 `observer_token` 对规范化后的源/目标 IP 与端口生成 HMAC flow ID；提交载荷不含明文 IP、端口、原始包、TLS random、session ticket、PSK/key-share 公钥或密钥材料。Nginx 则必须覆盖注入下列内部头，不能追加或透传客户端同名值。`edge_token` 与 `observer_token` 必须是两个不同的至少 32 字节随机 secret，只能由 Nginx、observer 和主服务读取，也不能写入 access log：

```nginx
# 建议放入 root-only include；示例值必须替换，且不得提交到仓库。
proxy_set_header X-TokenRouter-Ingress-Edge-Token "<独立 edge_token>";
proxy_set_header X-TokenRouter-Ingress-Connection-Id $connection;
proxy_set_header X-TokenRouter-Ingress-Connection-Requests $connection_requests;
proxy_set_header X-TokenRouter-Ingress-Remote-Addr $realip_remote_addr;
proxy_set_header X-TokenRouter-Ingress-Remote-Port $realip_remote_port;
proxy_set_header X-TokenRouter-Ingress-Resolved-Client-Addr $remote_addr;
proxy_set_header X-TokenRouter-Ingress-Server-Addr $server_addr;
proxy_set_header X-TokenRouter-Ingress-Server-Port $server_port;
proxy_set_header X-TokenRouter-Ingress-Session-Reused $ssl_session_reused;
proxy_set_header X-TokenRouter-Ingress-Alpn $ssl_alpn_protocol;
proxy_set_header X-TokenRouter-Ingress-Http-Protocol $server_protocol;
```

启用 RealIP 模块时，`Remote-Addr`/`Remote-Port` 必须改用 `$realip_remote_addr`/`$realip_remote_port` 表示 observer 实际看到的 TCP 对端，而 `Resolved-Client-Addr` 保持 `$remote_addr`。两者地址不相同表示可信 CDN/代理已经重终止客户端 TLS，主服务会 fail-close 拒绝该指纹关联，防止把 Cloudflare 指纹错误标为客户客户端；DNS Only 直连时两者应相同。

主服务仅在完整 API Key 鉴权成功后处理这些头，并立即从请求中删除；采集组件只收到白名单连接/客户端字段及 API Key/User 两个内部数字 ID，不接触完整 API Key 领域对象、Authorization、Cookie、请求体或原始凭据。每条 TLS 连接只接受 `$connection_requests=1` 且 `$ssl_session_reused=.` 的首个已认证请求，并要求 ClientHello 观测先于鉴权且时间差不超过五秒；PSK/ticket、重放或端口复用造成的迟到关联均被拒绝。候选按本站 SNI、模板摘要和固定验证目标全局去重，客户 ID、flow ID 与连接元组不进入持久模板。当前分类器接受带版本的 Codex（`codex_cli_rs`、`codex-tui`、`codex_vscode`）、Claude Code（`claude-cli`）和 Node（`openai/nodejs`、`node`/`node.js`）User-Agent；其它或无法解析版本的客户端正常获得 API 服务，但不会进入模板候选。

同一全局模板摘要累计至少三个不同完整连接后，主服务先执行本地 uTLS 精确回放，再依次对配置中的 `chatgpt.com` 与 `api.openai.com` 做无凭据 TLS-only 握手。客户端版本只作为分类证据，不参与全局模板去重；每日验证与持久模板创建另有固定上限。任一重组、配对、分类、回放、目标验证、内存容量或数据库写入失败都只丢弃/拒绝该观测，不能阻塞 API 鉴权、网关请求或 Nginx；observer 停止、队列满或内部入口不可用同样 fail-open 到正常业务。验证全部成功且 `auto_create_verified=true` 时自动创建 `source_kind=authenticated_ingress` 的 `verified` 模板；自动模板保持未批准且不进入随机池，管理员显式发布前不能绑定账号或 Router。

pending ClientHello/鉴权事件默认保留 300 秒，未凑够连接的候选默认保留 24 小时，进程重启即丢失；容量上限采用有界淘汰。持久模板不保存客户 ID，但客户端版本、OS/架构（若 User-Agent 可解析）、本站 SNI 和时间仍可能构成运维数据，须限制管理员访问并按隐私说明和删除策略处理。“客户零额外配置”只表示无需改变客户端网络或安装 CA，不取消部署者对采集用途、字段、留存和退出方式的透明告知及适用法律义务。此链路只验证 TLS ClientHello 和目标握手；Nginx 注入的 ALPN/HTTP 协议只供短期关联上下文，当前不采集或复现 HTTP/2 SETTINGS、伪头顺序、WINDOW_UPDATE、PRIORITY、流复用、TCP 或请求时序。

```nginx
limit_conn_zone $binary_remote_addr zone=tokenrouter_conn:20m;
limit_req_zone  $binary_remote_addr zone=tokenrouter_auth:20m rate=5r/s;
limit_req_zone  $binary_remote_addr zone=tokenrouter_api:40m rate=30r/s;
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;

    client_header_timeout 10s;
    client_max_body_size 256m;
    large_client_header_buffers 4 16k;
    limit_conn tokenrouter_conn 40;

    location ~ ^/(auth|api/auth|api/v1/auth)/ {
        limit_req zone=tokenrouter_auth burst=10 nodelay;
        proxy_pass http://127.0.0.1:8080;
    }

    location ~ ^/(v1/)?(embeddings|alpha/search)$ {
        client_max_body_size 32m;
        limit_req zone=tokenrouter_api burst=60 nodelay;
        proxy_pass http://127.0.0.1:8080;
    }

    location / {
        limit_req zone=tokenrouter_api burst=60 nodelay;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 1800s;
        proxy_send_timeout 1800s;
        proxy_pass http://127.0.0.1:8080;
    }
}
```

如果在 `http` 块中启用 Nginx gzip，必须从 `gzip_types` 排除 `text/event-stream`，也不能为 TokenRouter 使用 `gzip_types *`。上面的 `proxy_buffering off` 会关闭代理缓冲，但不会关闭 gzip 响应过滤器。普通响应应使用明确列表：

```nginx
gzip on;
gzip_types text/plain text/css application/json application/javascript application/xml image/svg+xml;
```

如果共享全局配置无法按内容类型排除 SSE，应在提供流式 API 的 location 中设置 `gzip off;`，同时保留 Web 管理界面和静态资源的 gzip。

除非 Nginx 的 Real IP 处理只信任明确的代理 CIDR，否则不要使用入站 `$http_x_forwarded_for` 值。

## Caddy 与 CDN

仓库内 `deploy/Caddyfile` 设置了 64 KiB 请求头上限、10 秒请求头超时和 256 MiB 绝对请求体上限，并根据 TCP 对端覆盖转发地址，因此它是客户端直连 Caddy 的基线。Caddy 位于 CDN 后面时，其中的 `{remote_host}` 转发行要按下面的方式修改；原样使用的话，所有客户端都会被算成 CDN 的出口地址，使拒绝聚合和无效认证限速错误地作用于互不相关的用户。

仓库内 Caddy 配置没有设置 `flush_interval`，让 Caddy 自动刷新 `text/event-stream` 响应，同时把客户端取消向上游传播。不要全局设置它：正值会增加流式延迟，而 Caddy 2.6.2 的特殊 `-1` 模式还会让反向代理请求在客户端断开后继续执行。配置用一个明确的响应内容类型列表决定压缩哪些内容。`text/*` 或简写的 `encode gzip zstd` 都会匹配 `text/event-stream`，可能把 SSE 缓冲到响应结束才发出，所以不要这样替换。流式响应应保持不压缩，Web 管理界面、JSON 和静态资源仍可压缩。

CDN 部署应先通过防火墙限制源站，只允许当前 CDN 出口 CIDR 连接；再把这些准确网段配置为 Caddy 可信代理，并根据 Caddy 解析后的 `{client_ip}` 生成上游请求头。例如：

```caddyfile
{
	servers {
		trusted_proxies static 192.0.2.0/24 2001:db8:1234::/48
		trusted_proxies_strict
		client_ip_headers CF-Connecting-IP X-Forwarded-For
	}
}

api.example.com {
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {client_ip}
		header_up X-Forwarded-For {client_ip}
	}
}
```

必须把示例网段替换为 CDN 已发布且自动维护的出口网段。只有在源站直连被阻止且 Caddy 只信任这些 TCP 对端时，`CF-Connecting-IP` 才是安全的。TokenRouter 的 `server.trusted_proxies` 应配置为 Caddy 地址或私有子网，使应用只接受 Caddy 重写的请求头。

Caddy 核心不提供通用请求限速器，应使用可信 CDN/WAF、受支持的限速模块或宿主机防火墙控制。

在 CDN/WAF 上，应在流量到达源站前配置连接数限制、请求头和请求体限制、机器人挑战，以及按 IP 或 ASN 的速率限制。源站只允许 CDN 的出口 CIDR 或私有负载均衡器访问，应用端口不直接暴露到公网。

## DDoS 防护分工

应用里的检查，只能减少连接到达 Go 进程之后的放大效应；容量型攻击、TLS 洪泛、带宽打满和大规模的分布式来源，都需要在网络边缘处理，依靠上游的网络容量、CDN 和 WAF 过滤、云厂商的防火墙规则和源站隔离。拒绝风暴期间，避免使用高基数的指标，也不要逐个请求写数据库安全日志。

相关文档：[HTTP 接口](../interfaces/http_api.md)、[配置](../interfaces/configuration.md)、[网关请求生命周期](../architecture/gateway_request_lifecycle.md)、[可观测性与数据生命周期](observability_and_data_lifecycle.md)和[运维目录](index.md)。

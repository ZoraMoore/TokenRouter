# 部署与数据库迁移

本文记录 TokenRouter 的构建产物、运行方式、首次初始化、数据库迁移、升级和恢复的工程约束。安装命令见部署手册；本文用于核对启动、迁移和恢复的兼容要求。各次迁移的升级步骤见[版本升级说明](upgrade_notes.md)。

## 章节导航

- [构建与运行形态](#构建与运行形态)：修改产物或部署拓扑时读取。
- [产品名称与升级兼容](#product_name_compatibility)：从旧命名的部署升级时读取。
- [初始化与启动](#初始化与启动)：修改 setup、配置或健康检查时读取。
- [迁移执行](#migration_execution)：修改 runner 或迁移格式时读取。
- [新增与同步迁移](#新增与同步迁移)：新建本 fork 的迁移或同步上游时读取。
- [升级与恢复](#升级与恢复)：修改更新、备份或回退流程时读取。

## 构建与运行形态

标准发布先生成一次前端静态资源，再由独立的 runner 把同一份前端嵌进各平台的 Go 二进制，并行交叉编译；最后的发布阶段统一归档二进制，并用 Linux `amd64` 和 `arm64` 的产物组装多架构镜像和必要的运行时工具。GitHub Release 同时发布 Linux `amd64`、Linux `arm64` 等产物，具体的矩阵以 [release workflow](../../.github/workflows/release.yml) 为准。

源码镜像的前端阶段校验 `.node-version`，再按 `frontend/package.json` 的 `packageManager` 字段安装 pnpm，本地和 CI 读取同样的版本声明。默认 Node 镜像使用 26.10.0，覆盖 `NODE_IMAGE` 时仍需满足完整版本检查。Node 26 镜像通过 npm 安装声明的 pnpm 版本，冻结安装前复制 workspace 配置。

前端构建会导入 `backend/internal/pkg/locale/manifest.json`、`error_messages.json` 和 `backend/internal/provider/openai_defaults.json`。根目录和 `deploy/` 的 Dockerfile 在前端构建阶段将这些文件复制到 `/app/backend/` 下对应的目录，与源码中的相对导入路径一致。

定制镜像由 `deploy/Dockerfile` 构建，应用入口为 `/app/tokenrouter`。前端构建阶段将 Node.js 的堆上限设为 4096 MiB。

仓库支持以下运行方式：

| 方式 | 入口 | 依赖和说明 |
| --- | --- | --- |
| 单二进制、systemd | `deploy/install.sh` | 外部的 PostgreSQL 和 Redis；安装器管理二进制版本和服务 |
| 完整 Compose | `deploy/docker-compose.yml`、`docker-compose.local.yml` | 应用、PostgreSQL、Redis；分别使用命名卷或本地目录 |
| 独立应用容器 | `deploy/docker-compose.standalone.yml` | PostgreSQL 和 Redis 由部署环境提供 |
| 源码开发 Compose | `deploy/docker-compose.dev.yml` | 在本地构建应用，并启动配套的依赖 |
| Apple Container | `deploy/apple-container.sh` | 独立脚本管理容器、卷和健康状态 |

应用至少依赖 PostgreSQL 和 Redis。`/app/data` 或等价的 `DATA_DIR` 保存配置、安装锁和本地的运维产物；数据库、Redis 和对象存储各有自己的生命周期，只备份应用数据目录，不算完成了系统备份。

逐步操作见[中文部署指南](../guides/deployment/index.md)、[Docker 镜像说明](../../deploy/DOCKER.md)和 [Apple Container 指南](../guides/deployment/apple_container.md)。这些是给部署者的手册，本文的工程约束在它们之上。

旧的 data management 接口已经下线：`backup` 的兼容入口固定返回 `DATA_MANAGEMENT_DEPRECATED`，不会连接 Unix Socket，也不会发起 gRPC 调用。仓库里的旧安装脚本和 [datamanagementd 指南](../guides/deployment/datamanagementd.md) 是历史资料，当前服务没有这个守护进程。备份和恢复使用独立的 backup 模块。

<a id="product_name_compatibility"></a>
## 产品名称与升级兼容

新的产物、安装目录、systemd 服务、容器用户和 Compose 服务都使用 `tokenrouter`。镜像保留 `/app/sub2api` 兼容链接，容器的 UID 和 GID 仍是 1000。安装器识别到 `/opt/sub2api`、旧配置和旧 unit 时，继续使用原来的目录、可执行文件路径、服务和用户；同时发现新旧两套安装时停止，以免覆盖另一套部署。

标准发布的归档命名为 `tokenrouter_<版本>_<系统>_<架构>`，包含 `tokenrouter` 可执行文件。CI 将矩阵编译的二进制导入 GoReleaser，所有归档都写进 `checksums.txt`。当前更新器优先使用 `tokenrouter_` 归档，并支持读取历史发布的 `sub2api_` 归档，替换的位置是当前实际的可执行文件路径。只识别 `sub2api_` 归档或包内 `sub2api` 文件的旧更新器需要手动升级。

官方价格补充和离线模型目录都内嵌在二进制里，后台在线更新、安装脚本、完整归档和 Docker 使用同一份默认数据。替换或回退二进制时，官方补充随版本切换，不需要额外安装资源文件。安装器不会创建或覆盖外部的价格补充。`pricing.fallback_file` 默认为空，手动配置的自定义文件照常读取，并优先于内嵌补充；旧的打包路径按[配置兼容规则](../interfaces/configuration.md#configuration_sources)处理。不再需要旧文件时，清空这个配置，文件本身由部署者管理。

已有的 Compose 部署保留原来的编排和 `.env`，只升级镜像即可。换用新模板时，先记下实际的应用、PostgreSQL 和 Redis 卷名，分别设置 `TOKENROUTER_DATA_VOLUME`、`TOKENROUTER_POSTGRES_VOLUME`、`TOKENROUTER_REDIS_VOLUME`；同时保留数据库名、用户和密钥，停止旧栈后再启动新模板，不要使用 `down -v`。新变量为空时，按 Compose 项目前缀生成新的卷名。

Apple container 的新栈使用 `org.tokenrouter.stack` 标签；已有的 `sub2api-apple*` 资源，继续按 `org.sub2api.stack` 标签核对归属并原地复用。两种资源同时出现，或者归属不符时，脚本停止。脚本和旧脚本共用同一把互斥锁，新旧脚本不会同时修改同一个栈。`TOKENROUTER_ENV_FILE` 和 `APPLE_CONTAINER_TOKENROUTER_IMAGE` 分别兼容对应的旧变量。

迁移 283 只修改恰好等于旧品牌名的展示设置。历史 SQL 文件和校验和不变，数据库、数据卷和安装锁都不改名。升级前照常按本页的要求备份；回退二进制不会把站点名称自动改回旧品牌。

## 初始化与启动

进程入口先判断是否需要 setup。尚未安装时，可以使用 Web setup、`--setup` 命令行，或容器的 `AUTO_SETUP`。setup 依次测试 PostgreSQL 和 Redis、执行迁移、创建第一个管理员、写入配置，最后创建只读的安装锁。安装锁用来防止被人重新初始化，普通的配置问题不能靠删除它来修复。

正常启动时，`app/bootstrap` 在构造业务依赖图之前，执行同一套嵌入的迁移，所以每个新版本都在监听 HTTP 之前完成 schema 对齐。迁移或安全密钥初始化失败时，应用初始化失败，不会带着不完整的 schema 提供服务。默认的兼容迁移允许多实例滚动启动，由迁移锁保证只有一个实例执行 SQL；[版本升级说明](upgrade_notes.md)里标记为一次性或破坏性的变更，需要按各自的停机顺序执行，优先于这条默认规则。

`GET /health` 是容器的健康检查入口。健康响应只说明当前进程可以服务，升级后的业务抽样、账本核对和后台任务检查仍然要做。

<a id="migration_execution"></a>
## 迁移执行

迁移执行器在 `infra/postgres`，由 bootstrap 传入连接和迁移 FS；setup 和两个维护命令只调用需要的精简初始化能力。`backend/migrations/*.sql` 通过 `go:embed` 编进二进制，文件名就是迁移的身份，并按字典序决定执行顺序。执行器使用一个固定的 PostgreSQL advisory lock，让多个实例的迁移串行执行；`schema_migrations` 记录文件名、去掉首尾空白后的 SHA-256 和执行时间。已有文件的校验和不匹配时，启动失败；只有 runner 里逐个列出的历史兼容文件可以放行。

普通的 `*.sql` 在单个事务里执行，SQL 和迁移记录一起提交或回滚。包含 `CREATE INDEX CONCURRENTLY` 或 `DROP INDEX CONCURRENTLY` 的文件，需要以 `_notx.sql` 结尾：这种模式在事务外逐条执行语句，只接受带 `IF NOT EXISTS` 或 `IF EXISTS` 的并发索引语句，也不允许出现 `BEGIN`、`COMMIT` 和 `ROLLBACK`。非事务迁移可能在 SQL 成功、但记录还没写入时中断，所以每条语句都要能安全地重复执行。

第一次检测到旧的 `schema_migrations`、但缺少 Atlas 记录时，runner 用当前最后一个迁移建立 `atlas_schema_revisions` 基线。这条兼容记录不影响 SQL 文件作为 schema 来源的地位。

迁移只向前执行，已发布的文件保持原样：

- 已经进入任何环境的文件，不能修改、删除或改名；修正需要新建迁移。
- 文件里不放可执行的 Down 段；runner 不解析 Goose 的 Up 和 Down 标记。
- 普通迁移尽量写成幂等的，并在 SQL 里用中文注释说明变更原因和兼容窗口。
- schema、数据回填、Repository 查询和 Ent schema 的变化，放在同一个兼容序列里设计；确实无法让新旧二进制共存时，在[版本升级说明](upgrade_notes.md)里写明停机升级、备份和回滚的步骤。

## 新增与同步迁移

新文件命名为 `<递增数字>_<snake_case 描述>.sql`；并发索引命名为 `<递增数字>_<描述>_notx.sql`。仓库历史上有重复的编号和字母后缀，这些不能当作复用编号的理由。每次新建之前，扫描 `backend/migrations/` 的数字前缀，取当前最大值加一，并确认按字典序排在预期的位置。

本 fork 同步上游时，上游在 `backend/migrations/` 新增的迁移不能按原名照搬：按上游的提交顺序，逐个把数字前缀改成本 fork 当前最大编号加一，同时更新测试、runner 特例、文档和其他引用了原文件名的地方。已经在 fork 里的迁移保持原名，不为了"整理顺序"重新编号。

迁移变更至少验证：

- runner 单元测试，包括事务模式、`_notx` 校验、锁和 checksum；
- 受影响的 schema 和数据迁移测试；
- 从空数据库完整执行一遍，以及在已有的 schema 上重复执行；
- `schema_migrations` 里的文件名符合预期，已有文件的 checksum 没有变。

## 升级与恢复

### 通用规则

升级之前，先创建 PostgreSQL 备份并实际验证可以恢复，同时保存业务需要恢复的 Redis 和对象存储数据。后台备份服务可以把数据库 dump 流式写到本地或 S3 兼容存储，并用维护锁让备份和恢复串行执行；敏感的存储配置需要稳定的安全密钥。备份内容策略可能排除大的历史表，恢复之前要先核对备份的范围。

在线更新和安装脚本可以保留上一版的二进制或镜像，但这只能回退应用。镜像回退不会撤销数据库迁移；上线前要确认新迁移对旧版本是否向后兼容。schema 已经不兼容时，使用演练过的数据库备份恢复，或者新增一个前向修复的迁移；手工删除 `schema_migrations` 的记录不是回滚的办法。

升级完成后，至少检查：`/health`、登录和 API Key 鉴权、一个非流式和一个流式的网关请求、用量结算、关键的后台任务和迁移表。这些检查完成之前，保留旧产物和升级前的备份。

各次迁移的停机要求、缓存版本变化和回退方式，见[版本升级说明](upgrade_notes.md)。

<a id="maintenance_execution"></a>
### 备份与维护执行

备份配置、记录、定时和恢复的编排由 backup 负责，`backup/provider` 管理 dump 和 psql、压缩分卷、本地文件和 S3。系统更新和回退由 `ops/maintenance` 编排，技术适配层在可执行文件所在的目录下载并替换；版本查询复用 Ops 的发布查询实例（只有一个）。浏览器断开后，更新仍然继续；应用退出时，取消下载等准备工作；一旦进入二进制替换的临界区，就要么完成替换，要么恢复原文件。

备份停止时，立即拒绝新任务，取消启动回源、cron 和在途的任务，然后在应用剩余的后台预算内，等待子进程和清理完成。超时时保留未完成的状态，不算排空成功。恢复使用开启了 `ON_ERROR_STOP` 的单事务 psql；输入流损坏时，先取消进程再关闭标准输入，不完整输入的 EOF 不会被当成提交的条件。异步恢复只有在成功保存 running 记录之后才会启动。数据库最终提交之后，如果状态保存失败，数据库并没有回滚，不能据此认为恢复没有发生。

系统维护锁使用幂等表和 scope、key；processing 记录的 response_body 里保存一个独立的所有者令牌，续租和释放时，同时比较业务操作 ID 和令牌。旧的、没有令牌的记录，在原来的锁窗口内继续阻止认领，过期后可以被接管；普通的 HTTP 幂等记录不使用这套维护认领接口。回退旧二进制之前，先停止并等待维护操作完成，数据库恢复或二进制替换进行到一半时，不要切换实现。

相关文档：[系统架构](../architecture/system_architecture.md)、[配置](../interfaces/configuration.md)、[版本升级说明](upgrade_notes.md)、[运维目录](index.md)。


## 本部署的运行版本

2026 年 11 月 19 日的三个应用节点使用 `tokenrouter:0.2.10-pelican-html-20261119-1f59a2476`，镜像 ID 为 `sha256:bab47a2cb3c78d74ea1a2ae604985347a3e3b174d7d85d11fe4966cd757861f5`，源码提交为 `1f59a24769834e18121a4a4bc2d43919d4662bb3`。绘图详情支持完整 HTML 原文的预览、代码查看和下载，保存规则见[分组鹈鹕测智](provider_maintenance.md#pelican_group_tests)。

迁移 `303_pelican_raw_answer.sql` 为测试结果增加 `raw_answer` 列，历史记录使用空字符串。迁移表共 359 条，升级前 358 条记录的校验和一致。用户、API Key、分组、提供商、提供商分组关系、订单、价格配置及测试计划数量一致。发布逐节点等待现有连接结束后重建应用。三个节点的健康检查、首页及新前端资源通过验证，容器重启次数为 0。

发布按用户要求跳过备份。回退镜像为 `tokenrouter:0.2.10-custom-20261119-a7ef2a4d2`，旧应用可以忽略新增的原文列，回退时保留该列与迁移记录。发布镜像、版本及验证记录位于 `/home/ubuntu/.local/state/tokenrouter-pelican-html-20261119`，临时 SSH 凭据在验证后删除。

本地全量后端测试通过 211 个包，鹈鹕前端相关测试通过 47 项，历史迁移测试包及原文读写测试通过。源码提交的 [CI](https://github.com/ZoraMoore/TokenRouter/actions/runs/44403072765) 已通过。独立的 [Security Scan](https://github.com/ZoraMoore/TokenRouter/actions/runs/44403072558) 仍报告现有前后端依赖和 Go 标准库漏洞，依赖与编译器版本清单和更新前一致。

同日较早一次发布使用 `tokenrouter:0.2.10-custom-20261119-a7ef2a4d2`，镜像 ID 为 `sha256:5cc74598dd2e4d517004d715583d1f26909dac5d6873e218ec45084153da22d0`。源码来自 `ZoraMoore/TokenRouter` 的 `6f4b830b1f053fe540d6af4aea7589693f4ac716`，本机同步提交为 `a7ef2a4d279a236af1597ccab30e54b3db6d80f2`。远端快照 `e633356a3` 与本机同步前的 `1794c6bbf` 文件树相同，合并提交连接两份历史，运行代码采用远端版本。

镜像使用 Go 1.27.3 编译，前端由 Node 26.12.1 构建。运行层基于 `tokenrouter:0.2.10-header-activity-20261118-c7b73cb89`，更新主程序并移除 `/app/tls-ingress-observer`。构建产物、镜像归档、版本信息和节点核对记录位于 `/home/ubuntu/.local/state/tokenrouter-upgrade-20261119-6f4b830b1`。

迁移表共 358 条记录，最新为 `302_pelican_state_probe.sql`。新增的 299 至 302 为分组题库、作品展示和状态探针建立表及约束，已有迁移的校验和一致。升级前后的用户、API Key、分组、提供商、提供商分组关系、订单和价格配置数量一致。三个节点的私有首页、健康检查和全局白名单管理接口均返回 200，容器重启次数为 0。

OpenAI OAuth 全局最终模型白名单在“网关服务”的“OpenAI”设置中维护，规则见[运行时设置](../interfaces/configuration.md#runtime_settings)。本次部署后该名单尚未保存，模型准入继续使用账号自身的规则。

本次测试由用户完成，发布期间按要求停止测试套件并跳过数据库备份。主节点配置副本位于 `/mnt/data/sub2api/backups/remote-custom-20261119-6f4b830b1`，副节点配置副本位于各自的 `/opt/sub2api/backups/remote-custom-20261119-6f4b830b1`。回退镜像为 `tokenrouter:0.2.10-header-activity-20261118-c7b73cb89`。本次四个迁移增加题库相关表，旧应用可以忽略这些表；回退应用时恢复对应节点的配置副本和旧镜像，数据库中的迁移记录继续保留。

TLS 自动入站采集、observer 和模板验证按远端版本退役。主节点的三个 observer 容器、Compose 服务、专用环境变量、Nginx 请求头和 `18089`、`18090` 转发监听均已移除。公网 `/internal/` 路径继续受 Nginx 保护。运行日志中有升级前已存在的历史订单补发重试告警，详情保存在本次节点核对记录中。

2026 年 11 月 18 日的生产主服务使用镜像 `tokenrouter:0.2.10-header-activity-20261118-c7b73cb89`，主程序来自提交 `c7b73cb89bb072244834ff369d20e5f4ed1e6486`，业务定制基于 `af7fd1a3e982ff4b09331cd32b047b365c2c1666`。Edu 账号按套餐标识关闭加速，GPT 官key 分组使用 `follow_request`，规则见 [Fast 与 Ultra Fast 策略](../interfaces/openai_upstream.md#openai_fast_policy)。该版本包含 `18decad28b7157bf7e4890338cc1988d8f876351` 的 Fast 长上下文计价修复，账号 233 在分组 8 的使用记录、CSV 倍率列、个人密钥及本人所有的团队密钥把 0.2 展示为 0.14。预览记录使用响应式副本，费用、Token 和耗时明细浮层能够完成悬停目标比较。扣费、原始记录和后端接口使用计费数据，具体规则见[记录与对账](../domains/routing_and_billing.md#记录与对账)。镜像使用 Go 1.27.3 编译，运行于 Alpine 3.24.2，前端构建使用 Node 26.12.1。

Edu 兼容版本的源码、缓存构建配方和验收记录位于 `/mnt/data/edu-standard-20261106/`。后端完整单元测试有 210 个包通过。上线后已确认 Edu 的强制 Fast Key 按普通模式结算，商业 Pro 继续使用 Fast，价卡和提供商分组关联的校验值一致。主服务健康检查和首页返回 200，非法 Key 返回 401。迁移表有 354 条记录，最大迁移为 `298_preserve_custom_image_billing.sql`。本次按用户要求跳过备份。镜像回退使用 `tokenrouter:0.2.10-user-rate-tooltip-20261105-3b8c4ea1c`。

倍率浮层版本的源码、缓存构建配方和验收记录位于 `/opt/sub2api/data/builds/user-rate-preview-tooltip-20261105-3b8c4ea1c/`。浏览器渲染检查使用线上前端、只读数据夹具和模拟会话完成，截图与 `browser-render-verification.json` 同目录保存。配置恢复点位于 `/mnt/data/sub2api/backups/20261105T085736Z-usage-preview-tooltip/`，该版本的数据恢复引用已验收的 `/mnt/data/sub2api/backups/20261105T080541Z-usage-rate-preview/`。按用户要求，发布过程没有导出旧运行镜像。迁移表保持 354 条，最大迁移为 `298_preserve_custom_image_billing.sql`。Fast 长上下文修复和安全基础镜像的历史构建记录分别保存在 `/mnt/data/tokenrouter-pricing-release-20261023T112651Z/` 与 `/mnt/data/security-remediation-20261022-002329/`。

PostgreSQL 和 Redis 分别使用 `postgres:18.3-ca-security-20261022` 与 `redis:8.6.3-ca-security-20261022`。两张镜像更新了 CA 证书包，数据目录分别挂载 `/mnt/data/sub2api/postgres` 和 `/mnt/data/sub2api/redis`。宿主机与容器的证书库分别维护，升级后都要核对 `emSign ECC Root CA - G3` 已从可信根中移除。

## 本部署的数据盘布局

系统盘保存操作系统、Nginx、证书、Docker 镜像、源码和部署配置。数据盘挂载在 `/mnt/data`，文件系统 UUID 为 `c0f0d898-52a5-4335-b93f-b82e632559ff`。

| 服务 | 宿主机目录 | 容器目录 |
| --- | --- | --- |
| PostgreSQL | `/mnt/data/sub2api/postgres` | `/var/lib/postgresql/data` |
| Redis | `/mnt/data/sub2api/redis` | `/data` |
| TokenRouter | `/mnt/data/sub2api/app-data` | `/app/data` |
| OTP Manager | `/mnt/data/otp-manager/data` | `/app/data` |

主服务编排位于 `/opt/sub2api/docker-compose.yml`。OTP 编排位于 `/opt/otp-manager/releases/bugfix-20261113/compose.yaml`。这些数据挂载使用 `create_host_path: false`，目标目录缺失时容器创建失败。

`/opt/sub2api/postgres_data`、`/opt/sub2api/redis_data`、`/opt/sub2api/data` 和 `/opt/otp-manager/data` 是指向上述数据盘目录的符号链接，供已有宿主机脚本继续访问。容器直接挂载数据盘的绝对路径。

`/etc/systemd/system/docker.service.d/20-require-data-disk.conf` 要求 Docker 启动前挂载 `/mnt/data`，并检查挂载点和 UUID。数据盘缺失或替换为其他磁盘时，Docker 启动失败。`/etc/fstab` 的 `nofail` 允许操作系统启动，便于修复挂载问题。

TokenRouter、PostgreSQL 和 Redis 的 Docker 标准输出日志各配置为单文件 50 MB、最多 5 个文件。OTP 使用已有的单文件 10 MB、最多 5 个文件配置。应用文件日志和数据库中的日志表按各自的留存设置维护。

2026 年 11 月 13 日的数据盘迁移沿用运行中的镜像和 354 条数据库迁移记录。停止应用写入后，使用带内容校验的 rsync 同步数据库、Redis、应用文件和 OTP 数据。数据库启动后，在恢复应用写入前比较用户、余额总额、Key、分组、提供商、订单、订阅和迁移版本。用量、日志和审计保留期限保持迁移前的设置。

本次恢复材料位于 `/mnt/data/sub2api/backups/storage-migration-20261113`，包含迁移前后的一致性数据库归档、角色、配置、Redis 和 OTP 恢复材料及校验文件。目录限制为管理员访问。此前文中提及的 `/mnt/data/edu-standard-20261106`、定价及安全构建目录，以及旧的 `/mnt/data/sub2api/backups` 恢复点，已按用户要求清理；恢复时使用本次已验证的材料。

<a id="multi_node_deployment"></a>
## 多节点部署与应用更新

三个应用节点共用 `15.204.66.161` 上的 PostgreSQL 和 Redis，用户、余额、订阅、提供商、调度锁和并发计数使用同一套数据。三个应用使用 `tokenrouter:0.2.10-custom-20261119-a7ef2a4d2`，镜像 ID 为 `sha256:5cc74598dd2e4d517004d715583d1f26909dac5d6873e218ec45084153da22d0`。

三个应用的 `GATEWAY_IMAGE_CONCURRENCY_ENABLED=false`、`GATEWAY_IMAGE_CONCURRENCY_MAX_CONCURRENT_REQUESTS=0`，关闭进程内的生图独立并发限制。用户和提供商并发仍按共享调度规则检查。该图片限制器在启用时按进程计数，多节点上限需要分别配置。

| 节点 | 应用监听 | 编排 |
| --- | --- | --- |
| primary，`15.204.66.161` | `127.0.0.1:8080` | `/opt/sub2api/docker-compose.yml` |
| secondary，`135.148.26.72` | `10.203.76.2:8080` | 该节点的 `/opt/sub2api/compose.yaml` |
| tertiary，`51.81.92.195` | `10.203.76.4:8080` | 该节点的 `/opt/sub2api/compose.yaml` |

主节点 Nginx 的 `api.scienceedu.me` 和 `direct-api.scienceedu.me` 将请求交给 `127.0.0.1:18080` 的 HAProxy。`cn-api.scienceedu.me` 继续经过 `45.8.173.112`，再访问主节点的 HTTPS 入口。HAProxy 按活动连接数分配请求，primary、secondary 和 tertiary 的权重为 2、1、1。权重影响节点分配比例，实际请求数量还受连接持续时间影响。

HAProxy 每三秒检查应用 `/health`，连续两次失败后停止向该节点分配请求，连续两次成功后恢复。入口仅在后端连接尚未建立时改选节点。SSE 和 WebSocket 使用一小时的空闲超时。

`api.scienceedu.me` 的 Nginx 优先从 `/var/www/scienceedu-ui/assets` 提供按内容哈希命名的公共静态资源，未命中时转发到应用池。在镜像切换前运行 `bash deploy/publish-ui-assets.sh backend/internal/web/dist/assets /var/www/scienceedu-ui/assets`，源目录取构建该镜像的产物。脚本串行发布，检查同名资源的内容，并核对发布目录中的 SHA-256。各版本文件按内容哈希共存，供滚动更新中的节点和已打开的页面读取。

三个实例分别配置 `OPS_NODE_ID=primary`、`secondary`、`tertiary`，`OPS_NODE_NAME` 为对应公网 IP，`OPS_CLUSTER_NODE_IDS=primary,secondary,tertiary`。运维概览通过共享 Redis 读取各节点十五秒一次的资源心跳，展示整机容量与应用额度。当前整机合计 8 核 CPU、15379 MiB 内存，应用 CPU 额度合计 7.6 核，内存上限合计 11264 MiB。采样与离线规则见[多节点资源容量](ops_monitoring_and_alerting.md#多节点资源容量)。

资源监控版本的源码和构建产物位于 `/mnt/data/sub2api-build-cache/cluster-monitor-20261118`。这次构建以 Edu 定制镜像为基础，替换主程序并复用证书库。

顶栏标识和用户活跃人数版本的源码、构建产物与验收记录位于 `/mnt/data/sub2api-build-cache/header-activity-20261118` 和 `/mnt/data/sub2api-build-cache/header-activity-verification`。该镜像以资源监控版本为基础替换主程序。用户管理展示五分钟 API 活跃人数，三个节点按用户 ID 合并，每十五秒发布并刷新，统计规则见 [API 活跃用户人数](../domains/identity_and_tenancy.md#api_user_activity)。

### 加密连接与共享数据

WireGuard 接口为 `wg-sci`，主节点地址为 `10.203.76.1`，secondary 地址为 `10.203.76.2`，tertiary 地址为 `10.203.76.4`。节点使用 UDP 51821，主节点防火墙允许 secondary 和 tertiary 的公网地址建立通道。tertiary 使用独立的 `/32` 路由，peer 配置和路由变更可在线加载。

`sub2api-shared-data-proxy` 在 Docker 网络中通过服务名解析 PostgreSQL 和 Redis，并在 WireGuard 地址提供 `10.203.76.1:15432` 和 `10.203.76.1:16379`。secondary 和 tertiary 使用这两个地址访问共享数据。数据容器重新创建后，代理通过 Docker DNS 更新目标地址。公网地址未发布这些监听。

应用负载均衡容器 `sub2api-load-balancer` 使用主机网络，primary 通过本机回环连接应用，secondary 和 tertiary 通过 WireGuard 连接应用。这个布局与各应用的可信代理地址匹配。

主节点服务单元为 `scienceedu-load-balancer.service`，secondary 和 tertiary 分别使用 `scienceedu-node2.service`、`scienceedu-node3.service`。这些服务在 Docker 和 `wg-quick@wg-sci.service` 启动后运行。secondary 和 tertiary 的运行配置位于各自的 `/opt/sub2api/runtime.env`，权限为 `0600`。

Compose 项目名分别为 `sub2api`、`scienceedu-node2`、`scienceedu-node3`。更新容器时用 `--project-name` 指定该节点的项目名；使用目录推导的项目名会创建另一个项目，并与已有的 `sub2api` 容器发生名称冲突。

三个应用的数据目录分别保存日志、审计文件、模型目录和定价快照。文件配置的变更需要同步到各节点，数据库中的业务设置由三个应用共享。

secondary 和 tertiary 的容器资源上限均为 1.8 核 CPU、3 GiB 内存。tertiary 设置 `GOMEMLIMIT=2GiB`，为 Go 运行时之外的内存用量留出空间。PostgreSQL 的连接上限为 100，primary、secondary 和 tertiary 的应用连接池上限分别为 50、25、15；调整节点数量或连接池时，需要同时核对数据库剩余连接数。

### 排空节点后更新

在主节点执行以下命令查看状态：

```bash
sudo scienceedu-lb status
```

更新 primary 前，先停止向它分配新请求：

```bash
sudo scienceedu-lb drain primary
sudo scienceedu-lb status
```

等待该节点的 `scur` 降到 0，再更新它的应用容器。SSE 和 WebSocket 连接会继续占用节点，必要时安排客户端重连。更新完成后检查私有地址的健康状态、版本和日志，再恢复请求接入：

```bash
sudo scienceedu-lb enable primary
```

更新 secondary 或 tertiary 时使用相同流程，将命令参数换成对应名称，应用操作在对应服务器执行。排空状态保存在 HAProxy 运行内存中，负载均衡重新加载配置后按配置恢复健康节点。

HAProxy 使用 master-worker 模式。检查配置通过后，向 `sub2api-load-balancer` 发送 `SIGUSR2` 可平滑加载节点配置，旧 worker 继续处理已有连接。重载期间的统计 socket 展示新 worker 的数据；更新应用前，还需要确认旧 worker 已退出，避免遗漏其在途请求。

应用有五秒的 HTTP 关闭预算，容器的停止等待时间不能代替请求排空。更新使用与其他节点兼容的数据库迁移。PostgreSQL、Redis、HTTPS 入口和负载均衡仍位于 primary 主机；应用容器可以分别更新，主节点整机停机会中断共享服务。

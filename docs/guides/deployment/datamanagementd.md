# datamanagementd 部署说明（数据管理）

> 这是历史资料。TokenRouter 已经停用 data management 守护进程接口，备份和恢复使用内置的 backup 模块。

本文保留已停用的 `datamanagementd` 接入资料，供核对旧部署，或单独维护遗留的守护进程时参考。安装这个进程，当前 TokenRouter 的数据管理接口也不会恢复。

> 当前仓库没有 `datamanagement/` 源码目录，也不发布这个进程的构建产物，所以在本仓库执行 `make build-datamanagementd` 会失败，`install-datamanagementd.sh --source` 也会因为缺少源码目录而失败。需要维护遗留进程时，使用和它历史版本匹配的二进制或完整源码；当前的主服务不会调用它。

## 运行条件

- 仓库保留模板的 Socket 为 `/tmp/tokenrouter-datamanagement.sock`；旧部署可能仍使用 `/tmp/sub2api-datamanagement.sock`。当前的主进程两个路径都不探测。
- 只有 Unix Socket 可连接且健康检查成功时，后台才会启用数据管理。
- `datamanagementd` 使用 SQLite 保存自身元数据，不使用 TokenRouter 的 PostgreSQL 主库。
- 宿主机需要提供 `pg_dump`、`redis-cli`；使用 `source_mode=docker_exec` 时还需要 `docker`。

## 使用现成二进制安装

先确认二进制来源、版本和校验值，再从仓库根目录运行安装脚本：

```bash
sudo ./deploy/install-datamanagementd.sh --binary /absolute/path/to/datamanagementd
```

脚本会把二进制安装到 `/opt/tokenrouter/datamanagementd`，创建数据目录并安装仓库内的 `deploy/tokenrouter-datamanagementd.service`。完成后检查服务和 Socket：

```bash
sudo systemctl status tokenrouter-datamanagementd
sudo journalctl -u tokenrouter-datamanagementd -f
sudo test -S /tmp/tokenrouter-datamanagement.sock
```

若你持有另一个包含 `datamanagement/` 目录的完整源码包，可以使用脚本的 `--source` 模式；该模式不适用于当前仓库。

## Docker 联动

以下挂载示例只适用于仍然支持这个接口的历史版本主服务，当前版本不需要配置。历史部署应先确认守护进程实际创建的 Socket 路径，再将它映射到容器内相同位置：

```yaml
services:
  tokenrouter:
    volumes:
      - /tmp/tokenrouter-datamanagement.sock:/tmp/tokenrouter-datamanagement.sock
```

建议把挂载放在 `docker-compose.override.yml`，避免修改发布 Compose 文件。若 Docker 在 Socket 创建前把宿主机路径建成目录，先停止容器、删除该空目录、启动 `datamanagementd`，再重新创建应用容器。

## 验证

1. 确认 systemd 服务为 `active`，且 Socket 文件存在。
2. 容器部署时，在应用容器内确认同一路径也是 Unix Socket。
3. 打开管理后台“数据管理”，确认代理状态为已启用。
4. 分别执行一个最小 PostgreSQL 和 Redis 备份任务，确认宿主机依赖与文件权限正常。

工程约束和部署资产的现状见 [部署与数据库迁移](../../operations/deployment_and_migrations.md)。

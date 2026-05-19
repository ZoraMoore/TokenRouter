# GitHub Actions 自动部署到 DigitalOcean

本部署链路用于在 `main` 分支更新后自动构建 TokenRouter 镜像，并通过 SSH 更新 DigitalOcean 服务器上的 `/opt/sub2api` 部署。

## GitHub Secrets

在仓库 `Settings -> Secrets and variables -> Actions -> Repository secrets` 中配置：

| Secret | 必填 | 示例 | 说明 |
| --- | --- | --- | --- |
| `DO_HOST` | 是 | `134.209.221.239` | DigitalOcean 服务器公网 IP 或域名 |
| `DO_SSH_PRIVATE_KEY` | 是 | `-----BEGIN OPENSSH PRIVATE KEY-----...` | 可登录服务器的 SSH 私钥内容 |
| `DO_USER` | 否 | `root` | SSH 用户，默认 `root` |
| `DO_PORT` | 否 | `22` | SSH 端口，默认 `22` |
| `DO_DEPLOY_PATH` | 否 | `/opt/sub2api` | 服务器部署目录，默认 `/opt/sub2api` |
| `DO_HEALTHCHECK_URL` | 否 | `http://127.0.0.1:8080/health` | 健康检查地址，默认按 `.env` 的 `SERVER_PORT` 生成 |

## 行为说明

- workflow 文件：`.github/workflows/digitalocean-deploy.yml`
- 服务器更新脚本：`deploy/update-from-ghcr.sh`
- 镜像仓库：`ghcr.io/<GitHub owner>/tokenrouter`
- 每次部署使用本次构建的镜像 digest，而不是只依赖 `latest` 标签。
- 首次部署会在服务器目录不存在 `.env` 时基于 `.env.example` 创建，并自动生成 `POSTGRES_PASSWORD`、`JWT_SECRET`、`TOTP_ENCRYPTION_KEY`。
- 如果服务器已有 `docker-compose.yml`，脚本会保留该文件，只把 `sub2api` 服务的 `image:` 行改成可由 `TOKENROUTER_IMAGE` 覆盖，并先创建 `.bak.<timestamp>` 备份。
- 脚本不会覆盖已有 `.env`、`data/`、`postgres_data/`、`redis_data/`。

## 手动触发

配置 Secrets 后，可以在 GitHub Actions 页面手动运行 `Build Image and Deploy to DigitalOcean`，也可以直接推送到 `main` 自动触发。

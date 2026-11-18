# 云桥词元 API 文档站

这是 `https://docs.scienceedu.me` 的 VitePress 源码，面向云桥词元终端用户，不属于 `docs/` 下的 Project Doc 工程文档库。

## 本地开发

```bash
npm install
npm run docs:dev
```

## 构建

```bash
npm ci
npm run docs:build
```

静态产物位于 `.vitepress/dist/`。生产环境由 Nginx 直接托管该目录的部署副本。

构建时会从主站公开设置接口读取系统配置的 `site_logo`，并把当时的 Logo 快照写入静态产物。系统设置中的 Logo 更新后，需要重新构建和发布文档站才能同步；如果构建时接口不可用，则回退到仓库内的默认图标。

`deploy/nginx-http.conf` 只用于首次 ACME 验证；证书签发后使用 `deploy/nginx.conf`。两个模板都支持 VitePress 的无扩展名 URL。

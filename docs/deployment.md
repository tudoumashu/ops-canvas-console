# 部署说明

本仓库当前主要有两条运行路径：

- Web/API 控制台：Next.js 前端 + Go/Gin 后端 + SQLite/GORM，适合 Docker 或 VPS 部署。
- Local workspace：`opsc serve` 和 `opsc executor --watch` 在用户本机运行，管理本地 canonical files；它不是云端容器的一部分。

## Docker 本地运行

```bash
git clone https://github.com/tudoumashu/ops-canvas-console.git
cd ops-canvas-console
cp .env.example .env
docker compose up -d --build
```

默认访问 `http://localhost:3000`。管理员账号、数据库路径和模型服务配置以 `.env.example` 和当前 compose 文件为准。

## VPS 部署边界

VPS 可部署 Web/API 控制台和现有 PDD 工作流服务。Hybrid ecommerce local path 中，VPS API 只作为执行后端：

- 本地模板、本地 run、本地事件和 canonical artifact/ref 仍保存在 local workspace。
- 浏览器只连接本机 `opsc serve`，不直连 VPS API，不持有 VPS token/cookie。
- `opsc executor --watch` 使用 workspace profile/channel `secretRef` 调 VPS PDD API，并把状态与关键 artifact 导入本地。

不要把 VPS run 目录当作 local workspace canonical source，也不要批量迁移旧 PDD/VPS run。

## Local workspace 常驻

`opsc` 的本地安装、`opsc serve`/`opsc executor --watch` 启动方式和 Linux `systemd --user` 示例见 [opsc 本地安装与自启动](opsc-installation.md)。

## Render

旧的上游 Render 一键部署按钮已不再作为本仓库推荐路径。若要部署到 Render，需要基于当前仓库和当前环境变量重新配置服务、持久化存储和数据库；免费实例文件系统不适合保存正式 SQLite 数据或用户素材。

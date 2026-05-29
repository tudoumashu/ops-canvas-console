# 架构说明

## System Overview

`ops-canvas-console` 是一个单仓库应用：Next.js 提供浏览器 UI 和 `/api/*` 代理，Go/Gin 提供真实业务 API。浏览器本地保存画布项目、我的素材、我的提示词、AI 本地直连配置和登录 token；后端保存用户、算力点、系统设置、公共提示词、服务器素材、工作流模板和工作流运行记录，并读写 PDD workflow 的文件型运行产物。

核心边界：

- Frontend 负责交互、画布状态、浏览器本地数据和调用 API。
- Next.js server 只做代理和页面服务，`/api/[...path]` 代理到 Go API。
- Go API 负责鉴权、数据库、模型渠道代理、PDD run 文件读取、模板工作流执行和本地 agent 任务协调。
- PDD 原始 workflow 仍在外部 `/opt/pdd-workflow` 目录中运行；控制台只读取/展示/触发受控动作或自定义模板 run。

## Frontend Structure

- `web/src/app/(user)/`：用户侧页面，包含首页、登录、图片/视频/综合工作台、素材中心、提示词中心、画布、工作流。
- `web/src/app/(admin)/admin/`：管理后台页面，包含用户、算力点、设置、提示词、素材。
- `web/src/app/(user)/canvas/`：画布相关页面、组件、store、类型和工具函数。
- `web/src/app/(user)/workflows/`：工作流入口、PDD / ecommerce 路由和模板编辑器。
- `web/src/services/api/`：前端 API client；统一经过 `apiGet` / `apiPost` / `apiDelete`。
- `web/src/stores/`：全局 Zustand store，如用户、配置、主题、本地素材、我的提示词。
- `web/src/lib/localforage-storage.ts`：localforage 持久化适配层，失败时 fallback 到 `localStorage`。

## Backend Structure

- `main.go`：加载配置、确保默认管理员、同步 PDD 本地素材、启动提示词同步 scheduler、运行 Gin server。
- `router/router.go`：集中注册 `/api`、`/api/v1`、`/api/admin`、`/api/workflows/pdd`、`/api/local-agent` 路由。
- `handler/`：HTTP 入参、调用 service、返回 `{ code, data, msg }`。
- `service/`：业务逻辑、校验、模型渠道、PDD run 解析、模板工作流执行、local agent、prompt sync。
- `repository/`：GORM 数据库访问和查询。
- `model/`：数据库模型、DTO、枚举、分页查询结构。
- `middleware/`：JWT 鉴权、管理员/用户/可选鉴权和 JSON 404。
- `cmd/local-agent/`：本地脚本 agent CLI。

## Data Model And Storage

后端启动时通过 GORM `AutoMigrate` 管理这些表：

- `users`
- `credit_logs`
- `prompts`
- `assets`
- `settings`
- `workflow_templates`
- `workflow_runs`

本地与文件存储：

- 浏览器：画布项目、我的素材、我的提示词、AI 本地配置、auth token 通过 Zustand persist + localforage 保存。
- SQLite 默认数据库：`data/infinite-canvas.db`。
- 服务器素材：`CONSOLE_ASSETS_ROOT`，默认 `data/assets`。
- Flow2API 视频结果：`VIDEO_STORAGE_ROOT`，默认 `data/video`。
- PDD workflow：`PDD_WORKFLOW_ROOT`、`PDD_RUNS_ROOT`、`PDD_MATERIALS_ROOT`、`PDD_PROMPTS_ROOT`。
- PDD custom workflow 产物：`runs/<run_id>/logs/custom_workflow/` 及传统 `generated/`、`待上架/` 目录。

## API Boundaries

公开/半公开 API：

- `/api/health`
- `/api/auth/*`
- `/api/settings`
- `/api/prompts`
- `/api/assets`
- `/api/assets/pdd-materials/file`
- `/api/assets/local/file`

用户鉴权 API：

- `/api/v1/images/generations`
- `/api/v1/images/edits`
- `/api/v1/chat/completions`
- `/api/v1/videos`
- `/api/v1/videos/:id`
- `/api/v1/videos/:id/content`

管理员 API：

- `/api/admin/*`
- `/api/workflows/pdd/*`

本地 agent API：

- `/api/local-agent/heartbeat`
- `/api/local-agent/jobs/claim`
- `/api/local-agent/jobs/:jobId/complete`

所有业务响应按项目约定返回 `{ code, data, msg }`。OpenAI-compatible 代理接口会转发上游原始 JSON 或适配后的 OpenAI 风格响应。

## Auth And Session Model

- 密码登录：`users.password` 保存 bcrypt hash。
- JWT：`Authorization: Bearer <token>`，claims 包含 `userId`、`username`、`role`，过期时间来自 `JWT_EXPIRE_HOURS`。
- 角色：`guest`、`user`、`admin`。
- `AdminAuth` 要求 `admin`，`UserAuth` 要求非 `guest`，`OptionalAuth` 可选读取用户。
- 默认管理员：首次启动可由 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 创建。
- Linux.do OAuth：由后台系统设置开启并配置 client id/secret。
- local agent token：`LOCAL_AGENT_TOKEN`，不是用户 JWT，留空时 local agent 禁用。

## State Management

- 画布项目：`useCanvasStore`，持久化 key `infinite-canvas:canvas_store`。
- 我的素材：`useAssetStore`，持久化 key `infinite-canvas:asset_store`，图片/视频可进入独立本地 blob 存储。
- 我的提示词：`usePromptStore`，持久化 key `infinite-canvas:prompt_store`。
- AI 配置：`useConfigStore`，持久化 key `infinite-canvas:ai_config_store`。
- 用户会话：`useUserStore`，持久化 key `infinite-canvas-auth-token-v1`。

## Background Jobs Or Queues

- Prompt sync：`service.StartPromptSyncScheduler()` 使用 `robfig/cron`，默认 cron 为 `*/5 * * * *`，同步内置远程 GitHub 提示词源。
- PDD custom workflow：`StartWorkflowTemplateRun` 保存 run 后以 goroutine 执行模板 DAG。
- Log stream：PDD run 日志通过 SSE 每 2 秒 tail 文件。
- Local agent：后端内存 map 保存 pending/running job，本地 CLI 轮询领取并回传结果。
- inferred：当前没有 Redis、Celery、数据库队列表或 durable job queue；服务重启会丢失 local agent 内存任务。

## Deployment Topology

- Docker：`Dockerfile` 使用 Bun 构建 Next.js，Go 1.25-alpine 编译 API，运行镜像中同时启动 Go server 和 Next.js server。
- Standard compose：`docker-compose.yml` 使用 `ghcr.io/basketikun/infinite-canvas:latest`，挂载 `./data:/app/data`，暴露 `3000`。
- Local compose：`docker-compose.local.yml` 本地 build 同一 Dockerfile。
- PDD console compose：`docker-compose.pdd-console.yml` 使用 host network、host pid、privileged，挂载 `/opt/pdd-workflow` 和 `/opt/pdd-venv`，前端绑定 `127.0.0.1:13000`。
- Render：`render.yaml` 定义 Docker Web Service，免费计划，health check `/api/health`。
- GitHub Actions：`.github/workflows/docker-image.yml` 在 tag `v*` 或手动触发时 build/push GHCR image。

## Important Directories

- `web/src/app/`：Next.js routes。
- `web/src/components/`：全局组件和布局组件。
- `web/src/services/`：API、图片/文件本地存储。
- `web/src/stores/`：全局 Zustand store。
- `handler/`、`service/`、`repository/`、`model/`：Go 后端分层。
- `docs/`：项目功能、部署、数据结构和待测试文档。
- `docs/ai/`：AI 项目记忆。

## Diagrams

- `docs/ai/diagrams/system-architecture.mmd`
- `docs/ai/diagrams/request-flow.mmd`
- `docs/ai/diagrams/data-flow.mmd`
- `docs/ai/diagrams/deployment-flow.mmd`
- `docs/ai/diagrams/user-flow-canvas-generation.mmd`
- `docs/ai/diagrams/user-flow-pdd-workflow.mmd`

## Unknowns And Inferred Facts

- unknown：当前生产环境是否使用 SQLite、PostgreSQL 或 MySQL。
- unknown：当前是否已有真实 Render 部署或只使用 VPS Docker 部署。
- unknown：PDD VPS 当前运行状态、域名、隧道、账号、模型渠道和密钥不可从仓库证明。
- inferred：仓库的实际项目名是 `ops-canvas-console`，产品 UI/README 仍保留 `infinite-canvas` 品牌与上游文档。
- inferred：当前主要目标用户是单人/小团队本地或 VPS 控制台使用，不是公网多人 SaaS。


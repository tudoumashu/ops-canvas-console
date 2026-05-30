# 架构说明

## System Overview

`ops-canvas-console` 是一个单仓库应用：Next.js 提供浏览器 UI 和 `/api/*` 代理，Go/Gin 提供真实业务 API。自用 local-first 路径中，画布项目、画布项目媒体、画布导入/导出 zip 媒体、我的素材、我的提示词、工作台 text/image/video 生成记录、AI 本地 profile、本地项目引用、电商工作流私有模板、工作流入口自定义文件夹和本地 run/artifact 基础记录已通过 `opsc serve` 写入本地 workspace；本地项目面板列表不展示 `rootPath`，编辑时才读取路径，含 `credentialRef` 的项目不在 Web UI 写回；本地电商模板编辑器里的 `material_lookup` 固定素材选择读取当前 workspace 图片素材，启动本地 run 时固定素材会复制成 canonical artifact 并通过 run artifact ref 关联。`opsc executor` 现在可在 local workspace 内领取 run，并在 project capability/path guard 下执行最小 `condition`/`script`/output mapping 链路。浏览器只保留展示缓存、local workspace loopback `baseUrl` 或即时预览临时状态，启动时会清理旧浏览器私有事实源 key，素材媒体、画布媒体和工作台参考图保存到 workspace 后会清理已 canonical 化且当前状态不再引用的上传桥接 blob，AI API key 由 `opsc serve` 根据 profile `secretRef` 解析。后端保存用户、算力点、系统设置、公共提示词、服务器素材、服务器工作流模板和工作流运行记录，并读写 PDD workflow 的文件型运行产物。

核心边界：

- Frontend 负责交互、画布运行态、浏览器临时/cache 状态和调用 API。
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

- 浏览器：画布项目、我的素材、我的提示词只保留 `opsc:*_cache:v1` 展示/临时缓存，其中素材/提示词 cache 不再持久化私有列表或正文；本地工作区连接 store 只保留 loopback `baseUrl`，旧版 `workspace/runtime` 持久化字段会通过版本迁移丢弃；本地项目引用通过 `opsc serve` 写入 `projects/`，浏览器不持久化 `rootPath` 或 credential refs；启动时会清理旧 `infinite-canvas:ai_config_store`、`infinite-canvas:asset_store`、`infinite-canvas:prompt_store`、`infinite-canvas:canvas_store`、旧 generation log key 和旧 `ops-canvas-workflow-folders`；素材包导入不再恢复到浏览器 `image_files/media_files`，而是通过 `opsc serve` 写入 workspace；素材新增/更新/删除成功后会清理当前素材和画布都不再引用的 browser media blob；画布媒体保存成功后会清理已写入 workspace 且当前状态不再引用的 browser media blob；画布 zip 导入只临时使用浏览器 Blob 缓存桥接文件上传，导入完成后清理临时 import blob；工作台生成记录不再使用旧 generation log key 作为事实源，图片/视频生成结果和参考图保存成功后会切换为 `opsc serve` 文件端点回显并清理已保存的 reference blob；本地 run 列表/状态/artifact 预览从 `opsc serve` 读取；AI 本地配置不再持久化 API key，配置事实源是 local workspace profile；auth token 仍由现有登录状态保存。
- SQLite 默认数据库：`data/infinite-canvas.db`。
- 服务器素材：`CONSOLE_ASSETS_ROOT`，默认 `data/assets`。
- Flow2API 视频结果：`VIDEO_STORAGE_ROOT`，默认 `data/video`。
- PDD workflow：`PDD_WORKFLOW_ROOT`、`PDD_RUNS_ROOT`、`PDD_MATERIALS_ROOT`、`PDD_PROMPTS_ROOT`。
- PDD custom workflow 产物：`runs/<run_id>/logs/custom_workflow/` 及传统 `generated/`、`待上架/` 目录。

accepted local boundary：Phase 0 已接受并补全 `docs/local-workspace-v1-contract.md`，后续自用 local-first 数据底座以 `~/OpsCanvas` 为默认 workspace。私有模板、run、artifact、个人素材、个人 prompt、画布项目、工作台生成记录、本地项目路径和本地日志默认进入 local workspace；账号、授权、计费、官方/公共模板、公共素材和商用 profile 中转能力留在云端。Phase 6 已新增 `internal/localworkspace` foundation、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log file-backed repository、run artifact ref、run node state、run events、project root salted fingerprint、project path capability guard、`index.sqlite` 增量更新/扫描重建、默认 export plan 排除规则、GC dry-run plan，以及 `cmd/opsc` 的 `workspace init/info/doctor/index rebuild/export plan/gc plan`、`template list`、`run list/status/events`、`artifact list`、`profile list`、`project list`、`asset list`、`prompt list`、`serve` 和 `mcp`。Phase 7 已先把 Web UI 的 `我的素材`、`我的提示词`、画布项目库、画布项目内媒体文件、画布导入/导出 zip 媒体文件、工作台 text/image/video 生成记录、AI 本地 profile/proxy、本地项目引用、电商工作流私有模板 CRUD、模板 `material_lookup` 固定素材本地选择、工作流入口自定义文件夹、本地 run 列表/状态页/artifact 预览、本地模板创建 run 草稿和固定素材 artifact ref 接到 `opsc serve`，图片/视频工作台结果保存成功后也从 workspace 文件端点回显；`opsc mcp` 已提供 stdio 薄封装，读取/诊断/dry-run 工具复用现有 CLI JSON 命令，唯一维护写工具 `opsc_workspace_index_rebuild` 通过 active `opsc serve` loopback API 重建派生索引；Phase 8 已补充 `opsc serve` auth/redaction/session、AI proxy `secretRef` header 隔离、本地模板草稿 run 到 canonical artifact ref 和 MCP stdio smoke 的自动化回归；Phase 10 已在 `opsc executor` 本地 run-once MVP 上加入 project-aware `condition`、`script`、节点 retry、conditional edge skip、project output mapping、capability/path guard 和 artifact.write 校验；真实 PDD/VPS executor、DB 和 PDD/VPS 数据尚未迁移。

local workspace v1 的事实源是带 `schemaVersion/kind/id/revision/data` envelope 的 JSON、append-only JSONL event 和文件目录；`index.sqlite` 只做派生索引，可通过扫描 workspace 重建。当前 foundation 提供路径解析、manifest/envelope、ULID、`secretRef` 结构校验与脱敏 summary、atomic JSON/文件写入、lock、doctor report、runtime metadata 读取、泛型 JSON object repository、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log typed repository、run artifact ref、run node state、run events JSONL/follow、workspace scan、sqlite index rebuild、export plan 和 GC dry-run plan。project root fingerprint 使用 `.opsc/project-root.salt` 计算 opaque `rootfp_*`，该 salt 和本地 project document 默认排除 export。CLI `opsc` 是核心接口；`opsc serve` 已实现本机 loopback API，默认 `127.0.0.1:17680`，支持 `--port 0`、workspace 外 XDG state runtime、HTTP bearer token、browser 一次性 launch secret + HttpOnly session、CORS local origin 白名单、`{code,data,msg}` HTTP envelope，并提供 workspace 查询、workspace preferences 查询/更新、doctor、index rebuild、export/gc plan、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log summaries、local template create/get/update/delete、run create/get/update、run event append/SSE、run node state、run artifact ref、artifact create/update/import、profiles/projects/assets/prompts/canvas-projects create/update/delete、asset multipart import、canvas-project multipart media import、workbench-log multipart media import、artifact/asset/canvas-project/workbench-log 文件读取、prompt content 读取和 `/api/local/ai/v1/*` 本地 AI proxy。`opsc executor` 是唯一正式本地 workflow executor 入口，run-once 领取 `run.waiting_for_executor` 的 pending run 或带 executor metadata 的 running run，执行 `input`、`text_static`、固定本地素材 `material_lookup`、`text_generation`、`image_generation`、`condition` 和受 project capability/path guard 约束的 `script`，复用 local profile `secretRef` 调用 OpenAI-compatible provider，并写回 run/node 状态、events、global artifact、run artifact ref 和相对 project output metadata。`opsc mcp` 已实现 stdio MCP server，当前读取/诊断/dry-run 工具把 `tools/call` 映射到既有 CLI JSON 命令，覆盖 workspace info/doctor/export plan/gc plan、template/run/artifact/profile/project/asset/prompt 列表和 run status/events；`opsc_workspace_index_rebuild` 通过 active `opsc serve` bearer API 调 `/api/local/workspace/index/rebuild`，只重建派生 `index.sqlite`；MCP 不暴露 canonical object 写入工具、不直接操作 workspace repository、不启动 executor。Web UI 通过 `web/src/services/local-workspace.ts`、`web/src/services/local-workflow-templates.ts`、`use-local-workspace-store` 和 `use-config-store` 连接本地服务；当前迁移素材、提示词、画布项目、画布媒体、画布导入/导出 zip 媒体、工作台生成记录、AI profile/proxy、本地项目引用、电商工作流私有模板 CRUD、工作流入口自定义文件夹、本地 run/artifact 基础 UI、固定素材 artifact ref 和 local run `profileId/projectId` 传递；真实 PDD/VPS executor 和 PDD/VPS run 数据迁移尚未接入。

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

- 画布项目：`useCanvasStore`，canonical data 通过 `opsc serve` 写入 `canvas-projects/<canvas_id>/canvas-project.json`，图片/视频节点和助手媒体写入 `canvas-projects/<canvas_id>/files/` 并以 `workspaceFileKey` 引用；保存、导入或删除成功后会清理已写入 workspace 且当前状态不再引用的 `image:*`、`video:*`、`file:*` browser blob；浏览器展示缓存 key `opsc:canvas_store_cache:v1`，不再持久化项目列表，旧 `infinite-canvas:canvas_store` 不迁移。
- 我的素材：`useAssetStore`，canonical data 通过 `opsc serve` 写入 `assets/`；store 使用 `loadedWorkspaceId/workspaceLoaded` 只展示当前 workspace 的内存数据，持久化 key `opsc:asset_store_cache:v1` 不写私有列表，旧 `infinite-canvas:asset_store` 不迁移；素材 zip 导出从当前 workspace 回显 URL 读取文件，素材 zip 导入通过 `addAsset` 写回 workspace；素材新增、更新或删除成功后会清理当前素材列表和画布项目都不再引用的 browser media blob。
- 我的提示词：`usePromptStore`，canonical data 通过 `opsc serve` 写入 `prompts/`；store 使用 `loadedWorkspaceId/workspaceLoaded` 只展示当前 workspace 的内存数据，持久化 key `opsc:prompt_store_cache:v1` 不写提示词正文，旧 `infinite-canvas:prompt_store` 不迁移。
- 工作台生成记录：text/image/video 工作台通过 `workbench-log-storage.ts` 调 `opsc serve` 写入 `workbench-logs/<wblog_id>/workbench-log.json` 和 `files/`；图片/视频生成结果和参考图保存成功后会把当前卡片/参考图替换为 workspace 文件 URL，并清理已 canonical 化的 `image:*` browser blob；新会话、移除参考图和切换历史记录也会清理当前参考图临时缓存；旧 `text_generation_logs`、`image_generation_logs`、`video_generation_logs` 不迁移。
- 本地项目引用：顶部本地工作区弹窗内的 `LocalProjectsPanel` 通过 `opsc serve` 读写 `projects/<proj_id>/project.json`；列表只展示 project id、kind、adapter、capabilities 和 opaque `rootFingerprint`，不展示 `rootPath`；编辑时才调用 `GET /api/local/projects/<id>?showPaths=1`，含 `credentialRef` 的项目只读提示用 CLI 修改，避免 Web UI 写回脱敏 summary。
- 工作流入口自定义文件夹：`/workflows` 通过 `GET/PUT /api/local/workspace/preferences` 读写 `opsc-workspace.json.data.preferences.workflowFolders`；旧 `ops-canvas-workflow-folders` localStorage key 不再作为事实源，也不迁移。
- 电商工作流私有模板与本地 run：连接本地工作区时，模板列表和模板编辑器通过 `local-workflow-templates.ts` 调 `opsc serve` 写入 `templates/<tpl_id>/template.json`；`material_lookup` 固定素材下拉读取当前 workspace 的 `我的素材` 图片列表，不再请求服务器 admin asset 接口；运行本地模板会创建 `runs/<run_id>/run.json`、节点状态和 `run.waiting_for_executor` 事件，并进入本地 run 状态页。固定 `material_lookup` 图片素材会从 `assets/<asset_id>/files/original` 复制成全局 `artifacts/<art_id>/files/original`，run 内只保存 artifact ref；如果 Web 创建 run 时已预写固定素材节点，`opsc executor` 会跳过已成功节点。Phase 9 executor 已支持固定素材、文本生成和图片生成；自动匹配 local asset、project adapter、PDD/VPS executor 迁移和其它节点类型仍待下一阶段处理。
- AI 本地配置：`use-config-store` 从 local profile 读取/保存 Base URL、模型列表、默认模型和 env `secretRef`；连接或刷新 local workspace 时会按当前 workspace 重新加载 profile，无 profile 或断开连接时清空内存 profile 配置，避免跨 workspace 串用；浏览器启动时清理旧 `infinite-canvas:ai_config_store`，本地模型请求改走 `opsc serve` 的 `/api/local/ai/v1/*`，浏览器不保存或发送 API key。
- 本地工作区连接：`use-local-workspace-store` 只持久化 `baseUrl`，连接态、workspace info 和 runtime metadata 都来自当前会话的 `/api/local/workspace` 查询；persist migration 会清理旧版 store 中的 `workspace/runtime/tokenFile/launchSecretFile` 字段；Web UI 会先用免鉴权 `/api/health` 区分 `opsc serve` 未启动和已启动但 browser session 未建立/过期，in-scope 本地私有页面通过统一状态提示阻断继续把浏览器缓存当事实源。
- legacy browser state：`ClientRootInit` 启动时调用 `clearLegacyPrivateBrowserState`，清理旧 AI config、旧素材/提示词/画布 store、旧 text/image/video generation logs 和旧 workflow folders；不清理当前 `opsc:*_cache:v1` 或 `image_files/media_files` 临时媒体桥接库。
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
- `docs/ai/diagrams/local-workspace-v1.mmd`
- `docs/ai/diagrams/user-flow-canvas-generation.mmd`
- `docs/ai/diagrams/user-flow-pdd-workflow.mmd`

## Unknowns And Inferred Facts

- unknown：当前生产环境是否使用 SQLite、PostgreSQL 或 MySQL。
- unknown：当前是否已有真实 Render 部署或只使用 VPS Docker 部署。
- unknown：PDD VPS 当前运行状态、域名、隧道、账号、模型渠道和密钥不可从仓库证明。
- inferred：仓库的实际项目名是 `ops-canvas-console`，产品 UI/README 仍保留 `infinite-canvas` 品牌与上游文档。
- inferred：当前主要目标用户是单人/小团队本地或 VPS 控制台使用，不是公网多人 SaaS。

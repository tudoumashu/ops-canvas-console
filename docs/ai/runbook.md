# Runbook

## Runtime Versions

- Go：`go.mod` 声明 `go 1.25.0`。
- Backend framework：Gin `v1.11.0`、GORM `v1.31.1`。
- Frontend：Next.js `16.2.3`、React `19.2.5`、TypeScript `^5`。
- UI：Ant Design `^6.4.2`、Tailwind CSS `^4`。
- Package manager：`web/bun.lock` 和 Dockerfile 使用 Bun；Docker runtime 使用 `oven/bun:1.3.13`。

## Install

```bash
go mod download
cd web
bun install --frozen-lockfile
```

## Required Env Vars

只列变量名，不记录密钥值：

```text
ADMIN_USERNAME
ADMIN_PASSWORD
JWT_SECRET
JWT_EXPIRE_HOURS
PORT
API_BASE_URL
STORAGE_DRIVER
DATABASE_DSN
LINUX_DO_AUTHORIZE_URL
LINUX_DO_TOKEN_URL
LINUX_DO_USERINFO_URL
CONSOLE_ASSETS_ROOT
VIDEO_STORAGE_ROOT
PDD_WORKFLOW_ROOT
PDD_RUNS_ROOT
PDD_MATERIALS_ROOT
PDD_PROMPTS_ROOT
PDD_PYTHON
PDD_WORKFLOW_CONFIG
PDD_WORKFLOW_ENV_FILE
PDD_ACTION_NSENTER
PDD_ACTION_AUDIT_LOG
PDD_CONSOLE_READ_ONLY
PDD_ACTION_TIMEOUT_SECONDS
LOCAL_AGENT_TOKEN
```

## Local Services

- Backend API 默认监听 `:8080`。
- Frontend dev server 默认监听 `0.0.0.0:3000`。
- 默认数据库是 SQLite：`data/infinite-canvas.db`。
- Docker 默认挂载 `./data:/app/data`。
- PDD console 需要 VPS 或本机存在 `/opt/pdd-workflow` 和可选 `/opt/pdd-venv`。

## Database Migrations And Seeds

- 启动时 `repository.DB()` 调用 GORM `AutoMigrate`。
- 当前迁移表：`users`、`credit_logs`、`prompts`、`assets`、`settings`、`workflow_templates`、`workflow_runs`。
- 首次启动可由 `service.EnsureDefaultAdmin()` 根据 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 创建默认管理员。
- 没有独立 migration CLI；项目约定尚未上线，不需要旧数据兼容，表结构调整直接按新设计修改。

## Dev Server Commands

源码方式：

```bash
cp .env.example .env
go run .
```

```bash
cd web
API_BASE_URL=http://127.0.0.1:8080 bun run dev
```

Docker 本地构建：

```bash
cp .env.example .env
docker compose -f docker-compose.local.yml up -d --build
```

PDD console：

```bash
cp .env.example .env
docker compose -f docker-compose.pdd-console.yml up -d --build
```

PDD console 建议通过 SSH 隧道访问：

```bash
ssh -p 443 -L 13000:127.0.0.1:13000 root@<vps-host>
```

Local agent：

```bash
go run ./cmd/local-agent --server http://127.0.0.1:13000 --token "$LOCAL_AGENT_TOKEN" --root /path/to/local/repo
```

## Test / Typecheck / Build Commands

仓库可用命令：

```bash
go test ./...
```

```bash
cd web
bun run format:check
bun run build
```

注意：当前项目 `AGENTS.md` 写明“每次写完代码，不需要检查语法，不需要执行构建，用户会自己做”。对普通业务修改按用户当前指令优先；项目记忆初始化只运行轻量文档/Wiki 验证。

## Deploy

普通 Docker：

```bash
docker compose up -d
```

Render：

- `render.yaml` 定义 Docker Web Service。
- 需要在 Render UI 填写 `ADMIN_PASSWORD`。
- 免费版本地文件非持久，SQLite 数据可能随重启/重部署丢失。

GHCR：

- `.github/workflows/docker-image.yml` 在 tag `v*` 或 `workflow_dispatch` 构建并推送 Docker image。

PDD VPS console：

```bash
docker compose -f docker-compose.pdd-console.yml up -d --build
```

该 compose 使用 `network_mode: host`、`pid: host`、`privileged: true`。只有在明确需要读取 `/opt/pdd-workflow`、执行 allowlist VPS 动作或通过 `nsenter` 操作宿主机时才使用。

## Rollback

- Docker compose：回退 image tag 或回退代码后重新 `docker compose up -d --build`。
- GHCR：使用旧 tag 对应 image。
- Database：unknown，仓库没有独立迁移/回滚脚本。
- PDD run：运行产物是文件型目录，回滚应用不会自动回滚已写入的 `runs/<run_id>`。

## Common Failures And Fixes

- `/api/*` 返回“接口连接失败”：检查 Go API 是否启动，以及 Next.js `API_BASE_URL` 是否指向正确后端。
- 默认管理员无法登录：确认首次启动时 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 已设置；已有管理员时不会重复创建。
- SQLite 数据丢失：确认 Docker 是否挂载 `./data:/app/data`；Render 免费实例文件不持久。
- 模型调用失败：检查后台 `private.value.channels` 的 `baseUrl`、`apiKey`、`models`、`enabled` 和 `weight`；密钥不应写入文档。
- 生成扣费但上游失败：后端设计会在失败回调中返还算力点；仍需检查 `credit_logs` 是否出现 `ai_refund`。
- Flow2API 视频重启后无法访问：确认 `VIDEO_STORAGE_ROOT` 位于持久化 `data/` 卷。
- PDD console 读不到 run：确认 `PDD_RUNS_ROOT` 和 Docker volume 映射，run id 必须匹配 `^[A-Za-z0-9_.-]+$`。
- PDD allowlist 动作失败：确认 `PDD_CONSOLE_READ_ONLY=false`、`PDD_ACTION_NSENTER=true`、容器 privileged/host pid/network，以及宿主机命令存在。
- Local agent 无任务或报 token 错：确认服务端和本地 CLI 使用同一个 `LOCAL_AGENT_TOKEN`，且脚本路径在 `--root` 内。


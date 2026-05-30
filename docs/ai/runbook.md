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

Local workspace foundation：

```bash
go run ./cmd/opsc workspace init --workspace ~/OpsCanvas --json
go run ./cmd/opsc workspace info --workspace ~/OpsCanvas --json
go run ./cmd/opsc workspace doctor --workspace ~/OpsCanvas --json
go run ./cmd/opsc workspace index rebuild --workspace ~/OpsCanvas --json
go run ./cmd/opsc workspace export plan --workspace ~/OpsCanvas --json
go run ./cmd/opsc workspace gc plan --workspace ~/OpsCanvas --json
go run ./cmd/opsc profile list --workspace ~/OpsCanvas --json
go run ./cmd/opsc project list --workspace ~/OpsCanvas --json
go run ./cmd/opsc template list --workspace ~/OpsCanvas --json
go run ./cmd/opsc run list --workspace ~/OpsCanvas --json
go run ./cmd/opsc run status <run_id> --workspace ~/OpsCanvas --json
go run ./cmd/opsc run events <run_id> --workspace ~/OpsCanvas
go run ./cmd/opsc run events <run_id> --workspace ~/OpsCanvas --follow
go run ./cmd/opsc artifact list --workspace ~/OpsCanvas --json
go run ./cmd/opsc artifact list --run <run_id> --workspace ~/OpsCanvas --json
go run ./cmd/opsc asset list --workspace ~/OpsCanvas --json
go run ./cmd/opsc prompt list --workspace ~/OpsCanvas --json
go run ./cmd/opsc serve --workspace ~/OpsCanvas --port 17680 --origin http://127.0.0.1:3000
go run ./cmd/opsc mcp --workspace ~/OpsCanvas
```

默认 workspace 解析顺序是 `--workspace`、`OPSC_WORKSPACE`、`~/OpsCanvas`。默认 JSON 输出不包含 workspace 绝对路径；需要路径时显式加 `--show-paths`。`workspace doctor` 的人类诊断输出到 stderr，`--json` 模式下 stdout 才输出机器可读 report。`workspace export plan` 只输出相对路径和排除原因；`workspace gc plan` 只做 dry-run，输出相对路径 candidates，动作固定为 `review`，不会删除文件；`project list` 不输出 `rootPath`，只输出 opaque `rootFingerprint`；`profile list` 不输出 secret 值。`run events` 输出 JSONL 事件流，每行一个 event envelope，不使用 `{ ok, data, warnings }` 包装；`--follow` 会持续轮询追加事件直到调用方中断。

`opsc serve` 默认只监听 `127.0.0.1:17680`，`--port 0` 可使用系统空闲端口，`--origin` 只接受明确的本地浏览器 origin，不接受 `*`。启动后 runtime/state 文件位于 `$XDG_STATE_HOME/opsc/workspaces/<workspaceId>-<rootHash>/`，无 XDG 时 fallback 到 `~/.local/state/opsc/workspaces/<workspaceId>-<rootHash>/`；state 目录权限应为 0700，`bearer.token`、`launch.secret` 和 `sessions.json` 权限应为 0600。CLI 或显式 HTTP client 可用 `Authorization: Bearer <bearer.token>`，browser 应用 `launch.secret` 调 `POST /api/local/bootstrap/session` 换 HttpOnly session；带 `Origin` 的请求不接受 bearer。`GET /health` 和 `GET /api/health` 免鉴权且只返回 `ok`；其它 `/api/local/*` 需要 session 或 bearer。`serve.json`、`workspace info` 和 `--json` 启动输出不得包含 token、launch secret、session id 或 workspace 绝对路径。

`opsc mcp` 是本地 agent 的 stdio MCP server。开发期可直接用 `go run ./cmd/opsc mcp --workspace ~/OpsCanvas`，实际配置 Codex / Claude Code 等 MCP client 时建议先构建 `opsc` 二进制，再把 command 指向该二进制并传 `["mcp", "--workspace", "/home/<user>/OpsCanvas"]`。首版工具只做 workspace 查询、doctor、index rebuild、export/GC dry-run、template/run/artifact/profile/project/asset/prompt 列表和 run status/events；不暴露 canonical object 写入工具、不暴露 `run events --follow`，默认不输出 secrets、workspace 绝对路径或 project `rootPath`。`opsc_workspace_info` 默认只保留 `runtime.active`，不输出 `runtime.baseUrl`、`runtime.host`、`runtime.port` 或可重建本地 serve URL 的字段。`opsc_workspace_index_rebuild` 是唯一维护写工具，调用前必须先启动同一 workspace 的 `opsc serve`；该工具只从 runtime state 读取相对 `bearer.token` 并调用 loopback `/api/local/workspace/index/rebuild`，不会把 token、token 文件路径或 serve URL 写入 MCP 输出。

`opsc executor` 是当前唯一正式本地 workflow executor 入口。开发期可直接用 `go run ./cmd/opsc executor --workspace ~/OpsCanvas`，也可加 `--run <run_id>` 限定单个 run。它只处理带 `run.waiting_for_executor` 的 pending run 或已由 executor 接管的 running run；支持固定本地素材 `material_lookup`、`text_generation`、`image_generation` 和最小 `input/text_static` 辅助节点。模型调用通过 workspace profile 的 `secretRef` 解析 env/file secret，不从浏览器读取 API key，也不迁移 PDD/VPS run。

Web UI 本地工作区连接：

1. 启动 `go run ./cmd/opsc serve --workspace ~/OpsCanvas --port 17680 --origin <当前 Web origin>`。
2. 在顶部导航点击本地工作区按钮，服务地址使用与 Web origin 同一 loopback host，例如 Web 是 `http://localhost:3000` 时优先填 `http://localhost:17680`，避免 `SameSite=Lax` cookie 因 `localhost` / `127.0.0.1` 混用不发送。
3. 从 runtime/state 目录读取本次启动的 `launch.secret`，输入后建立 browser session。不要把 `bearer.token` 填到浏览器或写进 `localStorage`。
4. `我的素材`、`我的提示词`、画布项目库、工作台 text/image/video 生成记录、电商工作流私有模板、local run/artifact 基础记录和工作流入口自定义文件夹通过 `opsc serve` 读写 workspace；图片/视频工作台结果保存成功后从 workbench-log 文件端点回显；浏览器 localforage/localStorage 只保留 `opsc:asset_store_cache:v1`、`opsc:prompt_store_cache:v1`、`opsc:canvas_store_cache:v1` 展示缓存、loopback `baseUrl` 或生成中/上传中的临时媒体状态。旧 `infinite-canvas:*`、`text_generation_logs`、`image_generation_logs`、`video_generation_logs`、`ops-canvas-workflow-folders` 测试数据不迁移。本地模板当前可创建 local run，另开终端执行 `go run ./cmd/opsc executor --workspace ~/OpsCanvas` 可领取并执行固定本地素材、文本生成和图片生成最小节点集；现有 PDD/VPS run 仍不迁移。

## Test / Typecheck / Build Commands

仓库可用命令：

```bash
go test ./...
```

```bash
go test ./internal/localworkspace ./cmd/opsc
```

Phase 8 local workspace 稳定化目标验证：

```bash
docker run --rm -e GOPROXY=https://goproxy.cn,direct -v "$PWD":/src -w /src golang:1.25-alpine /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc
```

该目标测试覆盖 `opsc serve` 鉴权/redaction/session、CLI `serve` 输出脱敏、AI proxy `secretRef` 与浏览器 header 隔离、本地模板草稿 run 到 canonical artifact ref happy path，以及 `cmd/opsc` MCP stdio wrapper smoke、工具面冻结、`workspace_info` active runtime URL/host/port 脱敏、doctor/export plan/GC dry-run/run events/index rebuild。Go 文件改动后可用 Docker 执行 `gofmt`，避免本机未安装 Go：

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine /usr/local/go/bin/gofmt -w internal/localworkspace cmd/opsc
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
- Local workspace GC 显示候选项：先检查 `workspace gc plan --json` 的 `reason` 和 `referencedBy`，当前命令不会删除文件；如怀疑 index 漂移，先运行 `workspace index rebuild --json`，再重新查看 GC plan。

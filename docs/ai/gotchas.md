# Gotchas

## 当前工作区不是干净基线

项目记忆初始化时 `git status --short` 显示大量已修改和未跟踪业务文件。本次项目记忆只记录当前工作区事实，不代表这些改动已经提交、发布或测试通过。

## 浏览器本地数据不是云同步

用户本地 AI 配置不应保存在浏览器 localforage/IndexedDB 或 `localStorage`。登录账号后不表示这些本地业务数据自动同步到服务器。当前画布项目、“我的素材”、“我的提示词”、工作台 text/image/video 生成记录、AI 本地 profile、本地项目引用、电商工作流私有模板、本地 run/artifact 基础记录和工作流入口自定义文件夹已先接 `opsc serve`，浏览器里只保留 `opsc:*_cache:v1` 展示缓存、loopback `baseUrl` 或生成中/上传中的临时媒体状态；旧 `infinite-canvas:*`、`text_generation_logs`、`image_generation_logs`、`video_generation_logs`、`ops-canvas-workflow-folders` 测试数据不会自动迁移，并会在 Web UI 启动时从 localforage `app_state` 和 `localStorage` 清理已知 legacy private keys。不要把这个清理扩展到 `image_files/media_files`，它们仍承担上传中、生成中或保存失败重试前的临时媒体桥接。

`opsc:local_workspace_connection` 是连接提示缓存，不是 workspace fact。浏览器最多保存 loopback `baseUrl`；旧版本如果写入过 `workspace/runtime/tokenFile/launchSecretFile`，必须通过 store version migration 丢弃，不能把这些字段当成可复用的 session 或路径信息。

AI local profile 是 workspace-scoped 会话视图，不是全局浏览器配置。切换到没有 profile 的 workspace、刷新失败或断开 local workspace 时必须清空内存中的 Base URL、模型列表、默认模型和 `SecretRef`，否则会把上一个 workspace 的私有配置串到新 workspace。

## Local Workspace 只部分接入 Web UI

`docs/local-workspace-v1-contract.md` 是 Phase 0 已接受并补全的设计 contract。Phase 13 当前已实现 `internal/localworkspace` foundation、`cmd/opsc workspace init/info/doctor/index rebuild/export plan/gc plan`、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log repository、`opsc serve` 本机 loopback API、`opsc mcp` stdio 查询/诊断薄封装和 serve-backed index rebuild 工具、Web UI local workspace adapters、`opsc executor` run-once/watch worker、project-aware `condition`/`script`、以及单条已确认电商模板的 hybrid VPS backend。服务端 DB 和现有 PDD/VPS 文件目录仍不是 local workspace canonical data；自动 `material_lookup` 本地素材匹配、`image_edit`/`video_generation`、复杂 loop/guardrail、完整失败恢复和 MCP 执行工具仍未实现。不要把局部接入误读成全站 Web UI 本地化、MCP canonical object 写入/执行工具或旧数据迁移已经完成。

Run events 的 actor type 有白名单。测试或手工 seed local run 时不要随手写 `manual` 这类新 actor type；使用现有允许值，例如 `cli`、`web` 或 `system`，否则 `AppendRunEvent` 会返回 `event actor type is not allowed`。

本地项目引用的 Web UI 是轻量管理入口，不是完整 project adapter。列表不得展示本机 `rootPath`，只有用户点击编辑时才通过 `showPaths=1` 读取路径；如果项目包含 `credentialRef`，Web UI 只拿到脱敏 summary，不能保存覆盖真实 secretRef，应该提示用户用 CLI 修改。

`workspace doctor` 当前只做结构、引用和占位符级检查：能发现 schema/目录/manifest/lock/index 文件、index 可能过期、executor worker active/stale/not running、hybrid run 等待/卡住、broken refs、`secretRef` 明文字段、project root 可访问性/fingerprint、project execution policy、asset file、prompt content、canvas-project media file、workbench-log media file、export rule 和 GC dry-run candidates。它不会解析真实 secret、不会访问模型供应商，也不会替用户执行 Full GC；若提示 `index.sqlite may be stale`，先运行 `opsc workspace index rebuild --json`。

`workspace gc plan` 是 dry-run，只返回相对路径 candidates，动作固定为 `review`，不会删除 artifact、asset file、prompt content、canvas-project media file、workbench-log media file 或 run refs。后续如果实现删除命令，必须先确认引用检查和回滚/备份策略。

## `opsc serve` Token 不是 `LOCAL_AGENT_TOKEN`

当前 `cmd/local-agent` 使用 `LOCAL_AGENT_TOKEN` 连接 VPS 控制台领取脚本任务。`opsc serve` 的 `bearer.token` 存在 workspace 外 XDG state 目录，只用于显式 HTTP client 访问 local workspace；browser 只能用一次性 `launch.secret` 换 HttpOnly session，带 `Origin` 的请求不接受 bearer。`opsc serve` 普通启动日志最多打印 `bearer.token` / `launch.secret` 文件名，不应打印 workspace 绝对路径、state dir、token 文件真实路径或 token 内容。`opsc mcp` 的查询/诊断/dry-run 工具仍包装同进程 CLI JSON 命令，但 `opsc_workspace_info` 会做 MCP 专用脱敏，默认只输出 `runtime.active`，不能输出 serve URL、host、port、pid 或 runtime 文件名；唯一例外是 `opsc_workspace_index_rebuild`，它只在 active loopback `opsc serve` 存在时读取 runtime state 中的相对 `bearer.token` 并调用 `/api/local/workspace/index/rebuild`，不把 token、token 文件路径或 serve URL 输出给 MCP client。两者不能混用，也不要把任一 token、launch secret 或 session id 写进 exports、日志、普通 JSON 或默认 CLI 输出。`GET /health` / `GET /api/health` 是唯一免鉴权健康检查端点，只能返回 `ok`。

浏览器 session cookie 是 `SameSite=Lax`，开发时不要混用 `localhost` 和 `127.0.0.1`。如果 Web UI 是 `http://localhost:3000`，本地工作区服务地址也优先填 `http://localhost:17680`；如果 Web UI 是 `http://127.0.0.1:3000`，再使用 `http://127.0.0.1:17680`。否则 session bootstrap 可能成功但后续 fetch 不带 cookie。

## AI Key 可能在两处

- 本地直连模式：当前应通过 `opsc serve` local profile 的 `secretRef` 解析用户 API Key，浏览器不应长期保存 API Key，也不应直接请求 OpenAI-compatible 接口。
- 远程渠道模式：管理员渠道密钥保存在后端 `settings.private.channels`，前端不应读取真实密钥。

Local profile 的完整 JSON 使用 `secretRef.name` 保存 env var 名；`opsc serve` summary 会脱敏成 `secretRef.reference`。Web UI 读取 profile 时需要兼容这两个字段，但两者都不是 secret 值，不能把它们当 API Key 展示、日志记录或发送给模型供应商。

`/api/local/ai/v1/*` 代理只能向模型供应商转发模型请求必需的 `Content-Type` / `Accept`，并用本地 profile `secretRef` 解析出的 secret 设置供应商 `Authorization`。浏览器传给 `opsc serve` 的 `Authorization`、cookie、local token、自定义 profile header 或 launch secret 不能继续转发给上游；缺失 env/file secret 的错误也不能回显 env value、文件路径、workspace path、bearer token 或 launch secret。

文档和 UI 说明需要区分这两种模式。

## GORM AutoMigrate 不是显式迁移系统

项目启动时自动迁移表结构，没有独立 migration / rollback CLI。表结构或字段变更需要同步 `docs/backend-database.md`，但不要假设可以精细回滚生产数据。

## Local Agent 任务不持久

`service/local_agent.go` 使用内存 map 保存任务。服务重启会丢失 pending/running local agent job。不要把它当 durable queue。

## PDD Console 权限很高

`docker-compose.pdd-console.yml` 使用 host network、host pid、privileged，并挂载 `/opt/pdd-workflow`。修改 PDD allowlist 动作、nsenter 或脚本执行边界前必须先确认安全影响。

## PDD sync_local 依赖反向 SSH

自定义工作流里的 `sync_local` / `scripts/trigger_local_receive_and_upload.sh` 需要 VPS 能连到 `127.0.0.1:22222`，通常由用户本地建立 `ssh -R 22222:127.0.0.1:22 ...`。未建立或被重置时应让脚本节点失败并提示“本地同步通道不可用”，不要把它伪装成仍在运行。

## PDD 创作画布轮询不能覆盖本地交互

结果页“创作画布”会同时有浏览器本地拖拽/缩放/生成状态、debounced 保存和后端 live workflow 状态。后台轮询只能非破坏性合并原始 run 节点状态，不能因 `updatedAt` 变化整张 hydrate，也不能重放服务器 viewport；否则会出现缩放抽搐、模型配置回退、替换图旧内容复活、多角度/裁剪节点消失和布局跳变。

## Render 免费版文件不持久

`docs/deployment.md` 已说明 Render 免费 Web Service 空闲休眠，且本地文件不是持久化存储。默认 SQLite 不适合长期正式数据。

## Flow2API 视频要落盘到持久卷

Flow2API 视频结果会写入 `VIDEO_STORAGE_ROOT`。Docker 部署时应让该目录在 `data/` 持久卷内，否则重启后视频工作台历史结果可能无法继续预览/下载。

## PDD Run 文件读取需要路径约束

PDD run id 和文件路径都应保持受限解析，避免从 API 读取 run 根目录外文件。新增 run/file 接口时沿用现有 safe path 和 run id 校验。

## 提示词定时同步会访问外部 GitHub

默认 prompt sync cron 是 `*/5 * * * *`，会同步内置 GitHub 远程提示词源。离线、本地演示或网络受限环境中，启动后可能出现同步失败日志。

## Docker 同时跑 Next.js 和 Go

运行镜像中 shell `CMD` 先启动 Go API，再启动 Next.js。`API_BASE_URL`、`API_PORT`、`WEB_PORT` 和 `APP_HOSTNAME` 的组合决定容器内代理和对外监听，排查 `/api/*` 时需要同时看两个进程。

## README 品牌与 repo remote 不完全一致

README 仍写 `infinite-canvas` 和上游 `basketikun/infinite-canvas` 部署链接；当前 repo path 和 remote 是 `ops-canvas-console` / `tudoumashu/ops-canvas-console.git`。对外发布前需要确认品牌和部署按钮是否应保持上游或改为当前 fork。

# Handoff

## Current Objective

Phase 8 Local Workspace v1 稳定化已进入收口：本轮只做 hardening / verification / docs-sync，不新增 local executor、不迁移现有 PDD/VPS run、不做 Full GC、不扩大 MCP 写能力、不新增 canonical object 类型。已补充 `opsc serve` runtime/session/auth/redaction 回归、AI proxy `secretRef` 与浏览器 header 隔离回归、本地模板草稿 run 到 canonical artifact ref happy path 回归；`cmd/opsc` 测试继续覆盖 MCP stdio wrapper smoke。README、features、contract、pending-test、todo、changelog 和 docs/ai 已同步本地事实源、浏览器缓存边界、旧浏览器测试数据不迁移、现有 PDD/VPS run 不迁移、MCP 只是薄封装等约束。下一阶段仍应优先设计真实 local workflow executor 和 project adapter，但必须复用 workspace core、`opsc serve` single-writer、auth/redaction/path safety 边界。

## Completed Work

- Phase 8 新增稳定化验证：`opsc serve` state/session/auth/redaction、AI proxy `secretRef` 与浏览器 header 隔离、本地模板草稿 run -> canonical artifact -> run artifact ref happy path；同步 README、features、contract、pending-test、todo、CHANGELOG、项目记忆和中央 Wiki。
- 已和用户拍板 local-first 数据分离基线：私有模板、run、artifact、个人素材、个人 prompt、本地项目路径和本地日志默认本地；云端只保留账号/授权/计费、公共模板、公共素材和商用 profile 能力。
- 已确认默认 workspace 为 `~/OpsCanvas`，支持多 workspace；项目文件只保存外部路径引用，不复制进 workspace；生成 artifact 复制进 workspace；secrets 不写普通 JSON；`opsc serve` 使用本地随机 bearer token 或 browser session。
- 已确认不迁移浏览器历史测试数据，不迁移现有 PDD VPS run；浏览器 IndexedDB/localforage 后续只做 UI 缓存或临时状态。
- 已新增 `docs/local-workspace-data-separation-plan.md` 作为数据分离计划入口。
- 本轮新增并补全 `docs/local-workspace-v1-contract.md`，定稿 Phase 0 contract：workspace resolution、目录布局、对象 ID、common object envelope、event envelope、canonical files、artifact metadata、run artifact ref、index contract、CLI envelope、exit code、`opsc serve` workspace 外 runtime metadata/token/session/port/pid 文件、secretRef、project capability、安全写入、云端边界、cache/export 排除规则、现有 DB/VPS/browser 兼容边界和 Phase 1 默认实现顺序。
- 本轮新增 `docs/ai/decisions/ADR-0002-local-workspace-v1-contract.md` 和 `docs/ai/diagrams/local-workspace-v1.mmd`，并更新项目记忆索引。
- Phase 1 新增 `internal/localworkspace` foundation：路径解析、workspace init/open/info/doctor、统一 envelope、manifest 命名、ULID、`secretRef` 结构校验、atomic JSON write、workspace lock、runtime metadata 读取、doctor report、泛型 JSON object repository、workspace scan 和 index rebuild Go interface；当前 lock/runtime 已迁到 workspace 外 XDG state。
- Phase 1 新增 `cmd/opsc`：`opsc workspace init/info/doctor`，支持 `--workspace`、`OPSC_WORKSPACE`、`--json`、`--show-paths` 和 JSON success/error envelope；默认输出不泄露 workspace 绝对路径。
- Phase 1 foundation 加固测试覆盖 path escaping、atomic write 失败不覆盖旧文件、lock、ID 格式与前缀校验、workspace schema 校验、`secretRef` 明文拒绝、scan 跳过 `.opsc/cache/exports`、rebuild lock 和 `opsc workspace` stdout/stderr 约定。
- Phase 1 `workspace doctor` 已覆盖 schema、目录完整性、manifest、lock 状态、index 文件、broken refs、`secretRef` 占位/明文字段和 project root 可访问性；human-readable report 始终输出 stderr，`--json` 模式下 stdout 输出 machine-readable report。
- Phase 3 新增 template/run/artifact typed repository：`TemplateRepository`、`RunRepository`、`ArtifactRepository`、`WriteRunArtifactRef`、`WriteRunNodeState`、`AppendRunEvent` / `ReadRunEvents` / `FollowRunEvents`、summary DTO、artifact 文件相对路径校验和 run status 校验。
- Phase 3 新增真实 `index.sqlite` 派生索引：增量记录 templates、runs、artifacts、run artifact refs、run node states、run events，并支持从 canonical JSON/JSONL/files 扫描重建；canonical 文件仍是唯一事实源。
- Phase 3 新增 `cmd/opsc` 查询命令：`opsc workspace index rebuild --json`、`opsc template list --json`、`opsc run list --json`、`opsc run status <run_id> --json`、`opsc run events <run_id>`、`opsc run events <run_id> --follow`、`opsc artifact list --json`、`opsc artifact list --run <run_id> --json`；查询不经过现有 DB，`run events` 输出 JSONL，不包 success envelope。
- Phase 4 新增 profile/project/asset/prompt typed repository：profile 只保存非 secret 配置和 `secretRef`，project 保存外部 `rootPath` 与 capability metadata 但 summary 不暴露路径，asset 校验 source artifact ref 与相对文件路径，prompt 使用 `prompt.json` + `content.md`。
- Phase 4 扩展 `index.sqlite` 派生索引：增量记录 profiles、projects、assets、prompts，并支持扫描 canonical JSON/文件重建。
- Phase 4 扩展 `workspace doctor`：补齐 asset file、prompt content、project rootPath 绝对路径、export rules 和 `secretRef` 文件引用边界检查。
- Phase 4 新增 `opsc workspace export plan --json`，默认只输出相对路径，并排除 `.opsc/`、`cache/`、`exports/`、`index.sqlite`、本地 project path document、`secretRef.type=file` document 和 symlink。
- Phase 4 新增查询命令：`opsc profile list --json`、`opsc project list --json`、`opsc asset list --json`、`opsc prompt list --json`；查询不经过现有 DB，不打印 secrets 或 project `rootPath`。
- Phase 5 新增 project 安全边界：workspace-local salted `rootFingerprint`、`.opsc/project-root.salt`、project path read/write/exec capability gate、默认 deny `.env` / `.git` / `node_modules`、symlink/root escape 防护，以及 project `credentialRefs` 结构校验。
- Phase 5 新增 `secretRef` 脱敏 summary，`file` 类型只显示 `"<file>"`，不回显本机私有文件绝对路径。
- Phase 5 新增 `opsc workspace gc plan --json` 和 `BuildGCPlan`，只做 dry-run，输出相对路径 candidates，动作为 `review`，覆盖 orphan artifact、broken run/asset artifact ref、缺失 asset file、缺失 prompt content 和缺失 workbench-log media file。
- Phase 5 扩展 `workspace doctor` 和 `index.sqlite`：doctor 检查 project fingerprint/execution policy/credentialRefs/GC 规则；index summary 覆盖 project fingerprint、asset 分类/用途/来源、prompt 分类/模型/状态等字段。
- Phase 6 新增 `opsc serve` session/runtime 改造：默认本机 `127.0.0.1:17680`，支持 `--port 0`、`--origin` local CORS 白名单、workspace 外 XDG state runtime、HTTP bearer token、browser 一次性 launch secret + HttpOnly session、`{code,data,msg}` HTTP envelope 和 `--json` runtime summary，默认不输出 token、launch secret、session id 或 workspace 绝对路径。
- `opsc serve` HTTP API 已覆盖 `/api/health`、runtime/workspace info、doctor、index rebuild、export/gc plan、templates/runs/artifacts/profiles/projects/assets/prompts/canvas-projects/workbench-logs summaries、local template create/get/update/delete、profiles/projects/assets/prompts/canvas-projects create/update/delete、workbench-log create/delete、run create/get/update/status、run events append/SSE、run node state、run artifact refs、artifact create/get/update/import、artifact/asset/workbench-log file read 和 prompt content read；health/bootstrap 外都需要 session cookie 或 bearer token，且带 `Origin` 的请求只接受 session。
- Phase 7 补齐 `opsc serve` profiles/projects/assets/prompts delete API，asset 增加 multipart import/update import，文件 canonical 写入 `assets/<asset_id>/files/`，并更新 index。
- Phase 7 新增 Web UI local workspace adapter：`web/src/services/local-workspace.ts` typed client、`use-local-workspace-store`、顶部本地工作区连接弹窗和启动时 refresh；浏览器只保存 loopback `baseUrl`，不保存 bearer token 或 launch secret。
- 本轮继续加固 local workspace 连接缓存：`use-local-workspace-store` 的 persist version migration 会把旧版 `opsc:local_workspace_connection` 中的 `workspace/runtime/tokenFile/launchSecretFile/status` 等字段丢弃，只保留规范化后的 loopback `baseUrl`。
- 本轮继续补齐 local workspace bootstrap UX：`use-local-workspace-store` 会先探测 `/api/health`，把服务未启动、服务已启动但未授权、错误 launch secret 和连接成功状态分开；顶部本地工作区弹窗与本地私有页面会给出明确提示。
- `我的素材`、`我的提示词` 和画布项目库已改为通过 `opsc serve` 加载、创建、更新、删除；旧 `infinite-canvas:*` 浏览器测试数据不迁移。素材/提示词 store 只在内存中保留当前 `loadedWorkspaceId` 的列表，持久化 cache 不再写私有列表或提示词正文，避免切换 workspace 时显示上一个 workspace 的数据。
- 素材中心、提示词中心、素材选择器、画布/工作台/PDD 创作画布“存素材”路径已等待 local workspace 写入结果；未连接本地工作区时显示错误而不是把浏览器缓存当事实源。
- “我的素材”zip 导出会从当前可读文件 URL / workspace 文件回显 URL 读取图片/视频 Blob 并打包，不再只有旧 browser `storageKey` 文件能进入导出包；素材包导入会通过 `addAsset` 写入 local workspace，不再把包内图片/视频恢复到浏览器 `image_files/media_files` 作为事实源。
- 本轮继续加固“我的素材”临时媒体边界：新增、更新或删除素材的 workspace 操作成功后，会清理当前素材列表和画布项目都不再引用的 `image:*`、`video:*`、`file:*` browser blob；保存失败时保留临时 Blob 供用户重试。
- Phase 7 本轮新增 local `canvas_project` canonical object：`canvas-projects/<canvas_id>/canvas-project.json` 保存画布节点、连线、聊天会话、背景和 viewport；index/doctor/rebuild/serve API 覆盖 create/list/get/update/delete。
- Web UI 画布项目库已改为通过 `opsc serve` 加载、创建、重命名、删除和导入画布项目；浏览器 key 改为 `opsc:canvas_store_cache:v1` 展示缓存，旧 `infinite-canvas:canvas_store` 不迁移。
- Phase 7 本轮新增画布项目媒体 canonical 化：`canvas-projects/<canvas_id>/files/` 保存图片/视频节点和助手媒体文件，`canvas-project.json.data.files` 保存 metadata，节点/助手消息通过 `workspaceFileKey` 引用；`opsc serve` 支持 canvas-project multipart `file:<file_key>` 上传和文件读取；index/doctor/GC 覆盖 `fileCount`、路径逃逸和缺失文件。
- Web UI 画布 store 保存时会把可读取的 `storageKey`、`data:` 或 `blob:` 媒体转成 workspace 文件；画布浏览器 cache 不再持久化项目列表，旧 `storageKey` 只作为兼容桥接。
- Web UI 画布导出 zip 现在会从 `opsc serve` 读取 `workspaceFileKey` 对应文件并写入压缩包；导入 zip 时这些文件只短暂落到浏览器 Blob 缓存，随后通过 `importProject` 重新上传到目标 workspace `canvas-projects/<canvas_id>/files/`，导入结束会清理临时 `image:import_*` / `file:import_*` browser blob。
- 本轮继续加固画布临时媒体边界：画布项目保存、导入或删除成功后，会清理已写入 workspace 且当前画布状态不再引用的 `image:*`、`video:*`、`file:*` browser blob；若保存期间用户继续编辑并仍引用同一 `storageKey`，则保留该临时 Blob 到下一次安全保存。
- Phase 7 本轮新增 local `workbench_log` canonical object：`workbench-logs/<wblog_id>/workbench-log.json` 保存 text/image/video 工作台生成记录，关联媒体写入同目录 `files/`；index/doctor/rebuild/GC/serve API 覆盖 create/list/get/delete 和 media file read。
- Web UI 文本、图片、视频工作台生成历史已改为通过 `opsc serve` 读写 `workbench-logs/`；图片/视频工作台生成结果保存成功后会把当前结果卡片替换为从 `/api/local/workbench-logs/<id>/files/<media_key>` 读取的 URL；旧 `text_generation_logs`、`image_generation_logs`、`video_generation_logs` 浏览器测试数据不迁移，生成中、上传参考图和失败未保存的 Blob 仍属于临时状态。
- 本轮继续加固工作台临时媒体边界：图片/视频工作台保存生成记录成功后会把当前参考图替换为 workspace 文件 URL，并清理本次已 canonical 化的 `image:*` browser blob；新会话、移除参考图和切换历史记录也会清理当前参考图临时缓存；视频结果保存成功后释放保存前使用的临时 `blob:` URL。
- Phase 7 本轮新增 `opsc serve` AI proxy：`/api/local/ai/v1/*` 根据 workspace profile channel 的 `baseUrl` 和 `secretRef` 调 OpenAI-compatible target，当前支持 env 和绝对路径 file secret resolver，不转发浏览器 Authorization/cookie 到模型供应商。
- Web UI AI 配置本地模式已接入 local profile：配置弹窗保存 Base URL、模型列表、默认模型和 env `secretRef` 到 `profiles/<profile_id>/profile.json`；模型列表、图片/图片编辑/文本问答/视频请求改走 `opsc serve` proxy；`use-config-store` 不再持久化配置，启动清理会移除旧 `infinite-canvas:ai_config_store`，浏览器不再长期保存 API key。
- Web UI 读取 local AI profile 时已兼容完整 profile document 的 `secretRef.name` 和脱敏 summary 的 `secretRef.reference`；自定义 env var 名不应在保存/刷新后隐式回退到 `OPENAI_API_KEY`。
- Web UI 连接、刷新或断开 local workspace 时会同步加载或清空当前 workspace 的 AI local profile；目标 workspace 没有 profile 或连接不可用时会把内存 profile 配置重置为默认值，避免继续使用上一个 workspace 的 Base URL、模型列表、默认模型或 `SecretRef`。
- 本轮新增 Web UI 本地项目引用面板：顶部本地工作区弹窗通过 `/api/local/projects` 加载、创建、编辑和删除 `projects/<proj_id>/project.json`；列表只显示 id、kind、adapter、capabilities 和 `rootFingerprint`，不显示 `rootPath`；编辑时才请求 `showPaths=1`，含 `credentialRef` 的项目提示用 CLI 修改，避免 Web UI 写回脱敏 summary。
- Web UI 启动时会清理 legacy browser private state keys：旧 AI config、旧素材/提示词/画布 store、旧 text/image/video generation logs 和旧 workflow folders；不清理当前 `opsc:*_cache:v1` 展示缓存或 `image_files/media_files` 临时上传桥接库。
- 本轮新增 local template HTTP CRUD 和 Web UI 电商模板 adapter：连接本地工作区时，模板列表/新建/复制/删除和模板编辑器加载/保存会通过 `opsc serve` 读写 `templates/<tpl_id>/template.json`；未连接本地工作区时继续走现有服务器模板接口。
- 本轮继续收口本地模板素材边界：连接本地工作区时，模板编辑器 `material_lookup` 节点的“固定选择一个素材”读取当前 workspace 的 `我的素材` 图片列表，不再请求服务器 admin asset 接口；未连接本地工作区的服务器模板仍沿用原有 admin asset 查询。
- 本轮新增 local run/artifact HTTP write API 和 Web UI 基础接入：`opsc serve` 支持 run create/update、event append、node state、artifact ref、artifact create/update/import/file read；电商模板运行在连接本地工作区时会创建 local run、写入模板节点状态和 `run.waiting_for_executor` 事件；`/workflows/ecommerce` 可列出 local runs，`run_<id>` 运行页会显示本地 run summary、nodes、events、artifact refs 和 artifact 文件预览。
- 本轮继续收口 local run 固定素材边界：连接本地工作区启动本地电商模板时，固定 `material_lookup` 节点会读取 `assets/<asset_id>/files/original`，复制成 `artifacts/<art_id>/files/original`，写入 `runs/<run_id>/artifacts/<art_id>.ref.json`，并把对应节点状态标记为 `success`；浏览器只做传输桥接，不保存长期事实数据。
- 本轮新增 workspace preferences HTTP API 和 Web UI 工作流入口 adapter：`opsc-workspace.json.data.preferences.workflowFolders` 保存自定义工作流文件夹；`GET/PUT /api/local/workspace/preferences` 走 workspace 写锁、atomic write、revision 检查和明文 secret 拒绝；`/workflows` 页面连接本地工作区后读取/写入该字段，未连接时提示先连接，不再写旧 `ops-canvas-workflow-folders` localStorage key。
- 本轮新增并加固 `opsc mcp` stdio MCP 薄封装：支持 initialize/ping/tools/list/tools/call，读取/诊断/dry-run 工具映射到现有 CLI JSON 命令，覆盖 workspace info/doctor/export plan/gc plan、template/run/artifact/profile/project/asset/prompt 列表和 run status/events；`opsc_workspace_index_rebuild` 改为通过 active `opsc serve` bearer API 调 `/api/local/workspace/index/rebuild`，只重建派生 `index.sqlite`；不暴露 canonical object 写入工具、不暴露 `run events --follow`、不直接读写 workspace repository。
- 结果页创作画布移除左上角说明气泡，连接线恢复 `ConnectionPath` 默认贝塞尔曲线。
- 结果页媒体节点通过 `CanvasNode mediaFrame` 启用带背景和边框的节点包裹感，不改变原生画布默认媒体节点样式。
- 创作画布图片/视频节点会根据保存的 `naturalWidth/naturalHeight` 或前端探测到的媒体尺寸适配节点宽高；后端初始化图片 artifact 时也会尝试写入自然尺寸。
- 本轮继续补齐结果页创作画布：小地图使用紧凑节点标识，新增节点按外框寻找空位；悬浮工具栏延迟隐藏，鼠标从节点移动到工具栏时不应闪退；工具栏已接入文本编辑、文本生图、图片/视频替换、下载、存素材、锁比例、裁剪、多角度、查看大图、失败重试、编辑面板和“应用副本并重跑下游”。
- 后端新增创作画布输出应用接口：结果页生成/裁剪/多角度/替换后的节点可覆盖对应工作流节点输出，并沿原模板 DAG 重跑下游。
- 已排查 `custom_20260529_073257`：VPS run 状态文件为 failed，失败点是 `sync_local`，日志显示连接 `127.0.0.1:22222` 被重置；未发现该 run 仍有执行进程。
- `sync_local` 脚本节点现在会先预检 VPS 到 `127.0.0.1:22222` 的反向 SSH 通道，不可用时返回明确错误；自定义 run 若无 running 商品但存在失败商品，会收敛为 `error`。
- 模板编辑器为模型调用节点增加“失败重试”配置：启用、重试次数、间隔秒数；默认启用、次数 0 无限、间隔 0 系统退避。
- 后端执行器对模型调用节点读取 `node.retry`；guardrail 生成/修复/复检瞬时重试改为同一语义，不再默认固定 100 次。
- 本轮新增修复：创作画布不再因后台轮询的 `updatedAt` 变化整张重灌节点或重放服务器 viewport；后续轮询只非破坏合并 live 状态，避免缩放抽搐、模型配置回退和生成节点消失。
- 本轮新增修复：保存逻辑改为串行保存最新 refs，避免保存过程中新增的裁剪/多角度/生成结果没有被再次保存。
- 本轮新增修复：图片/视频替换会清理旧生成 metadata、标记 `source=user_upload` 并立即更新节点内容和尺寸；锁比例/自由比例对齐原生画布；裁剪和多角度弹窗增加提交 loading 并保留结果节点。
- 本轮新增修复：后端 creative canvas merge 不再覆盖用户上传/creative 派生节点内容，也不再覆盖已保存的 `prompt/model/size/quality/count` 等配置；新增 `service/pdd_test.go` 覆盖关键 merge 语义。
- 本轮新增修复：旧保存创作画布如果只有 run 派生节点且媒体真实尺寸导致节点矩形重叠，后端 live merge 会自动重排一次；包含用户上传或 creative 派生节点时不自动重排，避免打乱用户本地结果。
- `docs/pending-test.md` 已记录需要人工回归的前端交互和重试行为。
- 本地 `npx tsc --noEmit` 已通过；本地 Docker `golang:1.25-alpine` 执行 `gofmt` 和 `go test ./...` 已通过。
- 已运行 `git diff --check`。
- 本轮相关文件已通过 tar-over-ssh 同步到 VPS `/opt/ops-canvas-console`；VPS 执行 gofmt、`docker compose -f docker-compose.pdd-console.yml up -d --build app`，API `/api/health` 返回 `ok`，Next `/workflows/pdd` 返回 `HTTP/1.1 200 OK`。
- 本轮最终浏览器回归已通过：`custom_20260529_073257` 创作画布 8 个节点无重叠、5 张图片加载完成、工具栏按视口夹取且从节点移动到工具栏后仍可点击、缩放后等待 6.5 秒最大位移为 0；API 确认该 run 已收敛为 `error`。

## In Progress

- Phase 7 还未覆盖全部 workflow 执行边界：local run 目前只能在创建时 materialize 固定本地素材 artifact，其它节点仍是 pending 草稿/记录；真实 PDD/VPS executor、PDD/VPS run 数据迁移，以及运行时 `material_lookup` 自动匹配本地素材仍待迁移或审计。

## Blockers

- 当前本机没有 `go` / `gofmt`，Go 格式化需通过 Docker 或 VPS 环境执行。
- fact：VPS 上本项目部署目录为 `/opt/ops-canvas-console`，compose 文件为 `docker-compose.pdd-console.yml`，容器名为 `pdd-run-console`。
- fact：VPS `0.0.0.0:443` 当前监听进程是 `sshd`；应用容器内/host network 暴露的服务监听在 `127.0.0.1:18080` API 和 `127.0.0.1:13000` Next。
- fact：截至提交 `2923795`，此前大量工作区改动已被提交并推送到 `origin/main`。

## Files Or Areas To Avoid

- 不要回滚或覆盖初始化前已经存在的未提交业务改动。
- 不要读取或记录 `.env`、真实密钥、模型 API Key、Linux.do Client Secret、PDD 账号/会话。
- 不要在未确认前改 `docker-compose.pdd-console.yml` 的 privileged / host network / nsenter 行为。
- 不要把本地 workspace 的画布项目、画布媒体、“我的素材”、素材媒体、“我的提示词”、工作台生成记录、工作流入口自定义文件夹和电商私有模板误写成服务器云同步；它们通过 `opsc serve` 写入用户本机 workspace。
- “我的素材”里的图片/视频只允许在保存/更新前短暂使用 browser blob 作为上传桥接；workspace 写入成功后，当前素材列表和画布项目不再引用的 browser media blob 应清理。
- 画布图片/视频节点和助手媒体只允许在保存/导入前短暂使用 browser blob 作为桥接；保存成功后，已 canonical 化且当前状态不再引用的 browser media blob 应清理。
- 工作台图片/视频上传参考图只允许作为保存前的 browser bridge；生成记录保存成功后，参考图和结果都应从 `workbench-logs/<wblog_id>/files/` 回显，已保存的 browser reference blob 应清理。

## Next Recommended Steps

- 下一阶段：设计并实现真实 local workflow executor，把 pending local run 接到可执行队列、PDD/local project adapter 和 artifact 产物写入；如需扩展 MCP，只在已有 CLI/core 或 `opsc serve` 能力上增加写入型工具，不重复实现业务逻辑。
- run/artifact 写入 HTTP API 已暴露给本地 Web adapter；下一阶段要补执行器鉴权/actor 约束、事件语义和失败恢复策略。
- `workspace doctor` 下一阶段可增加 index 新鲜度/重建建议；当前只做结构、引用和占位符级检查，不解析真实 secrets 或模型供应商凭据。
- project path guard 当前还只是 foundation API，下一阶段需要由 project adapter 或写入型业务 API 实际调用，才能形成端到端执行边界。
- 本地项目引用现在已有 Web UI 入口，但仍只是 workspace 引用管理；下一阶段需要让真实 local executor 使用 `proj_<id>`、capability guard 和 adapter metadata 执行业务。
- 对真实产物写入类动作继续人工回归：替换图片旧内容不残留、自由比例/锁比例、裁剪确认、多角度生成节点保留、artifact 预览、应用副本后下游重跑。
- 如后续要求公网 Web 直接通过 `https://96.9.225.98` 访问，需要先单独确认反向代理/域名/端口方案；本轮未修改部署配置或 `.env`。

## Validation Status

- passed：Phase 8 已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace/serve_test.go`。
- passed：Phase 8 已运行 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`，覆盖 `opsc serve` auth/redaction/session、AI proxy `secretRef` 边界、本地模板草稿 run/artifact ref happy path 和 `cmd/opsc` MCP stdio wrapper smoke。
- passed：Phase 8 已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`。
- passed：Phase 8 中央 Wiki 已更新轻量 project entity，并运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- passed：Phase 0 文档变更已运行 `git diff --check`，diff 范围只包含 Markdown/Mermaid 文档。
- passed：中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- passed：Phase 1 已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc`。
- passed：Phase 1 已运行 `go test ./internal/localworkspace ./cmd/opsc`。
- passed：Phase 1 foundation 加固后已再次运行 `gofmt`、`go test ./internal/localworkspace ./cmd/opsc` 和 `go test ./...`。
- passed：Phase 1 已用 `go run ./cmd/opsc` 在临时 workspace 烟测 `workspace init/info/doctor --json`。
- passed：Phase 1 已通过挂载 `/tmp` Go module/build cache 的 Docker 容器运行 `go test ./...`。
- passed：本轮 `workspace doctor` 输出约定和诊断加固修改后，已重新运行 Docker `gofmt`、`go test ./cmd/opsc ./internal/localworkspace`、`go test ./...` 和 `go run ./cmd/opsc` 烟测。
- passed：Phase 3 template/run/artifact repository、run events、index.sqlite 和查询 CLI 修改后，已运行 Docker `gofmt`、`go test ./cmd/opsc ./internal/localworkspace`、`go test ./...`，并用 `go run ./cmd/opsc` 烟测空 workspace 的 `workspace index rebuild`、`template list`、`run list`、`artifact list` JSON 输出。
- passed：Phase 4 profile/project/asset/prompt repository、doctor、export plan、index 和查询 CLI 修改后，已运行 Docker `gofmt`、`go test ./internal/localworkspace ./cmd/opsc` 和 `go test ./...`。
- passed：Phase 4 已用 `go run ./cmd/opsc` 烟测空 workspace 的 `workspace init/info/doctor/index rebuild/export plan`、`profile list`、`project list`、`asset list`、`prompt list` JSON 输出。
- passed：Phase 5 project root fingerprint/path guard、secretRef summary、GC dry-run、doctor、index 和 CLI 修改后，已运行 Docker `gofmt`、`go test ./internal/localworkspace ./cmd/opsc` 和 `go test ./...`。
- passed：Phase 5 已用临时 workspace 烟测 `workspace init/info/doctor/index rebuild/export plan/gc plan`、`profile list`、`project list`、`asset list`、`prompt list` JSON 输出，确认默认输出不暴露敏感绝对路径。
- passed：Phase 5 已运行业务路径 guard，确认本轮未改 `main.go`、router、service、repository、DB/model、handler、config 或 web。
- passed：Phase 6 `opsc serve` 已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc`、`GOPROXY=https://goproxy.cn,direct go test ./...` 和 `git diff --check`；单测覆盖 XDG state runtime、token/launch secret 文件权限、`/api/health` 免鉴权不泄露、非授权拒绝、Origin bearer 拒绝、session bootstrap、runtime summary 脱敏、profiles/projects/assets/prompts 写入脱敏、并发写入和 graceful cleanup。首次默认 `go test ./...` 因 `proxy.golang.org` 依赖下载 EOF 失败，备用 GOPROXY 后通过。
- passed：Phase 7 Web UI adapter 已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc` 和 `go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`。
- passed：Phase 7 workbench logs adapter 已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace cmd/opsc` 和 `go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`。
- passed：Phase 7 canvas media canonicalization 已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace cmd/opsc` 和 `go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`。
- passed：Phase 7 asset temp media cleanup 已运行 `cd web && npx tsc --noEmit`、`git diff --check` 和中央 Wiki lint/reindex/embed。
- passed：Phase 7 AI profile secretRef roundtrip 修复已运行 `cd web && npx tsc --noEmit`、`git diff --check` 和中央 Wiki lint/reindex/embed。
- passed：Phase 7 local projects Web UI adapter 已运行 `cd web && npx tsc --noEmit`、`git diff --check` 和中央 Wiki lint/reindex/embed。
- passed：Phase 7 canvas zip media roundtrip 已运行 `cd web && npx tsc --noEmit`。
- passed：Phase 7 AI profile/proxy 已用 Docker `golang:1.25-alpine` 执行 `gofmt` 和 `GOPROXY=https://goproxy.cn,direct go test ./internal/localworkspace ./cmd/opsc`；首次默认 Go proxy 依赖下载 EOF 后备用 GOPROXY 通过。已运行 `cd web && npx tsc --noEmit`。
- passed：Phase 7 canvas temp media cleanup 已运行 `cd web && npx tsc --noEmit`。
- passed：Phase 7 workbench preview canonicalization 已运行 `cd web && npx tsc --noEmit`。
- passed：Phase 7 local workflow templates adapter 已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace` 和 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`。
- passed：Phase 7 workspace preferences adapter 已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace` 和 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`。
- passed：Phase 7 local run/artifact adapter 已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace/serve.go internal/localworkspace/serve_workflow_writes.go internal/localworkspace/serve_test.go` 和 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`。
- passed：Phase 7 workbench reference temp cleanup 已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- passed：Phase 7 legacy browser state purge 已运行 `cd web && npx tsc --noEmit`、`git diff --check` 和中央 Wiki lint/reindex/embed。
- passed：Phase 7 template material asset boundary 已运行 `cd web && npx tsc --noEmit`、`git diff --check` 和中央 Wiki lint/reindex/embed。
- passed：Phase 7 fixed material run artifact refs 已运行 `cd web && npx tsc --noEmit`、`git diff --check` 和中央 Wiki lint/reindex/embed。
- passed：Phase 7 项目记忆同步后，中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- passed：Phase 7 serve availability UX 已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- passed：Phase 7 `opsc mcp` stdio 薄封装和 index rebuild single-writer 加固已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w cmd/opsc/mcp.go cmd/opsc/mcp_test.go` 和 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./cmd/opsc ./internal/localworkspace`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- not run：Phase 7 `opsc mcp` 尚未在真实 Codex / Claude Code MCP client 中做端到端配置测试；本轮未运行 full build 或前端 typecheck，因为改动范围是 Go CLI/MCP 和文档/项目记忆。
- not run：本轮未运行 full build，也未在真实浏览器中启动 `opsc serve` 回归画布项目 create/import/export、session/CORS 和已有媒体节点 Blob 回显。
- passed：中央 Wiki 已更新轻量 project entity，并运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- previous passed：Phase 5 `opsc serve` 已构建临时二进制 smoke `workspace init` + `serve --port 0`，验证 `/health`、bearer token 调本地 API、runtime/JSON 输出不含 token 或 workspace 绝对路径、SIGTERM 后清理 runtime/lock。
- passed：本轮 local workspace 文档与中央 Wiki 同步后，已运行 `git diff --check`、`lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- previous passed：`cd web && npx tsc --noEmit`。
- previous passed：`docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine gofmt -w service/pdd.go service/pdd_test.go`。
- previous passed：`docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine go test ./...`。
- passed：`git diff --check`。
- passed on VPS：tar-over-ssh 同步本轮相关文件；`docker compose -f docker-compose.pdd-console.yml up -d --build app`；`curl -fsS http://127.0.0.1:18080/api/health`；`curl -fsSI http://127.0.0.1:13000/workflows/pdd`。
- passed browser：Playwright 登录后打开 `custom_20260529_073257` 创作画布，确认节点不重叠、图片加载、悬浮工具栏稳定、节点信息按钮可点击、缩放后等待轮询不抖动。
- not run：真实多角度生成和应用副本重跑会消耗模型额度或改写 run 产物，保留在 `docs/pending-test.md` 做人工回归。

## Recent Important Commits

- `f9e4c92 refactor(video): 重构视频配置参数标准化逻辑`
- `1be6b4f feat(video): 新增视频创作台页面`
- `7968820 feat(account): 新增用户账号与算力点体系`
- `8c506f9 feat(video): 对齐 grok-imagine-video 接口并优化视频生成功能`
- `b21f8c3 feat(admin): 优化管理后台渠道模型选择功能`
- `d8cb1d6 fix(image): 修复图像生成中quality参数传递问题`
- `b8e50c1 feat(auth): 添加用户注册开关功能`
- `030541b feat(canvas): 添加画布图片信息显示功能`
- `2934b1d refactor(ai): 重构AI接口算力点扣费逻辑并添加失败返还机制`
- `01f2a4d feat(ai): 实现AI模型调用的积分计费功能`

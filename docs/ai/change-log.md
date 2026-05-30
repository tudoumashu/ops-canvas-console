# AI 项目记忆变更记录

## 2026-05-31 | Hybrid Ecommerce VPS smoke helper and probe | commit: pending

- 目标：为 Phase 11 真实 VPS API smoke 留下可重复执行的最小证据链路，避免后续手工串命令时绕过 local workspace canonical/source 和 `opsc` 边界。
- 变更：新增 `tools/hybrid_ecommerce_vps_smoke.py`，只编排 `opsc ecommerce import-template`、`opsc ecommerce create-run`、`opsc executor --run`、`opsc run status` 和 `opsc artifact list --run`；不直接读取 workspace 文件、不直接调用 VPS API、不打印 secret，并可写出 redacted evidence JSON。新增 `docs/manual-test-report-phase11.md` 记录当前目标 VPS probe、缺失 env/template 前置条件和后续复测命令。
- 原因：Phase 11 的真实验收必须证明本地 canonical run 能触发 VPS PDD API 并同步 artifact；当前网络/credential 条件不足，应把失败前置条件记录为可复现证据，而不是把 smoke 误写成通过。
- 验证：已运行 `python -m py_compile tools/hybrid_ecommerce_vps_smoke.py`；已用缺失 `OPSC_HYBRID_VPS_TOKEN` 的场景执行 helper，返回 exit 2，并生成只含 redacted workspace、remote URL、template placeholder 和 missing env 的 evidence。最新网络 probe 显示目标 `92.9.225.98` 的 TCP 22/443/80/18080/13000 可连通，但 SSH banner/key exchange 和 HTTP(S) health 仍不可用。
- 影响：只新增 smoke helper 和文档/项目记忆；不改业务代码、不改 Web UI、不扩大 MCP 写面、不迁移旧 PDD/VPS run。
- 风险：真实 VPS API smoke 仍未完成；需要可达 API、真实 admin credential 和已确认 template id 后重新执行 helper。
- 后续：拿到前置条件后执行 `tools/hybrid_ecommerce_vps_smoke.py --workspace ~/OpsCanvas --input-file /path/to/hybrid-input.json --evidence /tmp/opsc-phase11-vps-smoke.json`，并把结果更新到 `docs/manual-test-report-phase11.md`。

## 2026-05-31 | Hybrid Ecommerce headless local run draft | commit: pending

- 目标：补齐 Phase 11 hybrid ecommerce 的 headless 本地 run 创建路径，让 CLI 可以在不经过浏览器、不直接调用 VPS API 的情况下，从已导入模板创建 canonical local run 草稿。
- 变更：新增 `CreateHybridEcommerceRun`，基于已导入的 `metadata.hybridEcommerce.backend=vps_pdd` template 创建 pending `runs/<run_id>/run.json`、template snapshot、pending node state 和 `run.waiting_for_executor` event；新增 `opsc ecommerce create-run <tpl_id> --input-file <json> --json`，输入文件支持 JSON object 或 bare inputs array，模板默认 inputs/productConcurrency/maxRetries 会合并到 run input，CLI 输出不包含 workspace 绝对路径、输入文件路径或 secret。
- 原因：用户后续需要用 CLI/MCP/agent 驱动同一套 local-first 工作流；导入模板和 executor 中间需要一个可脚本化、可测试的 canonical run draft 创建入口，但不能让浏览器或 VPS run dir 成为事实源。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w cmd/opsc/main.go cmd/opsc/main_test.go internal/localworkspace/executor_test.go internal/localworkspace/hybrid_ecommerce.go`；已运行 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`，覆盖 CLI create-run 脱敏、template defaults 合并、waiting event、pending node state 和 hybrid executor 继续从该 run 执行。
- 影响：只增加 local workspace hybrid run draft 的 core/CLI/test/doc；不改旧 `main.go`、router、service、repository、DB 行为，不改 Web UI 语义，不迁移现有 PDD/VPS run，不扩大 MCP mutation surface。
- 风险：真实 VPS smoke 当前仍未完成；目标 VPS `92.9.225.98` 通过 SSH `-p 443` banner exchange 超时、SSH `-p 22` key exchange 被关闭，直接 health 访问超时或 empty reply，且本机未设置 `OPSC_VPS_ADMIN_TOKEN` / `OPSC_HYBRID_VPS_TOKEN` / `PDD_ADMIN_TOKEN` 与远端 template id env。
- 后续：拿到可达 VPS API、admin credential 和确认 template id 后，按 `import-template -> create-run -> executor --run <run_id> -> run status/artifact list` 做真实 smoke；之后再补 Web UI 一条 hybrid run 浏览器回归和远端事件增量同步。

## 2026-05-30 | Hybrid Ecommerce VPS backend MVP | commit: pending

- 目标：在不迁移现有 PDD/VPS run、不把 VPS run dir 当事实源、不扩大 MCP 写面的前提下，先接通一个已确认电商模板的 local workspace -> VPS API 真实执行路径。
- 变更：新增 `internal/localworkspace/hybrid_ecommerce.go`，提供远端 PDD template 导入、hybrid template metadata、profile/channel `secretRef` 解析、远端 run 创建、overview/product-detail 轮询和 artifact 下载同步；`opsc executor` 遇到 `hybridEcommerce.backend=vps_pdd` 模板时走 VPS PDD API backend，并把本地 run/node state/events/artifact ref 作为 canonical；新增 `opsc ecommerce import-template` CLI。
- 原因：用户计划把自用 local workspace 作为事实源，同时让 VPS 作为已存在电商工作流的真实执行后端；浏览器仍只能连接 `opsc serve`，不能保存或发送 VPS admin credential。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt`；已运行 `/usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`，覆盖 CLI 导入脱敏、profile/channel secretRef、远端 run start/status/artifact sync、workspace 不写明文 token 或远端 runDir、重复 executor 不重复同步 success run。
- 影响：新增 local workspace hybrid backend 和 CLI 子命令；不改旧 `main.go`、router、service、repository、DB 行为，不改 Web UI 直接调用路径，不迁移 PDD/VPS run 历史，不扩大 MCP mutation surface。
- 风险：真实 VPS smoke 仍依赖可用 admin token 和已确认 remote template id；当前只同步 overview/product-detail 和关键 artifact，不做远端细粒度事件流、取消/重试控制、危险运维动作或历史 run 导入。
- 后续：下一阶段优先做真实 VPS credential/template id smoke、Web UI 一条 hybrid run 浏览器回归、远端事件增量同步和 `image_edit`/`video_generation`/复杂 guardrail 的 executor 扩展。

## 2026-05-30 | Local Workflow Executor Phase 10 project-aware 收口 | commit: pending

- 目标：在不扩大 MCP 写面、不迁移 PDD/VPS run 的前提下，把 Phase 9 executor MVP 接到本地项目 capability/path guard，并补齐最小 `condition`/`script` 执行链路。
- 变更：`internal/localworkspace` executor 读取 run `projectId`，校验 project adapter、root fingerprint、capability、path safety 和 `artifact.write`；新增 `condition` 节点、project/local `script` 节点、`source/target` edge fallback、`fromHandle`/condition 路由跳过、节点级 retry、project output mapping、脚本最小环境变量和 root/secret 脱敏。Web 本地模板启动参数允许透传 `profileId/projectId` 到 local run；新增 `tools/local_workspace_browser_smoke.py` 作为真实浏览器 local workspace smoke 脚本。
- 原因：用户后续要把本系统作为自用本地项目管理和多 agent 接入底座；executor 需要先复用已有 workspace project capability model，而不是另起第二套本地脚本执行事实源。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `gofmt`；已运行 `go test ./internal/localworkspace ./cmd/opsc`，覆盖 project script retry/output mapping、condition edge skip、capability deny、path escape、secret/root redaction、index rebuild/status/events 和既有 Phase 8/9 回归。已新增 browser smoke 脚本并通过 Python 语法检查；真实浏览器端到端仍列入 `docs/pending-test.md`。
- 影响：新增本地 executor project-aware 能力和轻量浏览器 smoke 工具；不改旧 Go main/router/service/repository/DB，不改 PDD/VPS run 事实源，不扩大 MCP mutation surface，不新增 canonical object 类型。
- 风险：仍未实现 `image_edit`、`video_generation`、复杂 loop/guardrail、完整模板级重试、自动素材匹配、专用文章/视频/电商 adapter、安装打包和 CI 浏览器回归；真实模型账号和真实浏览器 session 仍需人工回归。
- 后续：Phase 11 优先做专用 project adapter 真接入、浏览器自动化回归落地和安装/打包文档，不优先迁移旧 PDD/VPS run 或扩大 MCP 写面。

## 2026-05-30 | Local Workflow Executor MVP | commit: pending

- 目标：把 local workspace 的 pending run 草稿接入一个明确、唯一的本地执行入口，并验证 canonical node state、events、artifact/ref 写回链路。
- 变更：新增 `internal/localworkspace` executor 和 `opsc executor` CLI。executor 领取 `run.waiting_for_executor` 的 pending run 或恢复已由 executor 接管的 running run，按模板 DAG 执行 `input`、`text_static`、固定本地素材 `material_lookup`、`text_generation`、`image_generation`，复用 local profile `secretRef` 调 OpenAI-compatible provider，并写回 run/node 状态、append-only events、global artifact 和 run artifact ref。本地 run 状态页在 pending/running 阶段轮询，并在进入终态后刷新 events/artifacts。新增单测覆盖 fixed material + text + image happy path、secretRef provider auth、running run 恢复跳过已成功节点和 CLI `opsc executor --json`。
- 原因：Phase 7/8 已具备 local run/artifact canonical 记录和 Web UI 状态页，但 run 只能停在草稿；需要先用单机、自用、最小节点集打通真实执行闭环，再进入 project adapter。
- 验证：已运行 Docker `golang:1.25-alpine` `gofmt -w internal/localworkspace/serve_ai_proxy.go internal/localworkspace/executor.go internal/localworkspace/executor_test.go cmd/opsc/main.go cmd/opsc/main_test.go`；已运行 `GOPROXY=https://goproxy.cn,direct go test ./internal/localworkspace ./cmd/opsc`。本轮继续补充 executor 写入后删除并重建 `index.sqlite` 的回归，确认 run status、node states、event sequence 和 run artifact summaries 可从重建索引读出；已用 `/tmp/opsc-phase9-manual` fake provider 手工 smoke 跑通 `opsc executor` 固定素材 + 文本生成 + 图片生成 CLI happy path，证据见 `docs/manual-test-report-phase9.md`。
- 影响：新增本地 executor core 和 CLI 入口；复用现有 localworkspace repository、lock、profile `secretRef`、AI proxy URL/secret resolver 和 `{ ok, data, warnings }` CLI envelope；不改旧 Go main/router/service/repository/DB，不改旧云端/后台/PDD 路由语义，不扩大 MCP mutation surface。
- 风险：当前 executor 仍是 run-once MVP，不含 daemon 调度、分布式 lease、完整 project adapter、`image_edit`、`video_generation`、条件、脚本或模板级重试配置；CLI/fake provider 链路已通过，真实浏览器 + 真实 workspace + 真实模型账号仍需手工回归。
- 后续：Phase 10 优先接本地项目 adapter，使 `projects/<proj_id>/project.json` 的 capability guard、path safety 和 adapter metadata 进入真实工作流；继续不要先扩大 MCP 写面或迁移旧 PDD/VPS run。

## 2026-05-30 | Local Workspace Phase 8.1 MCP workspace info redaction closeout | commit: pending

- 目标：只修复 MCP `opsc_workspace_info` 默认输出泄露本地 serve URL 的问题，并把 Phase 8 手工验收收口到可关闭状态。
- 变更：`opsc_workspace_info` 继续调用既有 CLI/core，但在 MCP 包装层对成功 stdout 做专用脱敏，默认把 `data.runtime` 缩减为 `{ "active": true|false }`，不再输出 `runtime.baseUrl`、`runtime.host`、`runtime.port`、`pid`、`tokenFile` 或 `launchSecretFile`。补充 active `opsc serve` 下的 MCP workspace info 回归测试；更新 Phase 8 手工验收报告和 pending-test，将剩余项标记为非阻塞跟进。
- 原因：CLI `workspace info --json` 和 `opsc serve` 需要保留 runtime metadata，但 MCP 默认输出面向 agent，不应暴露可重建本机 loopback serve URL 的字段。
- 验证：已运行 Docker `golang:1.25-alpine` `/usr/local/go/bin/gofmt -w cmd/opsc/mcp.go cmd/opsc/mcp_test.go`；已运行 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./cmd/opsc ./internal/localworkspace`；已用真实 `opsc` 二进制、临时 workspace 和 active/inactive `opsc serve` 复验 MCP 目标项，证据写入 `/tmp/opsc-phase8-1/evidence-F-mcp-phase8-1.json`。
- 影响：只改 MCP `workspace_info` 输出脱敏和测试/文档；不改 CLI `workspace info` 输出、不改 `opsc serve`、不改 `opsc_workspace_index_rebuild`、不改 Web UI 本地连接、不新增 local executor、不扩大 MCP mutation surface。
- 风险：真实 Codex / Claude Code 客户端 UI 展示层仍可做 spot check，但协议级自动化和真实二进制目标验收已覆盖本轮泄露风险。
- 后续：可以关闭 Phase 8；进入 Phase 9 前仍需保持 MCP 薄封装、`opsc serve` single-writer、auth/redaction/path safety 边界。

## 2026-05-30 | Local Workspace Phase 8 stabilization verification | commit: pending

- 目标：把 Local Workspace v1 从“功能已铺开”收口到“可验证、可回归、可对外说明”的稳定化状态。
- 变更：补充 `opsc serve` runtime/session/auth/redaction 回归，覆盖 state 目录和 token/session 文件权限、CLI `serve` 普通/JSON 输出脱敏、错误响应脱敏、Origin bearer 拒绝、browser session runtime/workspace 脱敏；补充 AI proxy 回归，确认只使用 profile `secretRef`，不转发浏览器 Authorization/cookie/local headers，缺失 env secret 的错误不泄露路径或 secret；补充 MCP stdio smoke，冻结 `tools/list` 对象 mutation 工具面，覆盖 doctor/export plan/GC dry-run/run events/index rebuild；补充本地模板草稿 run happy path，验证固定本地素材复制为 canonical artifact、run artifact ref、node state、event 和 artifact 文件读取。
- 原因：Phase 7 已铺开 CLI/serve/Web/MCP 能力，但需要在不扩大范围的前提下确认鉴权、脱敏、single-writer 和 canonical artifact ref 规则可以被持续回归。
- 验证：已运行 Docker `golang:1.25-alpine` `/usr/local/go/bin/gofmt -w internal/localworkspace/serve_test.go cmd/opsc/main_test.go cmd/opsc/mcp_test.go`；已运行 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`。`./cmd/opsc` 测试覆盖 CLI `serve` 输出脱敏、MCP stdio wrapper smoke、MCP 工具面冻结和 unhealthy doctor tool error。
- 影响：只改 local workspace 测试、README/features/local workspace contract/pending-test/todo/changelog 和项目记忆；不新增 local executor、不迁移 PDD/VPS run、不做 Full GC、不扩大 MCP 写能力、不新增 canonical object 类型。
- 风险：真实浏览器、真实模型供应商和真实 Codex / Claude Code MCP client 仍需人工回归；本轮自动化使用 httptest/provider 和本地 HTTP server。
- 后续：下一阶段优先设计真实 local workflow executor 和 project adapter，但必须继续复用 workspace core、`opsc serve` single-writer、path safety 和 redaction 规则。

## 2026-05-30 | Local Workspace Phase 7 MCP index rebuild single-writer | commit: pending

- 目标：把 MCP 中唯一会写派生状态的 `opsc_workspace_index_rebuild` 收回到 `opsc serve` single-writer 边界，避免 stdio agent 绕过本地服务直接写索引。
- 变更：`opsc_workspace_index_rebuild` 不再调用 `opsc workspace index rebuild --json`，改为要求同一 workspace 的 `opsc serve` 已启动，读取 workspace runtime state 中的相对 `bearer.token`，只允许 loopback `baseUrl`，并调用 `POST /api/local/workspace/index/rebuild`；服务未启动或 token 不可用时返回 MCP tool `isError=true`。补充测试覆盖 active serve 成功、inactive serve tool error、敏感路径/token 不泄露和工具列表不暴露对象 mutation 工具。
- 原因：MCP 是 agent 封装层，不应成为第二套 writer 或第二套 repository；index rebuild 虽然只写派生 `index.sqlite`，仍需要沿用 `opsc serve` 的鉴权、runtime 和 single-writer 语义。
- 验证：已运行 Docker `golang:1.25-alpine` `/usr/local/go/bin/gofmt -w cmd/opsc/mcp.go cmd/opsc/mcp_test.go`；已运行 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./cmd/opsc ./internal/localworkspace`。
- 影响：只改 `cmd/opsc` MCP wrapper、MCP 单测、local workspace contract、runbook、gotchas、handoff、pending manual test 和中央 Wiki；不改现有 Go API/router/service/repository/DB，不接 Web UI，不新增 canonical object 写入 MCP 工具。
- 风险：真实 Codex / Claude Code MCP client 尚未端到端回归；如果用户调用 index rebuild 前没有启动 `opsc serve`，现在会得到明确 tool error，而不是直接重建。
- 后续：下一阶段继续做真实 local workflow executor；如果未来新增 MCP 写工具，必须先复用已有 CLI/core 或 `opsc serve` API，并保持 auth/redaction/path safety/single-writer 约束。

## 2026-05-30 | Local Workspace Phase 7 MCP thin wrapper | commit: pending

- 目标：在 local workspace CLI/core 稳定后补上首版 MCP 入口，让 Codex / Claude Code 等 agent 能查询本地 workspace，同时避免在 MCP 层重复实现业务逻辑。
- 变更：新增 `opsc mcp` stdio JSON-RPC server，支持 initialize、ping、tools/list、tools/call；MCP 读取/诊断/dry-run 工具映射到现有 CLI JSON 命令，覆盖 workspace info/doctor/export plan/gc plan、template/run/artifact/profile/project/asset/prompt list 和 run status/events；`opsc_workspace_index_rebuild` 是唯一维护写工具，后续已收敛为经 active `opsc serve` 执行；run events 只暴露有限 JSONL 查询，不暴露 `--follow`。
- 原因：CLI 是 local-first 的核心接口；MCP 应作为薄封装供多 agent 接入，而不是直接读写 workspace 文件或复制 repository/service 逻辑。
- 验证：已运行 Docker `golang:1.25-alpine` `/usr/local/go/bin/gofmt -w cmd/opsc/main.go cmd/opsc/mcp.go cmd/opsc/mcp_test.go`；已运行 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./cmd/opsc ./internal/localworkspace`。
- 影响：新增 `cmd/opsc/mcp.go` 和 MCP 单测，文档更新 `docs/local-workspace-v1-contract.md`、数据分离计划、runbook、handoff、diagram 和 pending manual test；不改现有 Go API/router/service/repository/DB，不接 Web UI，不改变 `opsc serve` HTTP 行为。
- 风险：尚未在真实 Codex / Claude Code MCP client 中做端到端配置测试；首版不支持 canonical object 写入工具、canvas/workbench 工具、local executor 工具或独立 HTTP/bearer MCP adapter。
- 后续：先做真实 local workflow executor；如需要 MCP 写入工具，必须先复用已有 CLI/core 或 `opsc serve` 能力，并继续遵守 single-writer、lock、revision、path guard 和 secret redaction。

## 2026-05-30 | Local Workspace Phase 7 serve availability UX | commit: pending

- 目标：补齐 Web UI 本地工作区 bootstrap 可用性检测和未启动提示，让本地私有页面明确依赖 `opsc serve`，不回退到浏览器长期事实源。
- 变更：`local-workspace` client 新增免鉴权 `/api/health` 探针；`use-local-workspace-store` 增加 `serveAvailable`，把“服务未启动”和“服务已启动但 browser session 未建立/过期”分成不同状态；顶部本地工作区弹窗和“我的素材 / 我的提示词 / 画布库 / 工作台 / 我的工作流”等本地私有页面新增统一状态提示；文档补充 Web UI 已切换路径、legacy/cloud/VPS 边界和 health response 例外。
- 原因：浏览器存储只允许保留 cache、temporary state 和展示层补水；用户未启动 `opsc serve` 或未完成 launch secret bootstrap 时，应看到明确阻断原因，而不是误以为旧 localforage/IndexedDB 仍是事实源。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只改 Web 本地工作区连接状态、提示组件、in-scope 页面提示和文档；不改 Go `main.go`、router、service、repository、DB、`opsc serve` HTTP API、public/admin/cloud/PDD 路由语义或旧浏览器测试数据迁移策略。
- 风险：真实浏览器仍需手工回归 `opsc serve` 未启动、已启动未授权、错误/已消费 launch secret、正确 launch secret 和 session 过期场景；本轮未启动真实 `opsc serve` 做端到端手测。
- 后续：继续用真实 workspace 回归本地私有页面 CRUD、session/CORS、媒体文件回显和 legacy path 清单，再进入真实 local executor / MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 connection store migration | commit: pending

- 目标：继续收紧 Web UI 本地工作区连接状态的浏览器持久化边界，避免旧版 `opsc:local_workspace_connection` 留下 workspace/runtime metadata。
- 变更：`use-local-workspace-store` 增加 persist version migration 和 rehydrate 后的 sanitized rewrite，只从旧状态读取并规范化 loopback `baseUrl`，丢弃 `workspace`、`runtime`、`tokenFile`、`launchSecretFile`、`status` 和错误信息等会话元数据。
- 原因：browser session 应依赖 HttpOnly cookie，workspace/runtime metadata 来自 `opsc serve` 实时查询；浏览器长期状态最多记住 loopback 服务地址。
- 验证：已运行 `cd web && npx tsc --noEmit`。
- 影响：只改 Web local workspace 连接 store、文档和项目记忆；不改 `opsc serve` HTTP API、Go `main.go`、router、service、repository、DB、PDD/VPS executor 或现有 run 数据。
- 风险：真实浏览器仍需手动写入旧版 store 结构并刷新，确认存储被重写为只有 `state.baseUrl` 和版本号；若旧 baseUrl 非法，会回退到默认 `http://127.0.0.1:17680`。
- 后续：继续把真实 PDD/VPS executor、本地项目 adapter、MCP wrapper 和自动 `material_lookup` 本地素材解析纳入 local workspace 边界。

## 2026-05-30 | Local Workspace Phase 7 local projects Web UI adapter | commit: pending

- 目标：继续补齐 Web UI 通过 `opsc serve` 管理本地私有对象的缺口，把本地项目引用纳入 local workspace UI 边界。
- 变更：顶部本地工作区弹窗新增“本地项目”面板，通过 `/api/local/projects` 加载、创建、编辑和删除 `projects/<proj_id>/project.json`；列表只显示 project id、kind、adapter、capabilities 和 opaque `rootFingerprint`，不显示本机 `rootPath`；编辑时才调用 `showPaths=1` 读取路径；含 `credentialRef` 的项目不允许 Web UI 保存，避免用脱敏 summary 覆盖真实 secretRef。
- 原因：本地项目路径和 capability model 已是 local-first contract 的私有数据，但此前 Web UI 只有 typed client，没有实际入口；浏览器不能成为本地项目路径或 secretRef 的长期事实源。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只改 Web 本地工作区弹窗、文档和项目记忆；不改 `opsc serve` HTTP API、Go `main.go`、router、service、repository、DB、PDD/VPS executor 或现有 run 数据。
- 风险：真实浏览器仍需手工回归 `opsc serve` session 下项目新建/编辑/删除、绝对路径校验、fingerprint 变化和含 credentialRef 项目的只读提示；当前只是引用管理入口，还没有把 project adapter 接到真实 local executor。
- 后续：下一阶段应把 local workflow executor 与 project path capability guard 贯通，明确 PDD/local project adapter 如何用 `proj_<id>` 解析路径和权限。

## 2026-05-30 | Local Workspace Phase 7 AI profile workspace switch boundary | commit: pending

- 目标：继续收紧 Web UI 本地 AI profile 的 workspace 隔离边界，避免连接不同本地 workspace 时沿用上一 workspace 的模型配置或 `SecretRef`。
- 变更：`use-config-store` 新增 `clearLocalProfile`；加载当前 workspace profile 为空时会重置本地 AI 配置；App 启动 refresh 本地 workspace 失败或无连接时清空 profile；顶部本地工作区连接、刷新、断开时会同步加载或清空当前 workspace profile。
- 原因：AI profile canonical data 位于 local workspace，浏览器状态只能是当前会话视图；即使不落盘，也不能在切换 workspace 后继续使用上一个 workspace 的 Base URL、模型列表、默认模型或 `SecretRef`。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只改 Web local workspace 连接与 AI profile 状态边界、文档和项目记忆；不改 `opsc serve` HTTP API、Go `main.go`、router、service、repository、DB、PDD/VPS executor 或 run 数据。
- 风险：真实浏览器仍需手工回归多个 workspace 的连接/断开/刷新流程；无 profile workspace 会重置当前未保存的本地配置输入。
- 后续：真实 local executor、PDD/VPS run 数据迁移、运行时 `material_lookup` 自动匹配和 MCP wrapper 仍待下一阶段。

## 2026-05-30 | Local Workspace Phase 7 fixed material run artifact refs | commit: pending

- 目标：继续收紧本地电商 run 的素材事实源边界，让本地模板中的固定 `material_lookup` 不只停留在模板 `assetId` 引用。
- 变更：本地模板启动 run 时，会通过 `opsc serve` 读取固定本地图片素材文件，复制为全局 canonical artifact，并把 run 内 artifact ref 挂到对应素材节点；固定素材节点标记为 `success`，其它未执行节点仍保持 pending。
- 原因：run 内产物引用应遵循“全局 `artifacts/<art_id>/artifact.json` 为 canonical metadata，run 目录只保存 ref”的规则，避免后续执行器再从浏览器或模板编辑状态反查固定素材。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只改 Web local workspace client / local PDD template run adapter、文档和项目记忆；不接真实 local executor，不改 `opsc serve` HTTP API、Go `main.go`、router、service、repository、DB 或现有 PDD/VPS run。
- 风险：artifact import 与 run ref attach 是两个 HTTP 写操作，极端中断可能留下未引用 artifact，需要后续 GC/doctor 或 executor 恢复策略处理；固定素材缺失时会把对应节点标记 error，但不会把整个 run 改成 error。
- 后续：真实 local executor 仍需接入执行队列、PDD/local project adapter、自动 `material_lookup` 匹配和失败恢复语义。

## 2026-05-30 | Local Workspace Phase 7 template material asset boundary | commit: pending

- 目标：继续收紧电商私有模板编辑器的本地素材边界，避免连接 local workspace 后的本地模板仍从服务器 admin asset 接口选择固定素材。
- 变更：PDD / 电商模板编辑器的 `material_lookup` 节点在本地模板模式下改用 `useAssetStore` 的当前 workspace 图片素材列表；未连接 local workspace 的服务器模板仍沿用 `fetchAdminAssets`。
- 原因：本地私有模板中的固定素材引用应指向同一个 local workspace 的 `assets/<asset_id>/`，不能让浏览器或 VPS DB 素材库成为本地模板编辑时的事实源。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只改 Web 模板编辑器、文档和项目记忆；不改 `opsc serve` HTTP API、不改 Go `main.go`、router、service、repository、DB、PDD/VPS executor 或 run 数据。
- 风险：当前只是编辑器固定素材选择边界收口；真实 local executor 尚未接入，现有 VPS executor 仍按服务器 DB asset id 解析 `material_lookup`。
- 后续：实现 local workflow executor 时，需要明确 `material_lookup` 的 local asset id 解析、自动匹配策略和 artifact 写入规则。

## 2026-05-30 | Local Workspace Phase 7 legacy browser state purge | commit: pending

- 目标：继续收紧 Web UI 浏览器长期私有数据边界，避免旧版本测试数据继续留在 localforage `app_state` 或 `localStorage`。
- 变更：新增 `clearLegacyPrivateBrowserState` 启动清理 helper，App 初始化时会清理旧 `infinite-canvas:ai_config_store`、`infinite-canvas:asset_store`、`infinite-canvas:prompt_store`、`infinite-canvas:canvas_store`、`text_generation_logs`、`image_generation_logs`、`video_generation_logs` 和 `ops-canvas-workflow-folders`。
- 原因：这些旧 key 曾保存 AI 配置、个人素材/提示词/画布、工作台生成记录或工作流入口偏好；现阶段已确认不迁移旧浏览器测试数据，canonical data 应来自 local workspace。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只改 Web 启动清理与文档；不清理当前 `opsc:*_cache:v1` 展示缓存，不清理 `image_files/media_files` 临时上传桥接库，不改 `opsc serve` HTTP API、Go `main.go`、router、service、repository、DB、PDD/VPS executor 或 run 数据。
- 风险：真实浏览器仍需手工确认旧 key 被移除，且生成中/上传中/保存失败所需的临时媒体 Blob 不受影响。
- 后续：继续审计真实 PDD/VPS executor、PDD/VPS run 数据迁移、模板素材查找本地 asset 边界和 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 AI profile secretRef roundtrip | commit: pending

- 目标：继续收紧 Web UI 本地 AI profile 的浏览器持久化边界，修正自定义 env secret 名在保存/刷新后回退到默认值的问题。
- 变更：`use-config-store` 从 local profile 读取 AI 配置时，同时兼容完整 profile document 的 `secretRef.name` 和脱敏 summary 的 `secretRef.reference`；保存仍只写 `secretRef` 引用，不写明文 API Key。
- 原因：本地 AI profile 的 canonical data 在 local workspace，浏览器配置只能作为当前会话状态；前端需要正确往返 full document 和 sanitized summary 两种结构，不能把 env var 名解析失败后隐式改回 `OPENAI_API_KEY`。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只改 Web AI profile 读取逻辑、项目文档和项目记忆；不改 `opsc serve` HTTP API、不改 Go `main.go`、router、service、repository、DB、PDD/VPS executor 或 run 数据。
- 风险：真实浏览器仍需手工回归自定义 env var 名保存、刷新、重新连接、模型拉取和一次模型请求；本轮不验证用户本机 env/file secret 是否真实存在。
- 后续：继续审计真实 PDD/VPS executor、PDD/VPS run 数据迁移、模板素材查找本地 asset 边界和 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 asset temp media cleanup | commit: pending

- 目标：继续收紧“我的素材”导入/更新后的浏览器临时媒体缓存边界，避免已写入 `assets/<asset_id>/files/` 的图片/视频 Blob 继续留在 `image_files/media_files` 里作为隐式事实源。
- 变更：`useAssetStore` 在素材新增、更新或删除对应的 workspace 操作成功后，触发现有 `cleanupUnusedImages` / `cleanupUnusedMedia`，只保留当前素材列表、画布项目或额外上下文仍引用的 `storageKey`。
- 原因：个人素材的 canonical data 和二进制文件已经通过 `opsc serve` 写入 local workspace；浏览器媒体库只应作为上传桥接或失败未保存的临时状态。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不改 `opsc serve` HTTP API、不改 Go `main.go`、router、service、repository、DB、服务器素材库、PDD/VPS run 或画布/workbench canonical schema。
- 风险：保存失败时临时 Blob 会继续保留，方便用户重试；真实浏览器仍需回归素材上传、编辑替换、删除、复制到我的素材、素材选择器插入和画布仍引用同一 `storageKey` 时不被提前清理。
- 后续：继续审计真实 PDD/VPS executor、PDD/VPS run 数据迁移、模板素材查找本地 asset 边界和 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 canvas temp media cleanup | commit: pending

- 目标：继续收紧画布项目媒体的浏览器临时缓存边界，避免已经写入 `canvas-projects/<canvas_id>/files/` 的图片/视频 Blob 继续作为隐式事实源。
- 变更：`useCanvasStore` 在保存、导入或删除画布项目成功后，会清理本次已 canonical 化且当前画布状态不再引用的 `image:*`、`video:*`、`file:*` browser blob；如果保存期间用户继续编辑且仍引用同一个 `storageKey`，则保留该临时 Blob 到下一次安全保存。
- 原因：画布项目 JSON 和媒体文件已经由 `opsc serve` 写入 local workspace，浏览器 localforage 只能作为上传桥接、导入桥接或失败未保存状态。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不改 `opsc serve` HTTP API、不改 Go `main.go`、router、service、repository、DB、PDD/VPS run 或工作台生成记录逻辑。
- 风险：保存失败、旧 Blob 无法读取或其它未保存页面仍引用同一临时 `storageKey` 时，browser blob 可能继续短暂停留；真实浏览器需回归画布保存、导入、删除、刷新和清空旧 `image_files/media_files` 后的 workspace 文件回显。
- 后续：继续审计真实 PDD/VPS executor、PDD/VPS run 数据迁移、模板素材查找本地 asset 边界和 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 workbench reference temp cleanup | commit: pending

- 目标：继续收紧图片/视频工作台参考图的浏览器临时缓存边界，避免上传参考图在保存到 `workbench-logs/` 后继续留作隐式事实源。
- 变更：图片/视频工作台在生成记录保存成功后，把当前参考图替换为 workbench-log workspace 文件端点回显，并清理本次已 canonical 化的 `image:*` browser blob；新会话、移除参考图和切换历史记录也会清理当前参考图临时缓存；视频生成结果保存成功后会释放原临时 `blob:` URL；`cleanupUnusedMedia` 复用 `deleteStoredMedia`，清理媒体缓存时同步 revoke object URL。
- 原因：工作台记录和关联媒体已经写入 local workspace，浏览器 IndexedDB/localforage 只能作为上传桥接或生成中临时状态。
- 验证：已运行 `cd web && npx tsc --noEmit`、`git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不改 `opsc serve` HTTP API、不改 Go `main.go`、router、service、repository、DB、云端素材库/提示词库或 PDD/VPS run。
- 风险：保存失败或保存过程中用户已经切换当前参考图时，旧 browser blob 可能继续短暂停留，仍属于临时状态；真实浏览器需回归图片/视频工作台参考图上传、保存、删除、新会话和历史记录预览。
- 后续：继续审计真实 PDD/VPS executor、PDD/VPS run 数据迁移、模板素材查找本地 asset 边界和 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 browser cache hardening | commit: pending

- 目标：继续收紧 Web UI 本地私有数据边界，避免浏览器 cache 在 workspace 切换或刷新后暴露旧素材/提示词事实数据。
- 变更：`use-asset-store` 和 `use-prompt-store` 增加 `loadedWorkspaceId/workspaceLoaded` 防护，重新加载时清空持久化私有列表，页面和素材选择器只展示属于当前 workspace 的内存数据；“我的素材”导出改为从当前可读文件 URL 打包 workspace 文件，不再只依赖旧 browser `storageKey` 才能导出图片/视频；“我的素材”zip 导入改为通过 `addAsset` 写入 local workspace，不再把包内媒体恢复到浏览器 `image_files/media_files`；画布 zip 导入使用新的临时 import storageKey，并在导入后清理临时 browser blob。
- 原因：local workspace 已是素材/提示词 canonical source，浏览器 localforage 只能作为展示/临时状态，不能在多 workspace 场景中成为隐式长期事实源或泄露上一个 workspace 的私有数据。
- 验证：已运行 `cd web && npx tsc --noEmit` 和 `git diff --check`。
- 影响：不改 `opsc serve` HTTP API、不改现有 Go `main.go`、router、service、repository、DB、云端素材库/提示词库或 PDD/VPS run。
- 风险：真实浏览器仍需手工确认 workspace 切换、刷新、素材/提示词列表加载、素材选择器、“我的素材”zip 导入/导出和画布 zip 导入后的临时 blob 清理。
- 后续：继续审计真实 PDD/VPS executor、PDD/VPS run 数据迁移、模板素材查找本地 asset 边界和 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 local run/artifact adapter | commit: pending

- 目标：补齐 `opsc serve` 的 local run/artifact 写入能力，并让 Web UI 能从本地模板创建 pending run、查看本地 run 状态和 artifact 引用。
- 变更：`opsc serve` 新增 run create/get/update、event append、node state 写入、run artifact ref 写入、artifact create/get/update/import/file read；Web `local-workspace` client 增加对应 typed API；电商模板运行在连接 local workspace 时创建 `runs/<run_id>/run.json`、pending 节点状态和 `run.waiting_for_executor` 事件；`/workflows/ecommerce` 合并展示本地 runs，`run_` 路由进入本地 run 状态页并展示 nodes/events/artifacts。
- 原因：私有 workflow run 和生成 artifact 必须先具备 local workspace canonical 写入/读取路径，后续 CLI/MCP/agent executor 才能复用同一事实源。
- 验证：已运行 Docker `golang:1.25-alpine` `/usr/local/go/bin/gofmt -w internal/localworkspace/serve.go internal/localworkspace/serve_workflow_writes.go internal/localworkspace/serve_test.go` 和 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`；已运行 `git diff --check`。
- 影响：不替换现有 DB-backed workflow runs，不改现有 Go `main.go`、router、service、repository 或 PDD/VPS executor；未连接 local workspace 时服务器 run 列表和 VPS run 页面继续走原路径。
- 风险：当前本地模板运行只创建 pending run 草稿，不执行 DAG、不调用模型、不生成 artifact；真实浏览器仍需手工回归 `opsc serve` session/CORS、本地列表刷新、本地 run 状态页 artifact 文件预览。
- 后续：设计真实 local executor、PDD/local project adapter、run event 语义和 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 workflow preferences adapter | commit: pending

- 目标：移除工作流入口自定义文件夹对浏览器 `localStorage` 的事实源依赖。
- 变更：workspace manifest 增加 typed `preferences.workflowFolders`；`opsc serve` 新增 `/api/local/workspace/preferences` GET/PUT，写入走 workspace write lock、atomic JSON、revision 冲突检查和明文 secret 字段拒绝；Web `/workflows` 页面改为连接本地工作区后读取/写入 workspace preferences，未连接时不再写旧 `ops-canvas-workflow-folders` key。
- 原因：工作流入口文件夹虽然是轻量 UI 数据，但属于用户本机私有长期配置；后续 CLI/MCP/agent 使用同一 workspace 时需要看到同一事实源。
- 验证：已运行 Docker `golang:1.25-alpine` `/usr/local/go/bin/gofmt -w internal/localworkspace/...`；已运行 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`。
- 影响：不改现有 Go `main.go`、router、service、repository、DB 或 PDD/VPS run；不迁移旧浏览器测试数据；只改变 `/workflows` 页面自定义文件夹的本地事实源。
- 风险：真实浏览器仍需手工回归 `opsc serve` session/CORS、断开本地工作区时的新建提示，以及并发 revision 冲突提示。
- 后续：继续审计 runs/artifacts 写入 UI、本地模板启动 run 和 PDD/VPS run 边界。

## 2026-05-30 | Local Workspace Phase 7 local workflow templates adapter | commit: pending

- 目标：把电商工作流私有模板列表和编辑器从服务器 DB/管理员登录路径中拆出本地模式，让连接 local workspace 后的模板 CRUD 通过 `opsc serve` 读写 `templates/`。
- 变更：`opsc serve` 新增 `/api/local/templates` create/list/get/update/delete；local template summary 增加 `description`，写入前归一化 title、description、workflowType、version 和 settings；Web 新增 local workflow template adapter，电商模板列表页和编辑页在本地工作区连接状态下使用 `templates/<tpl_id>/template.json`，支持新建、复制、保存、删除和刷新回显。
- 原因：私有工作流模板属于用户本机长期数据，后续 CLI/MCP/agent 需要能直接基于 workspace 读取模板；浏览器和 VPS DB 都不应成为自用模板的唯一事实源。
- 验证：已运行 Docker `golang:1.25-alpine` `/usr/local/go/bin/gofmt -w internal/localworkspace` 和 `GOPROXY=https://goproxy.cn,direct /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`、`git diff --check`、Wiki lint、reindex 和 embed。
- 影响：不替换现有 DB-backed workflow templates/runs，不改现有 `main.go`、router、service、repository 或 PDD/VPS run 执行路径；未连接 local workspace 时模板列表和编辑器继续走原服务器模板接口。
- 风险：本地模板目前不能启动 PDD/VPS run，编辑器会禁用/拒绝本地模板运行；模板节点内的素材库查找仍沿用现有素材查询边界，未在本轮把 PDD template material lookup 全量改成本地 asset 查询。
- 后续：继续迁移 runs/artifacts 写入 UI 和本地 run executor 边界，再决定本地模板如何启动本地 run 或显式发布到服务器模板。

## 2026-05-30 | Local Workspace Phase 7 workbench preview canonicalization | commit: pending

- 目标：减少工作台图片/视频生成结果对浏览器 Blob/localforage 即时缓存的长期依赖。
- 变更：图片工作台保存生成记录成功后，会用返回的 `workbench_log` 文档把当前结果卡片中的图片 URL 替换为 `/api/local/workbench-logs/<id>/files/<media_key>` 对应的 workspace 文件 URL；视频工作台不再先把生成视频写入 `media_files`，而是用内存 `blob:` URL 做生成完成到保存成功之间的临时预览，保存成功后同样切换到 workspace 文件 URL。
- 原因：工作台生成结果是用户私有产物，保存成功后的可见结果应以 local workspace 文件为事实源；浏览器 Blob/localforage 只适合生成中、上传参考图或失败未保存状态。
- 验证：已运行 `cd web && npx tsc --noEmit`。
- 影响：不改变 `opsc serve` HTTP API、Go repository、现有 DB、PDD/VPS run 或工作流模板逻辑；真实浏览器仍需手工确认图片/视频生成后当前结果卡片、历史记录和“添加到素材”都能使用 workspace 文件 URL。
- 后续：继续迁移 runs/artifacts 写入 UI、本地模板启动 run 和 PDD/VPS run 边界。

## 2026-05-30 | Local Workspace Phase 7 AI profile proxy | commit: pending

- 目标：移除 Web AI 本地直连配置/API key 对浏览器长期存储的依赖，让模型调用也经过 `opsc serve`。
- 变更：新增 `opsc serve` `/api/local/ai/v1/*` OpenAI-compatible proxy；proxy 从 workspace profile channel 读取 `baseUrl`，通过 `secretRef` 解析 env 或绝对路径 file secret，向供应商注入 Authorization，不转发浏览器 Authorization/cookie。Web `use-config-store` 改为从 local profile 读取/保存 Base URL、模型列表、默认模型和 env `secretRef`，启动清理会移除旧 `infinite-canvas:ai_config_store`，图片、图片编辑、文本问答、视频和模型列表请求在本地模式下改走本地 proxy。
- 原因：浏览器 `localStorage` 不能成为 API key 或本地模型渠道配置的事实源；Web UI 访问本地私有配置必须通过 `opsc serve`。
- 验证：`cd web && npx tsc --noEmit`；Docker `golang:1.25-alpine` 下 `gofmt` 和 `GOPROXY=https://goproxy.cn,direct go test ./internal/localworkspace ./cmd/opsc` 通过。默认 `proxy.golang.org` 首次依赖下载 EOF 后改用备用 GOPROXY。
- 风险：真实浏览器仍需手工回归：用 `OPENAI_API_KEY=<测试 key> opsc serve --origin <Web origin>` 启动后，连接本地工作区、保存 profile、拉取模型并发起一次文本/图片/视频请求；确认 profile JSON 不含明文 key，浏览器 localStorage 不再保留旧 key。
- 后续：继续审计 templates/runs/artifacts 写入 UI、PDD/VPS run 和 MCP wrapper；工作台图片/视频预览已在后续同日条目收口到保存后回显 workspace 文件。

## 2026-05-30 | Local Workspace Phase 7 canvas zip media roundtrip | commit: pending

- 目标：补齐 Web UI 画布导入/导出 zip 对 `workspaceFileKey` 媒体文件的支持，避免画布媒体 canonical 化后导出包缺文件。
- 变更：画布导出会从 `opsc serve` 读取 `canvas-projects/<canvas_id>/files/<file_key>` 并写入 zip，同时在 `projects.json` 文件条目中记录 `workspaceFileKey`、`role`、`mimeType` 和 `bytes`；导入 zip 时将这些文件临时写入浏览器 Blob 缓存作为桥接，再通过现有 `importProject` 上传回目标 workspace。
- 原因：浏览器 Blob 不能成为长期事实源；导出包必须能完整携带 local workspace 画布媒体，导入后仍以目标 workspace 的 `files/` 为 canonical data。
- 验证：已运行 `cd web && npx tsc --noEmit`。
- 影响：只改 Web 画布导入/导出和 local workspace 前端文件读取 helper，不改现有 Go API、DB repository、PDD/VPS run 或 local workspace 后端 contract。
- 风险：真实浏览器仍需手工回归：导出含图片/视频/助手媒体的画布，清空旧 `image_files/media_files` 后重新导入并确认新 workspace 文件和预览都正常。
- 后续：AI profile/secretRef 和工作台图片/视频预览已在后续同日条目完成收口；继续审计 templates/runs/artifacts 写入 UI。

## 2026-05-30 | Local Workspace Phase 7 canvas media canonicalization | commit: pending

- 目标：把 Web UI 画布图片/视频节点和助手媒体从浏览器 `storageKey` 长期依赖迁到 local workspace，同时不改现有 DB、PDD/VPS run 或工作流结果路径。
- 变更：`canvas_project` 增加 `data.files` metadata 和项目内 `files/` 目录；`opsc serve` 的 canvas-project create/update 支持 multipart `file:<file_key>` 上传，并新增 `GET /api/local/canvas-projects/<id>/files/<file_key>`；index summary 增加 `fileCount`，doctor/GC 能检查缺失或逃逸的 canvas project file。Web `useCanvasStore` 保存时会把图片/视频节点、助手引用图和助手生成图解析为 workspace 文件，项目 JSON 只保留 `workspaceFileKey` 和轻量 metadata；画布浏览器 cache 不再持久化项目列表。
- 原因：画布媒体是自用系统的核心私有数据，不能让 IndexedDB Blob key 成为长期事实源；后续 CLI/MCP/agent 需要能从 workspace 文件目录完整读取画布项目和媒体。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`。
- 影响：旧浏览器 `storageKey` 仍作为可读取 Blob 的导入桥接；无法从旧 IndexedDB 取回的 Blob 只能保留兼容引用或人工重新导入。不替换 workflow templates/runs/artifacts、DB repository 或 PDD/VPS run。
- 风险：画布导出/导入 zip 对 `workspaceFileKey` 的完整处理仍需后续审计；真实浏览器需手工确认清空旧 `image_files/media_files` 后，新保存的画布媒体仍能从 `opsc serve` 回显。
- 后续：AI profile/secretRef、画布导入导出和工作台图片/视频预览已在后续同日条目完成收口；继续迁移 templates/runs/artifacts 写入 UI 和 PDD/VPS run。

## 2026-05-30 | Local Workspace Phase 7 workbench logs adapter | commit: pending

- 目标：把 Web UI 文本、图片、视频工作台生成记录从浏览器长期事实源迁到 local workspace，同时不替换现有 DB-backed workflow templates/runs、PDD/VPS run 或业务 API。
- 变更：新增 local `workbench_log` canonical object，写入 `workbench-logs/<wblog_id>/workbench-log.json`，关联媒体写入同目录 `files/`；补齐 index 增量更新/扫描重建、doctor 检查、GC dry-run 候选和 `opsc serve` `/api/local/workbench-logs` create/list/get/delete/file API。Web 文本、图片、视频工作台生成历史改为通过 `opsc serve` 保存和回显；旧 `text_generation_logs`、`image_generation_logs`、`video_generation_logs` 浏览器测试数据不迁移。
- 原因：工作台生成历史包含个人 prompt、参考图、生成图/视频和模型参数，是 local-first 自用系统的私有数据，应和素材/提示词/画布项目一样以用户本机 workspace 为事实源。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `/usr/local/go/bin/gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`；已运行 `git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不替换现有 `main.go`、router、service、repository、DB-backed workflow templates/runs 或 PDD/VPS run；工作台历史保存开始依赖本地 `opsc serve` session，未连接本地工作区时不会把旧浏览器 generation log 当事实源。
- 风险：真实浏览器下的 session/CORS、文件回显和删除目录行为仍需手工回归；画布媒体与工作台图片/视频预览已在后续同日条目完成 workspace 文件回显收口。
- 后续：AI profile/secretRef、画布媒体和工作台图片/视频预览已在后续同日条目完成收口；继续迁移 templates/runs/artifacts 写入 UI，再封装 MCP/agent 入口。

## 2026-05-30 | Local Workspace Phase 7 canvas projects adapter | commit: pending

- 目标：把 Web UI 画布项目库从浏览器长期事实源迁到 local workspace，同时保持现有 Go API、DB、PDD/VPS run 和 cloud-backed 数据路径不变。
- 变更：新增 local `canvas_project` canonical object，写入 `canvas-projects/<canvas_id>/canvas-project.json`，保存画布节点、连线、聊天会话、背景和 viewport；补齐 index 增量更新/扫描重建、doctor 检查和 `opsc serve` `/api/local/canvas-projects` create/list/get/update/delete API。Web `useCanvasStore` 改为通过 `opsc serve` 加载、创建、重命名、删除和导入画布项目，浏览器只保留 `opsc:canvas_store_cache:v1` 展示缓存，旧 `infinite-canvas:canvas_store` 不迁移。
- 原因：画布项目是自用系统的核心私有数据，应和素材/提示词一样以用户本机 workspace 为事实源，后续 CLI/MCP/agent 才能稳定读取和管理。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`；已运行 `git diff --check`；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不替换现有 `main.go`、router、service、repository、DB-backed workflow templates/runs；不迁移旧浏览器测试数据或现有 PDD VPS run。画布节点中的 `storageKey` 仍可能指向浏览器 Blob 缓存，不等同于 workspace artifact ID。
- 风险：真实浏览器 session/CORS、画布导入导出和已有媒体节点回显仍需人工回归；templates/runs/artifacts 写入 UI 和 PDD/VPS run 尚未迁移。工作台生成记录、画布媒体 canonical 化和 AI profile/proxy 已在后续同日条目完成。
- 后续：继续迁移 templates/runs/artifacts 写入 UI 和 PDD/VPS run，再接 MCP wrapper。

## 2026-05-30 | Local Workspace Phase 7 Web UI assets/prompts adapter | commit: pending

- 目标：让 Web UI 的本地私有素材和提示词先通过 `opsc serve` 读写 local workspace，把浏览器持久化降级为缓存。
- 变更：新增前端 `local-workspace` typed client、`use-local-workspace-store` 和顶部本地工作区连接入口；browser 只保存 loopback `baseUrl`，用一次性 `launch.secret` 换 HttpOnly session，不保存 bearer token 或 launch secret。`use-asset-store`、`use-prompt-store` 改为通过 `opsc serve` 加载、创建、更新、删除；旧浏览器 key 不迁移，新 key 只作为 `opsc:*_cache:v1` 展示缓存。素材中心、提示词中心、素材选择器、画布/工作台/PDD 创作画布“存素材”路径已改为等待 workspace 写入结果。
- 后端：`opsc serve` 补齐 profiles/projects/assets/prompts 的 delete API，asset 增加 multipart import/update import，文件写入 `assets/<asset_id>/files/`，并通过 atomic write、workspace lock、revision 和 index 更新保持 single-writer 语义。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc`；已运行 `cd web && npx tsc --noEmit`；`git diff --check` 通过；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：现有云端素材库/提示词库、Go `main.go`、router、service、repository 和 DB 行为不替换；Web UI `我的素材` / `我的提示词` 的 canonical data 开始依赖本地 `opsc serve` session。
- 风险：templates/runs/artifacts 写入 UI 和 PDD/VPS run 仍未迁移；真实浏览器 cookie/CORS 需要在 `opsc serve --origin <Web origin>` 下手工回归，尤其 `localhost` 与 `127.0.0.1` 混用时的 SameSite cookie 行为。工作台生成记录、画布媒体 canonical 化和 AI profile/proxy 已在后续同日条目完成。
- 后续：继续迁移 templates/runs/artifacts 写入 UI 和 PDD/VPS run，再封装 MCP/agent 入口。

## 2026-05-30 | Local Workspace Phase 6 opsc serve session and write API | commit: pending

- 目标：在不改现有 `main.go`、router、service、repository 和 DB 行为的前提下，把 `opsc serve` 升级为后续 Web UI/MCP 的本地安全入口。
- 变更：`opsc serve` runtime/state 从 workspace `.opsc/runtime` 迁到 `$XDG_STATE_HOME/opsc/workspaces/<workspaceId>-<rootHash>/`，保留 CLI/MCP `bearer.token`，新增每次启动的一次性 `launch.secret` 交换 HttpOnly session cookie；带 `Origin` 的浏览器请求只接受 session，不接受 bearer；HTTP API 改为 `{code,data,msg}` envelope，并新增 `/api/local/*` 路由下的 profiles/projects/assets/prompts 查询和 create/update。workspace 写操作统一走 localworkspace core 的 write lock、atomic write、revision 检查、path guard 和 secret 脱敏。
- 原因：浏览器不应长期保存 bearer token，也不应直接写 `~/OpsCanvas`；local service 需要提供可并发调用的 single-writer 入口，并保持与现有 Go API response 约定接近，降低后续 Web UI adapter 成本。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc`；首次 `go test ./...` 因 `proxy.golang.org` 依赖下载 EOF 失败，改用 `GOPROXY=https://goproxy.cn,direct` 后 `go test ./...` 通过；`git diff --check` 通过；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：只新增/修改 `cmd/opsc`、`internal/localworkspace` 和 local workspace 文档/项目记忆；不接现有 Web UI，不新增现有 Go router 路由，不替换 DB repository，不迁移旧浏览器测试数据或现有 PDD VPS run。
- 风险：browser session bootstrap 仍未接真实 Web UI，写入 API 只覆盖 profiles/projects/assets/prompts 这四类本地对象；template/run/artifact 写入仍通过 repository/core 测试覆盖，尚未暴露写入 HTTP API。
- 后续：实现 Web UI local workspace adapter，并在真实浏览器流程中验证 launch secret、session cookie、CORS 和对象写入；之后再按同一 HTTP/CLI core 封装 MCP。

## 2026-05-29 | Local Workspace Phase 5 opsc serve loopback API | commit: pending

- 目标：实现 `opsc serve`，让 Web UI/MCP 后续访问 local workspace 时有统一的本机 HTTP 入口。
- 变更：新增 `internal/localworkspace.Serve`，默认监听 `127.0.0.1:17680`，支持 `--port 0`、bearer token、`.opsc/runtime/serve.{json,pid,port,token}`、`.opsc/locks/serve.lock`、CORS local origin 白名单、`/health` 免鉴权、workspace 查询/doctor/index rebuild/export plan/GC plan、template/run/artifact/profile/project/asset/prompt summaries、run events/SSE、artifact/asset 文件读取和 prompt content 读取；`cmd/opsc` 新增 `serve` 命令和相关测试。
- 原因：后续 Web UI 本地化、MCP wrapper 和多 agent 接入不能直接读写 `~/OpsCanvas`，需要先把 token、runtime metadata、CORS、文件读取和统一 response envelope 稳定下来。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc` 和 `go test ./...`；已构建临时 `opsc` 二进制 smoke `workspace init` + `serve --port 0`，验证 `/health`、bearer token 调 `/api/workspace/info`、runtime/JSON 输出脱敏和 SIGTERM 后清理 runtime/lock；`git diff --check` 通过，业务路径 guard 确认未改 `main.go`、router、service、repository、DB/model、handler、config 或 web。
- 影响：不接现有 Web UI，不新增现有 Go HTTP API，不替换 DB repository，不迁移旧浏览器测试数据或现有 PDD VPS run。
- 风险：当前 `opsc serve` 只覆盖读取、诊断和计划类 API，尚未提供写入型业务 CRUD；Web UI/MCP 还未接入，因此“唯一入口”目前是 local workspace 访问约束和服务能力，不是现有 UI 的实际数据路径。
- 后续：补 Web UI 本地 workspace adapter，使浏览器只通过 `opsc serve` 访问本地数据；再按同一 HTTP/CLI core 封装 MCP。

## 2026-05-29 | Local Workspace Phase 5 project safety and GC dry-run | commit: pending

- 目标：在不接 UI、不改现有 DB/API 的前提下，补齐 local workspace profiles/projects/assets/prompts 的隐私、安全和清理边界。
- 变更：project 新增 workspace-local salted `rootFingerprint`、`adapterMetadata`、`credentialRefs`、path capability guard 和默认 deny globs；`secretRef` 新增脱敏 summary，file secret 只显示 `"<file>"`；assets/prompts 本地对象字段对齐现有素材/提示词常用分类维度；`workspace doctor` 增加 project fingerprint、execution policy、credentialRef 和 GC dry-run 规则检查；新增 `BuildGCPlan` 和 `opsc workspace gc plan`，只输出相对路径 candidates，动作为 `review`，不删除文件；index summary 覆盖新增筛选字段。
- 原因：后续 CLI、`opsc serve`、MCP 和 Web UI 本地化需要先保证 project root 不泄露、不逃逸，secrets 不进入普通 JSON/默认输出，清理策略先可观察再执行。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `gofmt -w internal/localworkspace cmd/opsc`、`go test ./internal/localworkspace ./cmd/opsc` 和 `go test ./...`；已用临时 workspace 烟测 `workspace init/info/doctor/index rebuild/export plan/gc plan` 以及 profile/project/asset/prompt 查询 CLI；`git diff --check` 通过，业务路径 guard 确认未改 `main.go`、router、service、repository、DB/model、handler、config 或 web。
- 影响：不接 UI，不新增 HTTP API，不替换现有 DB repository，不迁移旧浏览器测试数据和现有 PDD VPS run。
- 风险：`workspace gc plan` 当前只做 dry-run，不提供删除；project path guard 是 foundation API，尚未被业务 adapter 调用。
- 后续：实现 `opsc serve` 本地 API，或补写入型 CLI/adapter 让 project capability guard 成为实际执行路径的一部分。

## 2026-05-29 | Local Workspace Phase 4 profile/project/asset/prompt repository | commit: pending

- 目标：完成 local workspace Phase 4，把 profiles、projects、assets、prompts 的本地私有存储边界、索引、doctor 和 export plan 规则落地。
- 变更：新增 profile/project/asset/prompt typed repository、summary DTO、校验和 `index.sqlite` 增量/重建索引；profile 只允许 `secretRef`，拒绝明文 secret 字段；project summary 不暴露 `rootPath`；asset 校验 source artifact ref 和相对文件路径；prompt 使用 `prompt.json` + `content.md`；新增 `workspace export plan` 默认排除 `.opsc/`、`cache/`、`exports/`、`index.sqlite`、本地 project path document、`secretRef.type=file` document 和 symlink；`workspace doctor` 补齐 asset file、prompt content、project rootPath、export rules 检查；`cmd/opsc` 新增 `profile list`、`project list`、`asset list`、`prompt list`、`workspace export plan`。
- 原因：后续 `opsc serve`、Web UI 本地化、MCP 和多 agent 接入需要先把本地私有配置、项目引用、素材和 prompt 的文件边界稳定下来。
- 验证：已运行 Docker `gofmt`；`go test ./internal/localworkspace ./cmd/opsc` 和 `go test ./...` 通过；已用 `go run ./cmd/opsc` 烟测空 workspace 的 `workspace init/info/doctor/index rebuild/export plan`、`profile list`、`project list`、`asset list`、`prompt list` JSON 输出；`git diff --check` 通过；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不接 UI，不新增 HTTP API，不替换现有 DB repository，不迁移旧浏览器测试数据和现有 PDD VPS run。
- 风险：当前仍只有只读查询 CLI 和 repository API，没有写入型业务 CLI；`index.sqlite` 仍是派生缓存，若 index 更新失败需运行 `workspace index rebuild`。
- 后续：实现 `opsc serve` runtime/token/API，或先补写入型 `opsc profile/project/template/run/artifact` CLI。

## 2026-05-29 | Local Workspace Phase 3 template/run/artifact repository | commit: pending

- 目标：完成 local workspace Phase 3，把 templates、runs、artifacts 的本地 file-backed 持久化和最小 CLI 查询能力做出来。
- 变更：新增 template/run/artifact typed repository、run artifact ref 写入与读取、run node state、append-only run events、artifact 文件相对路径校验、run status 校验和 summary DTO；`index.sqlite` 落地为可重建派生索引，增量记录 templates/runs/artifacts/run refs/node states/events，并支持扫描 canonical files 重建；`cmd/opsc` 新增 `workspace index rebuild`、`template list`、`run list`、`run status`、`run events`、`run events --follow`、`artifact list`、`artifact list --run`。
- 原因：后续 `opsc serve`、Web UI 本地化、MCP 和多 agent 接入需要先能通过同一套 local workspace core 读取本地模板、运行和产物索引。
- 验证：已运行 Docker `gofmt`；`go test ./cmd/opsc ./internal/localworkspace` 通过；`go test ./...` 通过；已用 `go run ./cmd/opsc` 烟测空 workspace 的 `workspace index rebuild`、`template list`、`run list` 和 `artifact list` JSON 输出；`git diff --check` 通过；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不接 UI，不新增 HTTP API，不替换现有 DB repository，不迁移旧浏览器测试数据或现有 PDD VPS run。
- 风险：`index.sqlite` 是派生缓存，若 canonical 写入后 index 更新失败，需要通过 `workspace index rebuild` 修复；`opsc serve`、Web UI 本地化、MCP、写入型 CLI 和业务迁移仍未实现。
- 后续：实现 `opsc serve` runtime/token/API 前，可以先补 template/run/artifact 写入命令或最小本机 HTTP 读取 API。

## 2026-05-29 | Local Workspace Phase 1 foundation 加固 | commit: pending

- 目标：在不改变现有 `main.go`、router、service、repository 和 DB 行为的前提下，补齐 local workspace foundation 的安全与索引边界。
- 变更：新增 `SecretRef` 结构校验，明确只保存 `env`、`keychain`、`file`、`cloud` 引用；新增 `WorkspaceManifest` 命名、workspace scan、`IndexRebuilder` / `RebuildIndex` Go interface 和 `NoopIndexRebuilder`；加强 repository path component、object id 和 ID prefix 校验；`workspace doctor` 补齐 schema/目录/manifest/lock/index/broken refs/secretRef/project root 诊断，并改为 human report 输出 stderr、`--json` 输出 stdout machine report；补充 path escaping、atomic write 失败不覆盖、lock、ID、schema、scan、rebuild 和 `opsc workspace` stdout/stderr 约定单测。
- 原因：后续 template/run/artifact 本地 repository、`opsc serve`、CLI/MCP 和多 agent 接入需要复用同一套 local-first 文件安全、对象 envelope 和可重建索引边界。
- 验证：已运行 `gofmt`；`go test ./internal/localworkspace ./cmd/opsc` 通过；`go test ./...` 通过。
- 影响：不接 UI，不新增 HTTP API，不替换现有 DB repository；`workspace index rebuild` CLI、sqlite index schema、`opsc serve` 和业务数据迁移仍未实现。
- 风险：当前 scan/rebuild 只是 foundation Go interface，尚未被业务路径调用；真实 index sqlite 写入策略需要下一阶段单独设计和测试。
- 后续：实现 template/run/artifact file-backed repository 和只读 CLI，再实现 `opsc serve` runtime/token/API；doctor 目前只做结构与引用检查，不解析真实 secret、不验证模型供应商凭据。

## 2026-05-29 | Local Workspace Phase 1 foundation | commit: pending

- 目标：实现 local workspace foundation 层，为后续 CLI、`opsc serve` 和 file-backed repository 提供统一底座。
- 变更：新增 `internal/localworkspace`，覆盖 workspace 路径解析、`opsc-workspace.json` envelope、ULID、目录初始化、`index.sqlite` placeholder、atomic JSON 写入、`.opsc/locks/workspace.lock`、doctor report、runtime metadata 读取和泛型 JSON object repository。新增 `cmd/opsc`，支持 `opsc workspace init/info/doctor`、`--workspace`、`OPSC_WORKSPACE`、`--json`、`--show-paths` 和 JSON success/error envelope。
- 原因：后续本地模板、run、artifact、`opsc serve`、Web UI 本地化和 MCP 都需要复用同一套 workspace contract，避免各自直接写文件。
- 验证：已用 Docker `golang:1.25-alpine` 执行 `gofmt`；`go test ./internal/localworkspace ./cmd/opsc` 通过；已用 `go run ./cmd/opsc` 在临时 workspace 烟测 `workspace init/info/doctor --json`；挂载 `/tmp` Go module/build cache 后 `go test ./...` 通过。
- 影响：不迁移旧浏览器测试数据，不迁移现有 PDD VPS run，不改变现有 Go API、DB、PDD/VPS 或前端业务路径。
- 风险：`index.sqlite` 目前只是 placeholder，尚未实现可重建索引；`opsc serve`、MCP、本地模板/run/artifact repository 和 Web UI 本地化仍未实现。
- 后续：实现本地 template/run/artifact repository 和列表命令，再实现 `opsc serve` 的本机 HTTP API、token/runtime 文件和 Web UI 接入。

## 2026-05-29 | Local Workspace v1 contract 定稿与补全 | commit: pending

- 目标：完成 Phase 0，把 local-first 数据分离、workspace 目录、对象 ID、canonical 文件、CLI 输出、`opsc serve`、云端边界和隐私规则定稿为可执行 contract，并基于现有 DB、设置、画布、本地 agent、路由和配置文档补齐契约细节。
- 变更：新增并补全 `docs/local-workspace-v1-contract.md`；更新 `docs/local-workspace-data-separation-plan.md` 指向正式 contract；新增 ADR-0002 和 `local-workspace-v1.mmd`，并更新 architecture、project card、gotchas 和 handoff。Contract 现已覆盖 `opsc serve` runtime metadata、token/port/pid 文件、object envelope、revision、event envelope、artifact metadata、run artifact ref、`secretRef`、project capability、atomic write、lock、index rebuild、cache/export 排除规则和现有 DB/VPS/browser 兼容边界。
- 原因：后续要先做好自用 local-first 系统，再通过同一套 CLI/core 支撑 Web UI、MCP、多 agent 和商用云端 profile，需要把数据归属和接口契约固定下来。
- 验证：仅文档变更；已运行 `git diff --check`，并检查 diff 范围只包含 Markdown/Mermaid 文档；中央 Wiki 已运行 `lint_wiki.sh`、`reindex_qmd.sh llm-wiki` 和 `qmd embed`。
- 影响：不修改业务代码；当前代码尚未实现 `opsc`、`opsc serve` 或 `~/OpsCanvas`。
- 后续：进入 Phase 1 时先实现 `opsc workspace init/info/doctor`，再实现本地模板、run、artifact 读写；`opsc serve` 和 MCP 放到 CLI/core 稳定之后。

## 2026-05-29 | PDD 创作画布本地状态优先与工具栏修复 | commit: pending

- 目标：修复电商工作流结果页“创作画布”缩放抽搐、工具栏操作回退、替换/裁剪/多角度结果丢失或布局跳变的问题。
- 变更：前端创作画布改为首次完整 hydrate，后续轮询只做非破坏合并，不再因 `updatedAt` 重置节点、选区、history 或 viewport；保存逻辑改为串行保存最新 refs，避免保存中产生的后续状态丢失。替换图片/视频会清理旧生成 metadata 并标记 `source=user_upload`；锁比例/自由比例对齐原生画布；裁剪和多角度增加提交 loading，并先保留结果节点再上传/生成；悬浮工具栏按实际尺寸夹在视口内，顶部空间不足时改到节点下方或视口边缘。
- 后端：`mergeCreativeMetadata` 不再用 live workflow metadata 覆盖用户上传、creative 派生节点或已保存的 `prompt/model/size/quality/count` 等配置；旧保存画布若只有 run 派生节点且实际矩形重叠，会在 live merge 后自动重排一次，给真实媒体尺寸留出间距；新增单测覆盖用户上传不被覆盖、原始 run artifact 可刷新但配置不回退和自动重排触发条件。
- 验证：已运行 `cd web && npx tsc --noEmit`；已用 Docker `golang:1.25-alpine` 执行 `gofmt` 和 `go test ./...`；`git diff --check` 通过。VPS 已同步并执行 `docker compose -f docker-compose.pdd-console.yml up -d --build app`，`/api/health` 与 `/workflows/pdd` 健康检查通过。Playwright 已在 `custom_20260529_073257` 回归：8 个节点无重叠、5 张图片加载完成、工具栏从节点移动到按钮后仍可点击、缩放后等待轮询最大位移为 0。
- 风险：真实多角度生成和“应用副本并重跑下游”会消耗模型额度或改写工作流产物，本轮自动验证默认不发起真实模型生成/重跑；需要在浏览器中用实际 run 手工确认这些会产生新产物的交互。

## 2026-05-29 | PDD 创作画布工具补齐与同步失败状态收敛 | commit: pending

- 目标：继续优化电商工作流运行结果页“创作画布”，补齐原生画布工具栏能力，并排查 `custom_20260529_073257` 长时间显示 running 的原因。
- 变更：结果页创作画布小地图新增紧凑节点标识；新增/生成/裁剪/多角度节点使用节点外框碰撞检测寻找空位；悬浮工具栏沿用原生画布延迟隐藏逻辑；接入文本编辑/文本生图/图片视频替换/下载/存素材/锁比例/裁剪/多角度/重试/编辑面板/应用副本重跑等动作。后端新增创作画布节点输出应用接口，可把副本接管原工作流节点输出并按模板 DAG 重跑下游。
- 排查：VPS 上 `custom_20260529_073257` 实际已失败，失败点是 `sync_local` 脚本连接 `127.0.0.1:22222` 被重置，推断为本地反向 SSH 通道不可用；未发现仍在执行的 run 进程。
- 修复：`sync_local` 前增加反向 SSH 端口预检和清晰错误信息；自定义 run 顶层状态增加收敛保护，若无 running 商品但存在失败商品，不再继续显示 running。
- 验证：本地 `npx tsc --noEmit` 通过；本地 Docker `golang:1.25-alpine` 执行 `gofmt` 和 `go test ./...` 通过；`git diff --check` 通过。VPS `/opt/ops-canvas-console` 已同步本轮相关文件，远端 gofmt、compose 重建启动完成，API `/api/health` 返回 `ok`，Next `/workflows/pdd` 返回 `HTTP/1.1 200 OK`。
- 风险：工具栏生成、裁剪、多角度、应用重跑涉及真实模型调用和工作流产物重写，仍需要浏览器人工回归；`sync_local` 仍要求用户先建立反向 SSH，未建立时按需求失败但提示更明确。

## 2026-05-29 | PDD 创作画布与模板重试配置 | commit: pending

- 目标：按当前需求优化电商工作流运行结果页的创作画布，并为 PDD 工作流模板模型调用节点增加失败重试配置。
- 变更：结果页创作画布移除左上角说明气泡，连接线恢复原生贝塞尔曲线，媒体节点增加可选节点框并按实际媒体尺寸适配；模板编辑器为 `text_generation`、`image_generation`、`image_edit`、`video_generation` 增加 `retry` 配置；后端模型节点执行和 guardrail 瞬时请求统一为 `retryCount=0` 无限重试、`intervalSeconds=0` 系统退避。
- 影响：PDD custom workflow 的结果展示、模板保存和运行执行语义变化；原生画布默认媒体节点样式不变，只有结果页传入 `mediaFrame`。
- 验证：本机无 `gofmt`，已用本地 Docker `golang:1.25-alpine` 执行 `gofmt` 和 `go test ./...`；已运行 `git diff --check`。VPS `/opt/ops-canvas-console` 已同步本轮相关文件并通过 compose 重建启动，`/api/health` 和 `/workflows/pdd` 本机健康检查通过。
- 风险：当前工作区仍包含大量既有未提交改动，diff 中会混入用户已有文件状态；重试次数 0 会按需求一直重试，遇到长期上游故障会让相关模型节点持续等待。

## 2026-05-29 | 初始化项目记忆基线 | commit: pending

- 目标：为既有仓库建立 `docs/ai/` 持久项目记忆，便于后续 AI/Codex 任务先读项目事实、架构边界和运行方式。
- 变更：新增 `project-card.md`、`architecture.md`、`runbook.md`、`handoff.md`、`gotchas.md`、Mermaid 架构/流程图和 `ADR-0001-durable-project-memory.md`。
- 原因：当前仓库已经包含 Next.js + Go 控制台、浏览器本地画布状态、后端模型代理、PDD workflow、local agent、Docker/Render/GHCR 部署路径，需要把这些当前事实集中保存。
- 验证：已运行 `git diff --check -- docs/ai`、Wiki lint、`reindex_qmd.sh llm-wiki` 和 `qmd embed`；具体结果见本次最终回复和 `handoff.md`。
- 影响：不修改业务代码；后续任务可从 `docs/ai/project-card.md`、`docs/ai/architecture.md` 和中央 LLM Wiki 项目实体快速定位项目上下文。
- 风险：初始化基线来自当前工作区，包括大量已有未提交业务改动；它记录“当前工作区事实”，不等同于最新发布 tag。
- 后续：请人工确认 product name、真实部署拓扑、PDD VPS 状态、生产数据库/持久化策略和模型渠道边界。

# ADR-0002 Local Workspace v1 Contract

## Context

当前项目同时存在浏览器 IndexedDB/localforage、本地 VPS/服务端文件目录、服务端数据库和 PDD workflow run 目录。用户计划先把自用 local-first 系统做好，再扩展 CLI、MCP、agent 接入和后续商用云端能力。继续把私有模板、run 结果、个人素材、个人 prompt 和项目路径混在云端/VPS 或浏览器私有存储中，会限制 CLI/MCP 复用，也会带来数据隐私和迁移风险。

## Decision

接受 `docs/local-workspace-v1-contract.md` 作为 Phase 0 设计 contract。

核心决策：

- 默认 workspace 为 `~/OpsCanvas`，并支持 `--workspace` 和 `OPSC_WORKSPACE` 指定多个 workspace。
- JSON、JSONL 和文件目录是事实源，`index.sqlite` 只是可重建索引。
- 私有模板、run、artifact、个人素材、个人 prompt、画布项目、工作台生成记录、本地项目路径和本地日志默认保存在 local workspace。
- 云端只保存账号、授权、计费、官方/公共模板、公共素材，以及商用 profile 所需的模型中转、限流和审计。
- 旧浏览器测试数据不迁移；浏览器 IndexedDB/localforage 只作为 UI 缓存或临时状态。
- 现有 PDD VPS run 不迁移；后续按 workspace adapter 重新接入。
- CLI `opsc` 是核心接口；`opsc serve` 是 Web UI 访问本地数据的唯一 HTTP 入口；`opsc mcp` 是 agent 的 stdio 查询/低风险维护入口，MCP 封装 CLI/core 或 `opsc serve`，不新增事实源。
- `opsc serve` 使用 workspace 外 XDG state 目录保存 runtime metadata、pid、port、`bearer.token`、一次性 `launch.secret`、session 和 lock；browser 使用 launch secret 换 HttpOnly session，CLI 或显式 HTTP client 可使用 bearer token，且两者都与现有 `LOCAL_AGENT_TOKEN` 分离。Phase 7 `opsc mcp` 的读取、诊断和 dry-run 工具通过 stdio 包装现有 CLI JSON 命令；唯一维护写工具 `opsc_workspace_index_rebuild` 通过 active loopback `opsc serve` 读取 runtime `bearer.token` 并调用本地 API，只重建派生 `index.sqlite`。
- Canonical JSON 使用统一 object envelope，包含 `schemaVersion`、`kind`、`id`、`revision`、`createdAt`、`updatedAt` 和 `data`；run event 使用 append-only JSONL event envelope。
- Profile channel 使用 `secretRef` 引用 secret，不复制现有服务端 `settings.private.channels[].apiKey` 明文。
- Project 使用 capability model 控制 `fs.read`、`fs.write`、`process.exec`、`network.local`、`artifact.write`；`process.exec` 的路径安全边界对齐当前 `cmd/local-agent`。Project root 额外保存 workspace-local salted opaque `rootFingerprint`，salt 位于 `.opsc/project-root.salt` 并排除 export。
- 写入通过 atomic rename 和 workspace lock；index rebuild 跳过 `.opsc/`、`cache/`、`exports/`；export/share/publish 默认排除 runtime、cache、exports、secrets 和本地绝对路径。
- 删除/清理策略先采用 GC dry-run plan，只返回候选项和相对路径，不直接删除 canonical object 或文件。

## Consequences

- 当前已建立 local workspace core、file-backed template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log repository、run events、project path guard、可重建 `index.sqlite` 查询索引、默认 export plan、GC dry-run plan、`opsc serve` 本机 loopback API 和 `opsc mcp` stdio 薄封装；profiles/projects/assets/prompts/canvas-projects/workbench-logs 已有本地查询与写入 HTTP API。后续仍需实现真实 local executor 和 canonical object 写入型 MCP 工具。
- CLI 输出、HTTP `{code,data,msg}` envelope、对象 ID、object envelope、目录结构、隐私边界、`opsc serve` runtime metadata 和 token/session 机制必须按 contract 保持稳定。
- 云端功能需要显式 `publish`、`upload`、`share` 或 `sync --cloud`，不得隐式上传私有数据。
- 当前 `我的素材`、`我的提示词`、画布项目库和工作台 text/image/video 生成记录已接入 local workspace；现有 Go API、DB repository、PDD/VPS run、AI 本地配置和 templates/runs/artifacts 写入 UI 仍保持原边界。

## Alternatives Considered

- 继续使用浏览器 IndexedDB 作为事实源：不利于 CLI、MCP、agent 和本地项目管理。
- 让 VPS 只做 API 中转但仍保存 run/artifact：隐私边界不清晰，也不适合本地项目自用。
- 同时设计两套 CLI：会分裂自用版和商用版能力；已决定使用一套 CLI，通过 profile 区分 local/cloud/hybrid。
- 先做 MCP：MCP 是 agent 适配层，不应先于稳定 CLI/core。

## Status

Accepted. Phase 6 foundation and `opsc serve` loopback API are implemented in `internal/localworkspace` and `cmd/opsc`; profiles/projects/assets/prompts/canvas-projects/workbench-logs local write API exists. Phase 7 has integrated Web UI `我的素材`、`我的提示词`、canvas project library, text/image/video workbench histories, AI local profile/proxy, local workflow templates, workspace preferences, local run/artifact foundation and first `opsc mcp` read/diagnostic wrapper; `opsc_workspace_index_rebuild` now uses active `opsc serve` as its single-writer path. Old data migration, real local executor and canonical object write-capable MCP tools remain future work.

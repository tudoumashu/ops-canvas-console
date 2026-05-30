# Local Workspace 数据分离计划

本文档记录 local-first 数据分离的已确认决策和后续落地顺序。目标是把私有模板、运行结果、个人素材、个人 prompt、生成产物和本地项目引用从云端/VPS 存储边界中拆出来，形成后续 CLI、MCP、Web UI 都能复用的本地 workspace 底座。

Phase 0 的正式设计 contract 已定稿并补全在 [`docs/local-workspace-v1-contract.md`](./local-workspace-v1-contract.md)。后续实现 local workspace、CLI、`opsc serve`、Web UI 本地化和 MCP 封装时，以 contract 文档为准；本文保留为数据分离路线说明。

Phase 6 foundation 已继续落地：`internal/localworkspace` 提供路径解析、workspace 初始化/打开/诊断、manifest/envelope、ULID、`secretRef` 结构校验与脱敏 summary、atomic JSON/文件写入、lock、泛型 JSON object repository、workspace scan、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log typed repository、run artifact ref、run node state、append-only run events、project root salted fingerprint、project path capability guard、`index.sqlite` 增量更新和扫描重建、默认 export plan、GC dry-run plan，以及 `opsc serve` 本机 loopback HTTP API；`cmd/opsc` 提供 `opsc workspace init/info/doctor`、`opsc workspace index rebuild`、`opsc workspace export plan`、`opsc workspace gc plan`、`opsc template list`、`opsc run list/status/events`、`opsc artifact list`、`opsc profile list`、`opsc project list`、`opsc asset list`、`opsc prompt list` 和 `opsc serve`。当前 `opsc serve` 使用 workspace 外 XDG state runtime metadata、HTTP bearer token、browser 一次性 launch secret + HttpOnly session，并已提供 profiles/projects/assets/prompts/canvas-projects/workbench-logs 查询、create/delete、对象写入、文件导入 API 和 `/api/local/ai/v1/*` 本地 AI proxy。

Phase 7 Web UI adapter 已部分落地：顶部导航可用一次性 `launch.secret` 连接 `opsc serve`，`我的素材`、`我的提示词`、画布项目库、画布项目内图片/视频媒体、画布导入/导出 zip 媒体文件、工作台 text/image/video 生成记录、AI 本地 profile、本地项目引用、电商工作流私有模板、模板 `material_lookup` 固定素材选择、工作流入口自定义文件夹和本地 run/artifact 基础查询写入已改为通过本地服务读写 local workspace；电商模板在连接本地工作区时可创建本地 run 草稿并进入本地 run 状态页。本地工作区弹窗内的“本地项目”面板通过 `/api/local/projects` 创建/编辑/删除 `projects/<proj_id>/project.json`，列表不展示 `rootPath`，编辑时才显式读取路径，含 `credentialRef` 的项目不在 Web UI 写回以避免用脱敏摘要覆盖真实 secretRef。“我的素材”新增/更新/删除成功后会清理当前素材和画布都不再引用的 browser media blob；画布保存成功后会清理已写入 workspace 且当前画布状态不再引用的 browser media blob；图片/视频工作台结果和参考图保存成功后会从 workspace workbench-log 文件端点回显，保存成功、新会话、移除参考图或切换历史记录时会清理已 canonical 化的 browser reference blob；浏览器 localforage/localStorage 只保留新 key 下的 UI cache、loopback `baseUrl` 或临时媒体状态，素材/提示词 cache 不持久化私有列表或正文，本地工作区连接 store 已加版本迁移，旧 `workspace/runtime` 持久化字段会在刷新后降级为仅保留 `baseUrl`；AI 本地 profile 读取兼容完整 `secretRef.name` 和脱敏 summary `secretRef.reference`，连接/刷新/断开 local workspace 时会按当前 workspace 加载或清空本地 profile，浏览器不保存明文 API Key；素材包导入/导出围绕 workspace 文件端点，画布 zip 导入完成后清理临时 import blob；Web UI 启动时会清理旧 AI config、旧素材/提示词/画布 store、旧 generation log 和旧 workflow folders 这些 legacy private keys，不迁移旧浏览器测试数据。本轮新增 `opsc mcp` stdio 薄封装，查询/诊断/dry-run 工具调用既有 CLI JSON 命令，唯一维护写工具 `opsc_workspace_index_rebuild` 必须通过 active `opsc serve` loopback API 重建派生索引，不直接读写 workspace repository。当前仍不迁移现有 PDD VPS run；本地 run 还没有接入真实 PDD/VPS executor，PDD run 数据仍未迁到 local workspace，运行时 `material_lookup` local asset id 解析仍待实现。

## 已确认决策

- 默认 workspace 根目录为 `~/OpsCanvas`。
- 支持多个 workspace，CLI 通过 `--workspace` 指定目标目录。
- 项目文件不复制进 workspace，只保存外部路径、类型、权限和 adapter metadata。
- 生成 artifact 复制进 workspace，由系统统一管理。
- secrets 不明文写入普通 JSON；优先使用系统 keychain，开发初期可引用环境变量名。
- `opsc serve` 是 Web UI 访问本地数据的唯一 HTTP 入口；`opsc mcp` 是 agent 通过 stdio 访问本地 workspace 查询能力的入口。
- `opsc serve` 即使只监听 `127.0.0.1`，也使用本地随机 bearer token 或 browser session。
- 浏览器旧数据不迁移；已知 legacy private keys 在 Web UI 启动时清理，现有 IndexedDB/localforage 测试数据直接丢弃。
- 现有 PDD VPS run 不迁移；后续本地化后按新 workspace 结构重新接入。
- CLI 是核心接口，MCP 封装 CLI/core，不重复实现业务逻辑。

## Local Workspace v1 目录结构

目标目录结构：

```text
~/OpsCanvas/
  opsc-workspace.json
  index.sqlite
  profiles/
  projects/
  templates/
  runs/
  artifacts/
  assets/
  prompts/
  canvas-projects/
  workbench-logs/
  cache/
  exports/
```

目录职责：

- `opsc-workspace.json`：workspace 元信息，例如 schema version、workspace id、默认 profile 和轻量 Web UI workspace preferences。
- `index.sqlite`：查询索引，用于快速列表、搜索、状态查询；不是唯一事实源。
- `profiles/`：本地 profile 配置，例如 local、cloud、hybrid；不直接保存明文 secrets。
- `projects/`：本地项目引用，保存外部项目路径、类型、权限和 adapter metadata。
- `templates/`：私有工作流模板，使用可读、可 diff、可 git 管理的 JSON。
- `runs/`：工作流运行记录，保存 run snapshot、节点状态、事件日志和结果引用。
- `artifacts/`：生成结果，每个 artifact 一个目录，保存 metadata、原始文件和可选缩略图。
- `assets/`：个人素材库，保存用户主动收藏或导入的素材。
- `prompts/`：个人 prompt 库。
- `canvas-projects/`：Web 无限画布项目，保存节点、连线、聊天会话、背景、viewport 和项目内 `files/` 媒体文件；不替代 workflow run。
- `workbench-logs/`：工作台 text/image/video 生成记录，保存可 diff 的日志 JSON 和记录关联媒体文件；不替代 workflow artifact。
- `cache/`：可重建缓存，例如缩略图、远程模板缓存、临时下载。
- `exports/`：导出包，用于备份、分享和迁移。

设计原则：

- JSON、JSONL 和文件目录是事实源。
- `index.sqlite` 只做索引，可通过扫描 workspace 文件重建。
- 浏览器 IndexedDB/localforage 只能作为 UI 缓存或临时状态，不再作为长期事实源。

## Web UI 本地化切换清单

已切换到 `opsc serve` 作为本地事实源的 Web UI 数据：

- 顶部本地工作区连接入口：浏览器只保存 loopback `baseUrl`，启动后先访问免鉴权 `/api/health` 区分“服务未启动”和“已启动但 session 未建立”，再用一次性 `launch.secret` bootstrap HttpOnly browser session。
- `我的素材`：`assets/<asset_id>/asset.json` 和 `files/original` 为 canonical data；浏览器 `image_files/media_files` 只作为上传、生成中或保存失败重试前的临时桥接。
- `我的提示词`：`prompts/<prompt_id>/prompt.json` 和 `content.md` 为 canonical data；浏览器不持久化提示词正文。
- 画布库与画布项目：`canvas-projects/<canvas_id>/canvas-project.json` 和项目内 `files/` 为 canonical data；`workspaceFileKey` 是新媒体引用，旧 `storageKey` 只做兼容和导入桥接。
- 工作台 text/image/video 生成记录：`workbench-logs/<wblog_id>/workbench-log.json` 和 `files/` 为 canonical data；旧 generation log key 不迁移。
- 本地 AI profile/proxy：模型渠道配置写入 `profiles/`，secret 只保存 `secretRef`，浏览器不保存 API Key。
- 本地项目引用：`projects/<proj_id>/project.json` 保存路径引用、capability 和 adapter metadata；列表不展示 `rootPath`，编辑时才显式读取路径。
- 工作流入口自定义文件夹：`opsc-workspace.json.data.preferences.workflowFolders` 为 canonical data，不再写旧 `ops-canvas-workflow-folders`。
- 电商私有模板、本地 run 和 artifact 基础 UI：连接本地工作区时通过 `templates/`、`runs/`、`artifacts/` 读写；固定本地素材启动 run 时会复制为 canonical artifact ref。

仍保持 legacy / cloud / VPS 边界的数据：

- public/admin 素材库、public/admin 提示词库、系统设置、账号登录态、远程模型渠道和后台管理仍走现有服务器 API / DB。
- 现有 PDD/VPS run、PDD 运行页结果、PDD 创作画布 artifact 展示仍按原有 VPS run 文件和 API 语义读取，不迁移到 local workspace。
- 浏览器主题、登录 token、当前页面交互状态、`opsc:*_cache:v1` 展示缓存和 `image_files/media_files` 临时媒体桥接仍可保留，但不得作为长期事实源。
- 旧 `infinite-canvas:*`、旧 generation logs 和旧 workflow folders 是测试数据，不迁移；Web UI 启动时清理已知 legacy private keys。

## 数据归属边界

默认本地保存：

- 私有工作流模板。
- 私有 workflow run。
- 个人素材。
- 个人 prompt。
- 生成图片、视频、文本和其他 artifact。
- 本地项目路径、项目权限和 adapter metadata。
- 本地运行日志、事件流和节点状态。

允许云端保存：

- 账号、授权、角色和计费信息。
- 官方模板、公共模板和用户显式发布的模板。
- 公共素材和后台素材。
- 商用模式下的模型渠道配置、限流、计费和中转审计。

云端上传规则：

- 私有模板、run、artifact、asset、prompt 默认不上云。
- 上传、发布、分享必须由用户显式触发，例如 `publish`、`upload` 或 `share`。
- 本地项目绝对路径默认不上传；需要云端摘要时必须脱敏。
- API key、token 和其他 secrets 不进入普通 JSON、日志或默认 CLI 输出。

模型调用按 profile 区分：

- 自用 profile：本地直接调用模型供应商，VPS 不经过私有内容。
- 商用 profile：可走 VPS 中转，用于隐藏平台 key、计费、限流和审计。

## CLI 与本地服务设计

CLI 是核心接口，Web UI 和 MCP 都应复用 CLI/core 能力。

最小命令草案：

```bash
opsc workspace init
opsc workspace info --json
opsc workspace doctor
opsc workspace index rebuild --json
opsc workspace export plan --json
opsc workspace gc plan --json
opsc profile list --json
opsc project list --json
opsc template list --json
opsc run list --json
opsc run status <run_id> --json
opsc run events <run_id>
opsc run events <run_id> --follow
opsc artifact list --json
opsc artifact list --run <run_id> --json
opsc asset list --json
opsc prompt list --json
opsc serve --workspace ~/OpsCanvas --port 17680 --origin http://127.0.0.1:3000
opsc mcp --workspace ~/OpsCanvas
```

CLI 输出约定：

- agent 友好命令都支持 `--json`。
- stdout 输出机器可读 JSON。
- stderr 输出人类日志、进度和诊断信息。
- 默认输出不暴露 secrets。
- 默认输出避免暴露敏感绝对路径；需要时通过显式参数查看。
- 所有核心对象使用稳定 ID，例如 `proj_*`、`tpl_*`、`run_*`、`node_*`、`art_*`、`asset_*`、`prompt_*`、`canvas_*`、`wblog_*`。

`opsc serve` 设计约定：

- 只监听本机地址，默认 `127.0.0.1`。
- Runtime/state 文件写入 workspace 外的 `$XDG_STATE_HOME/opsc/workspaces/<workspaceId>-<rootHash>/`；无 XDG 时 fallback 到 `~/.local/state/opsc/workspaces/<workspaceId>-<rootHash>/`。
- 启动时读取或生成本地随机 `bearer.token`，供 CLI 或显式 HTTP 客户端使用；browser 使用每次启动生成的一次性 `launch.secret` 换 HttpOnly session cookie。
- Web UI 通过 localhost API 访问 workspace；MCP stdio 的读取/诊断/dry-run 工具复用同一进程内 CLI JSON 命令，`opsc_workspace_index_rebuild` 作为唯一维护写工具必须经 active `opsc serve` HTTP API。
- 浏览器不直接写 `~/OpsCanvas`。
- workspace 文件统一由 CLI/core/local service 写入，避免并发写坏数据。
- 当前已实现查询/诊断/索引重建/导出计划/GC 计划、run event SSE、local template CRUD、run create/update、run node state、run event append、run artifact ref 写入、artifact create/update/multipart import、artifact/asset/canvas-project/workbench-log 文件读取、prompt content 读取，以及 profiles/projects/assets/prompts/canvas-projects 的 create/update/delete API；素材文件可通过 multipart import 写入 `assets/<asset_id>/files/`，artifact 文件可通过 multipart import 写入 `artifacts/<art_id>/files/`，画布项目媒体可通过 multipart 写入 `canvas-projects/<canvas_id>/files/`，工作台生成记录可通过 multipart 写入 `workbench-logs/<wblog_id>/files/`。

## 对象结构草案

run 目录建议结构：

```text
runs/
  run_xxx/
    run.json
    template.snapshot.json
    events.jsonl
    nodes/
      node_xxx.json
    creative_canvas/
      product_xxx.canvas.json
    artifacts/
      art_xxx.json
```

artifact 目录建议结构：

```text
artifacts/
  art_xxx/
    artifact.json
    original.png
    thumb.webp
```

artifact metadata 示例：

```json
{
  "id": "art_xxx",
  "type": "image",
  "mime": "image/png",
  "width": 1024,
  "height": 1024,
  "sourceRunId": "run_xxx",
  "sourceNodeId": "node_xxx",
  "sha256": "...",
  "files": {
    "original": "original.png",
    "thumbnail": "thumb.webp"
  }
}
```

v1 先采用每个 artifact 一个目录的直观结构；后续如需要去重，再增加 content-addressed `blobs/` 层。

## 落地顺序

1. 新增本计划文档，作为 local-first 数据分离的基线。
2. 定义 workspace v1 schema、对象 ID 规则和最小 JSON 字段。
3. 实现最小 CLI：`workspace init`、`workspace info --json`、`workspace doctor`。
4. 实现模板、run、artifact 的本地写入、run events、run artifact ref、node state 和 `index.sqlite` 重建。
5. 实现 profile、project、asset、prompt 的本地写入边界、索引重建、doctor 检查、export plan、project root fingerprint/path guard 和 GC dry-run plan。
6. 实现 `opsc serve` 本地 API。已完成本机 loopback 查询/诊断服务、workspace 外 runtime/state、browser session bootstrap、profiles/projects/assets/prompts/canvas-projects/workbench-logs 写入 API、对象删除、素材文件导入和工作台记录文件导入。
7. Web UI 改为通过 `opsc serve` 访问本地 workspace。已完成本地工作区连接入口、`我的素材`、`我的提示词`、画布项目库、画布项目内媒体文件、画布导入/导出 zip 媒体文件、工作台 text/image/video 生成记录、AI profile/proxy、电商工作流私有模板 CRUD、模板 `material_lookup` 固定素材本地选择、工作流入口自定义文件夹、本地 run 列表/状态页、run event/node/artifact ref 写入 API 和本地模板创建 run 草稿；AI profile 读取兼容完整 `secretRef.name` 和脱敏 `secretRef.reference`；图片/视频工作台结果保存成功后会从 workspace 文件端点回显；素材包导入不再写浏览器媒体库作为事实源，画布 zip 导入结束后清理临时 import blob；真实 PDD/VPS executor、PDD run 数据迁移和运行时 `material_lookup` local asset id 解析仍待迁移。
8. 新增 `opsc mcp` stdio 薄封装。已完成只读/诊断工具首版，MCP `tools/call` 的读取/诊断/dry-run 路径映射到现有 CLI JSON 命令；`opsc_workspace_index_rebuild` 是唯一维护写工具，通过 active `opsc serve` bearer API 执行，只重建派生索引。对象写入工具、local executor 工具和 canvas/workbench 工具后续再做。

## 不做事项

- 不迁移浏览器历史测试数据。
- 不迁移现有 PDD VPS run。
- 不把浏览器 IndexedDB/localforage 作为长期事实源。
- 不把私有 run、artifact、asset、prompt 默认上传云端。
- 不在本阶段实现 MCP 写入工具、模型调用工具或 local executor 工具。
- 不在普通 JSON、日志或默认 CLI 输出中保存或打印 secrets。

## 验收标准

- 文档位于 `docs/local-workspace-data-separation-plan.md`。
- 正式 contract 位于 `docs/local-workspace-v1-contract.md`。
- 文档覆盖已确认决策、workspace 目录、数据边界、CLI 设计、`opsc serve` runtime、object envelope、secretRef、capability、写入安全和兼容边界。
- Phase 0 只新增设计文档，不修改业务代码。
- Phase 1 foundation 新增本地 workspace 底座和最小 `opsc workspace` CLI，不迁移旧数据，不改变现有业务代码路径。
- Phase 3 foundation 新增 template/run/artifact file-backed repository、run events、run node state、run artifact ref、`index.sqlite` 增量更新/重建和只读查询 CLI，不接 UI，不替换现有 DB repository。
- Phase 4/5 foundation 新增 profile/project/asset/prompt file-backed repository、对应 index summary、doctor 检查、默认 export plan 排除规则、project root 安全边界、GC dry-run plan 和只读查询 CLI，不接 UI，不替换现有 DB repository。
- Phase 6 `opsc serve` 新增本机 loopback HTTP API、workspace 外 runtime metadata、bearer token、一次性 launch secret + HttpOnly session、CORS origin 白名单、run event SSE、artifact/asset/prompt/workbench-log 文件读取，以及 profiles/projects/assets/prompts 查询与写入能力；Phase 7 追加 canvas-projects/workbench-logs 和 local template 写入 API 与 Web UI adapter，不替换现有 DB repository。
- Phase 7 Web UI adapter 已接入 `我的素材`、`我的提示词`、画布项目库、画布项目内图片/视频媒体、画布导入/导出 zip 媒体文件、工作台 text/image/video 生成记录、AI profile/proxy、本地项目引用、电商工作流私有模板、模板 `material_lookup` 固定素材本地选择、工作流入口自定义文件夹和本地 run/artifact 基础 UI：canonical data 写入 local workspace，AI profile full document 的 `secretRef.name` 与 summary 的 `secretRef.reference` 已在 Web 读取侧兼容，本地项目列表不展示 `rootPath`，编辑时才读取路径且不会写回脱敏 credentialRef summary，图片/视频工作台结果保存成功后从 workspace 文件端点回显，浏览器只保留 `opsc:*_cache:v1` 缓存或临时媒体状态；素材/提示词 cache 已加 workspace 标识且不持久化私有列表/正文，素材包导入/导出不再依赖浏览器媒体库作为事实源，素材写入成功后会清理当前素材和画布都不再引用的 browser media blob，画布 zip 导入结束后会清理临时 import blob，避免 workspace 切换时显示旧数据；旧 AI config、旧素材/提示词/画布 store、旧 generation log 和旧 workflow folders 不迁移，并会在 Web UI 启动时清理。真实 PDD/VPS executor、PDD/VPS run 数据迁移和运行时 `material_lookup` local asset id 解析不在本轮迁移范围。
- Phase 7 MCP 首版新增 `opsc mcp` stdio server，读取/诊断/dry-run 工具映射到现有 CLI JSON 命令，覆盖 workspace info/doctor/export plan/gc plan、template/run/artifact/profile/project/asset/prompt 列表和 run status/events；`opsc_workspace_index_rebuild` 通过 active `opsc serve` bearer API 重建派生 `index.sqlite`；不暴露 canonical object 写入工具、不暴露 `run events --follow`、不直接读写 workspace repository。
- 后续实现时以本文档作为 local-first 数据分离基线。

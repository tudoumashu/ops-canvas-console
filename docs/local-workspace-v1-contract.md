# Local Workspace v1 Contract

本文档是 local-first 数据分离的 Phase 0 设计 contract。后续实现 CLI、`opsc serve`、Web UI 本地化和 MCP 封装时，必须以本文档为准；如需改变本文档中的 must 规则，需要先更新本 contract 或新增 ADR。

## Contract Status

- status：accepted for Phase 0
- scope：设计 contract 定稿；不迁移旧数据，不改变现有业务 API、DB、浏览器数据或 PDD/VPS 数据流。
- implementation status：Phase 6 已继续落地 `internal/localworkspace` 和 `cmd/opsc` 的 local workspace file-backed 底座，覆盖 workspace init/info/doctor、路径解析、manifest/envelope、ULID、`secretRef` 结构校验与脱敏 summary、atomic write、lock、doctor report、runtime metadata 读取、泛型 JSON object repository、workspace scan、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log typed repository、run artifact ref、run node state、append-only run events、project root salted fingerprint、project path capability guard、`index.sqlite` 增量更新与扫描重建、默认 export plan 排除规则、GC dry-run plan，以及查询 CLI：`workspace index rebuild`、`workspace export plan`、`workspace gc plan`、`template list`、`run list`、`run status`、`run events`、`run events --follow`、`artifact list`、`artifact list --run`、`profile list`、`project list`、`asset list`、`prompt list`；Phase 6 现已实现 `opsc serve` 本地 loopback API、XDG state runtime metadata、HTTP bearer token、browser 一次性 launch secret + HttpOnly session、CORS loopback origin 白名单、`{code,data,msg}` HTTP JSON envelope，以及 local profiles/projects/assets/prompts 查询与写入接口。Phase 7 已先补齐对象删除、asset multipart import 和 Web UI `我的素材` / `我的提示词` adapter；“我的素材”新增/更新/删除成功后会清理当前素材和画布都不再引用的 browser media blob；随后新增 local `canvas-projects` canonical object、serve API 和 Web UI 画布库 adapter；新增 local `workbench_log` canonical object、serve API、index/doctor/GC 检查和 Web UI text/image/video 工作台生成记录 adapter，并把画布图片/视频节点及助手媒体 canonical 化到 `canvas-projects/<canvas_id>/files/`；画布保存成功后会清理已 canonical 化且当前画布状态不再引用的 browser media blob；画布导入/导出 zip 现在会携带并恢复 `workspaceFileKey` 对应文件；继续新增 `opsc serve` 本地 AI proxy 和 Web UI AI profile adapter，本地模型渠道配置写入 `profiles/`，secret 只通过 `secretRef` 解析，不再把 API key 持久化到浏览器；图片/视频工作台保存成功后会把当前结果卡片切换到 workspace workbench-log 文件端点回显，并在保存成功、新会话、移除参考图或切换历史记录时清理对应浏览器参考图临时缓存；已补齐 `opsc serve` local template CRUD，并让电商工作流模板列表/编辑页在连接本地工作区时通过 `templates/<tpl_id>/template.json` 读写私有模板，本地模板 `material_lookup` 固定素材选择读取当前 workspace 图片素材；已新增 `workspace.preferences.workflowFolders` 和 `/api/local/workspace/preferences`，工作流入口自定义文件夹不再写浏览器 `localStorage`；本轮继续补齐 `opsc serve` local run/artifact 写入 API、Web UI 本地 run 列表/状态页/artifact 预览、本地模板创建 run 草稿，以及固定 `material_lookup` 本地素材启动时复制为 canonical artifact ref；顶部本地工作区弹窗已新增 projects 面板，可通过 `/api/local/projects` 创建/编辑/删除本地项目引用，列表不显示 `rootPath`，编辑时才用 `showPaths=1` 读取路径，含 `credentialRef` 的项目不在 Web UI 写回以避免覆盖真实 secretRef。本轮新增并加固 `opsc mcp` stdio MCP 薄封装，读取/诊断/dry-run 工具调用既有 CLI JSON 命令，唯一维护写工具 `opsc_workspace_index_rebuild` 通过 active `opsc serve` loopback API 重建派生 `index.sqlite`，不直接实现新的业务读写逻辑；Phase 12 已把 hybrid PDD/VPS executor 接到 `opsc executor --watch` 和 Web local run 黄金路径，现有 PDD/VPS run 历史仍不迁移。
- stabilization status：Phase 8 只做 hardening / verification / docs-sync，不新增 local executor、不迁移 PDD/VPS run、不做 Full GC、不扩大 MCP 写能力、不新增 canonical object 类型。已补充 `opsc serve` runtime/session/auth/redaction 回归、CLI `serve` 输出脱敏回归、AI proxy `secretRef` 与浏览器 header 隔离回归、本地模板草稿 run 到 canonical artifact ref 的 happy path 回归，并继续以 `go test ./cmd/opsc` 覆盖 MCP stdio wrapper smoke、对象 mutation 工具面冻结、doctor/export plan/GC dry-run/run events/index rebuild。
- executor status：Phase 14 已在 Phase 9/10 Local Workflow Executor 和 Phase 12 watch worker 基础上补齐已确认电商模板的 local-first 执行路径，唯一正式入口仍为 `opsc executor --workspace <path>`。run-once 领取 `run.waiting_for_executor` 的 local run；`--watch` 在单 workspace executor lock 保护下循环短持锁领取/同步 run，支持 `--poll-interval`，适合 Web UI 创建 run 后由本机常驻 worker 接管。executor 执行 `input`、`text_static`、固定本地素材 `material_lookup`、Phase 14 local ecommerce 自动 `anime_ip` 素材匹配、`text_generation`、`image_generation`、已确认模板所需 `image_edit`、`condition` 和本地项目 `script` 最小节点集，复用 workspace profile `secretRef` 调用 OpenAI-compatible provider，并写回 canonical node state、append-only events、global artifact 和 run artifact ref；已成功节点在重启恢复时跳过，避免重复写坏 artifact。带 `run.projectId` 的 run 会读取 `projects/<proj_id>/project.json`，校验 adapter、root fingerprint、capability、path safety 和 `artifact.write`，`script` 节点只允许 project-relative 命令/参数和最小环境变量；Phase 14 对 `package`/`sync_local` 提供 local ecommerce project action，把结果写到项目相对输出路径并在 node output 中保存相对 metadata。该入口不迁移 PDD/VPS run、不扩大 MCP 写面、不实现分布式 lease、Full GC、`video_generation`、超出已确认模板的通用 image-edit 编排、复杂 loop/guardrail、多轮质检修复或完整模板重试策略。
- hybrid ecommerce status：Phase 12 已把单条已确认电商模板的 hybrid VPS backend 路径产品化到 Web + worker 黄金路径。`opsc ecommerce import-template` 通过本地 profile/channel 的 `secretRef` 读取 VPS PDD template，重建为 local workspace `templates/<tpl_id>/template.json`，并记录 `remoteTemplateId`、`remoteTitle`、`remoteUpdatedAt`、`importedAt`、`sourceFingerprint`、`credentialMode` 等最小远端映射 metadata；显式 env `secretRef` 仅保留为 CLI smoke 诊断路径，不作为浏览器正式路径。local run 仍以 `runs/<run_id>/run.json`、template snapshot、node state、events 和 artifact refs 为 canonical。`opsc executor` 遇到带 `metadata.hybridEcommerce.backend=vps_pdd` 的模板时，只通过受控 PDD API 创建远端 run、同步 overview/product-detail、写入阶段进度 node output/metadata、下载确认 artifact 文件并写回本地 canonical `artifacts/`；远端 artifact path 必须是规范化相对路径，远端 run dir 和既有 PDD/VPS run 历史都不是本地事实源。浏览器仍只连接 `opsc serve`，不得保存或发送 VPS admin token、cookie 或明文 secret；MCP 写面不变。
- local ecommerce status：Phase 14 新增 `opsc ecommerce import-template --local-executable`，仍从已确认远端模板读取结构，但写入 `metadata.localEcommerce.backend=local_first` 的本地可执行 template，而不是 hybrid backend template。`opsc ecommerce create-run` 会按 template metadata 自动选择 hybrid 或 local-first 创建路径；local-first run 只写本地 canonical run、template snapshot、pending node state 和 `run.waiting_for_executor` event。默认素材库标识为 `anime_ip`，本机默认路径用于自用环境，真实路径只在 executor runtime 内部读取，不写入 artifact source；如用户显式传 `--material-library`，该路径属于本机开发配置，后续发布前需继续评估脱敏和配置化。local-first 路径只服务已确认电商模板的 material lookup、mockup base、source/mockup/main image edit、package 和 sync_local 黄金路径，hybrid VPS-backed fallback 保留。
- default workspace：`~/OpsCanvas`
- schema version：`local-workspace-v1`
- canonical data：JSON、JSONL 和文件目录。
- derived data：`index.sqlite`，只做索引，可删除后重建。
- local service：`opsc serve`，默认只监听 `127.0.0.1`。

## Core Decisions

- 私有数据默认本地：私有模板、run、artifact、个人素材、个人 prompt、本地项目路径和运行日志默认写入 local workspace。
- 云端只保存账号、授权、计费、官方/公共模板、公共素材，以及商用 profile 所需的模型中转、限流和审计。
- 浏览器 IndexedDB/localforage 只作为 UI 缓存或临时状态，不作为长期事实源；local workspace 连接状态最多持久化 loopback `baseUrl`，不得持久化 workspace/runtime metadata、token 文件名或 launch secret 文件名。
- 旧浏览器测试数据不迁移。
- 现有 PDD VPS run 不迁移；后续 PDD 本地化按 workspace adapter 重接。
- CLI 是核心接口；MCP 只封装 CLI/core，不重复实现业务逻辑。
- Web UI 访问本地数据时必须经过 `opsc serve`，浏览器不直接写 `~/OpsCanvas`。MCP stdio 入口由本机 `opsc mcp` 进程启动，读取/诊断/dry-run 工具包装现有 CLI JSON 命令，`opsc_workspace_index_rebuild` 作为唯一维护写工具经 active `opsc serve` loopback API 执行；未来若暴露 canonical object 写入或执行工具，必须复用 core/service 或 `opsc serve` 的写锁、鉴权和脱敏边界，不能绕过 workspace 安全规则。
- 现有 Go API、服务端 DB、PDD/VPS 文件目录和浏览器数据结构保持当前实现边界；本文只定义未来 local workspace contract。

## Workspace Resolution

CLI 和本地服务按以下顺序解析 workspace：

1. 命令行参数 `--workspace <path>`。
2. 环境变量 `OPSC_WORKSPACE`。
3. 默认路径 `~/OpsCanvas`。

规则：

- 除 `opsc workspace init` 外，命令不得静默创建 workspace。
- 若目标目录不存在或缺少 `opsc-workspace.json`，命令必须失败并给出明确错误。
- 多 workspace 是一等能力；实现不得把状态硬编码到单个全局目录。
- workspace 路径可在输出中脱敏；需要暴露绝对路径时必须由显式参数触发，例如 `--show-paths`。

## Directory Layout

```text
~/OpsCanvas/
  opsc-workspace.json
  index.sqlite
  .opsc/
    project-root.salt
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

- `opsc-workspace.json`：workspace 元信息、contract/schema version 和轻量 workspace preferences。
- `index.sqlite`：派生索引，服务列表、搜索和状态查询。
- `.opsc/`：workspace-local 控制目录；只保存 workspace 相关本地控制文件，例如 project root fingerprint salt；不得进入 export/share/publish。
- `.opsc/project-root.salt`：本 workspace 专用 project root fingerprint salt；只用于本地计算 opaque fingerprint，必须排除在 export/share/publish 之外。
- `profiles/`：local、cloud、hybrid 等 profile 配置；只保存非 secret 配置和 `secretRef`。
- `projects/`：外部项目引用；不复制项目源码或项目文件。
- `templates/`：私有工作流模板。
- `runs/`：工作流运行记录、节点状态、事件日志和 artifact refs。
- `artifacts/`：生成产物；每个 artifact 一个目录。
- `assets/`：用户主动导入或收藏的个人素材。
- `prompts/`：个人 prompt。
- `canvas-projects/`：Web 无限画布项目，保存节点、连线、聊天会话、背景和 viewport；不替代 workflow run。
- `workbench-logs/`：工作台 text/image/video 生成记录，保存日志 JSON 和关联媒体文件；不替代 workflow run/artifact。
- `cache/`：可重建缓存。
- `exports/`：显式导出的备份或分享包。

## Object IDs

所有核心对象使用带前缀的 ULID：

```text
ws_<ULID>
profile_<ULID>
proj_<ULID>
tpl_<ULID>
run_<ULID>
node_<ULID>
art_<ULID>
asset_<ULID>
prompt_<ULID>
canvas_<ULID>
wblog_<ULID>
evt_<ULID>
```

规则：

- ULID 使用 26 位 Crockford Base32 字符串。
- ID 一经写入不得重命名。
- 文件系统路径必须从 ID 派生，不能从用户输入标题直接派生。
- 标题、名称和标签只是 metadata，不参与唯一性判断。

## Common Object Envelope

除 `events.jsonl` 每行事件外，所有 canonical JSON object 使用统一 envelope：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "workspace",
  "id": "ws_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {}
}
```

规则：

- `schemaVersion` 必须为 `local-workspace-v1`。
- `kind` 必须与文件类型一致，例如 `workspace`、`profile`、`project`、`template`、`run`、`run_node`、`artifact`、`asset`、`prompt`、`canvas_project`、`workbench_log`、`creative_canvas`。
- `revision` 从 `1` 开始；canonical object 的业务内容变化时递增。
- `createdAt` 和 `updatedAt` 使用 UTC RFC3339 字符串。
- `data` 保存对象业务字段；实现不得把 secrets 写进 envelope 或 `data`。

## Event Envelope

`events.jsonl` 每行一个 event envelope：

```json
{
  "schemaVersion": "local-workspace-v1",
  "id": "evt_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "sequence": 1,
  "type": "run.started",
  "level": "info",
  "actor": {
    "type": "cli",
    "id": "opsc"
  },
  "subject": {
    "kind": "run",
    "id": "run_01HZZZZZZZZZZZZZZZZZZZZZZZ"
  },
  "message": "Run started",
  "createdAt": "<RFC3339>",
  "data": {}
}
```

规则：

- `sequence` 在同一个 event log 内从 `1` 递增。
- event log 是 append-only；不得重写已有事件来修正历史。
- `level` 取值为 `debug`、`info`、`warn`、`error`。
- `actor.type` 取值为 `cli`、`serve`、`mcp`、`web`、`system`、`project_adapter`。
- `data` 不得包含 secrets、完整 API key、cookie、session、用户本地敏感路径或大文件内容。

## Canonical Files

下面 JSON 示例中的 `<RFC3339>` 表示运行时写入的 RFC3339 timestamp 字符串。

### Workspace

`opsc-workspace.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "workspace",
  "id": "ws_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "name": "Default Workspace",
    "defaultProfileId": "profile_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "preferences": {
      "workflowFolders": [
        {
          "id": "custom-article",
          "title": "文章工作流",
          "description": "本地私有工作流入口",
          "kind": "custom"
        }
      ]
    }
  }
}
```

`data.preferences` 用于保存轻量 Web UI workspace preferences，当前只定义 `workflowFolders`。这类数据属于用户本机私有事实源，必须经 `opsc serve` 的 workspace 写锁、atomic write 和 revision 检查更新；不得回退到浏览器 `localStorage`。`workflowFolders[].id` 必须是 path-safe component，不能使用保留 id `pdd`；`kind` 当前允许 `custom`、`article`、`video`；`href` 如存在必须是站内相对路径；preferences JSON 不得包含明文 secret 字段。

### Profiles

```text
profiles/
  profile_<ULID>/
    profile.json
```

`profile.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "profile",
  "id": "profile_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "name": "local",
    "mode": "local",
    "channels": [
      {
        "id": "channel_openai_default",
        "protocol": "openai",
        "name": "OpenAI compatible",
        "baseUrl": "https://api.example.com",
        "models": ["gpt-5.5", "gpt-image-2"],
        "weight": 1,
        "enabled": true,
        "secretRef": {
          "type": "env",
          "name": "OPENAI_API_KEY"
        }
      }
    ]
  }
}
```

Profile mode：

- `local`：本地直连模型供应商。
- `cloud`：通过云端/VPS 中转。
- `hybrid`：按 workflow 或节点选择 local/cloud。

Channel 字段参考现有 `settings.private.channels`，但必须用 `secretRef` 替代明文 `apiKey`。

### `secretRef`

`secretRef` 用于引用 secret，不保存 secret 明文：

```json
{
  "type": "env",
  "name": "OPENAI_API_KEY"
}
```

支持类型：

- `env`：从环境变量读取，字段为 `name`。
- `keychain`：从系统 keychain 读取，字段为 `service`、`account`。
- `file`：从本机私有文件读取，字段为 `path`；该文件不得进入 exports/share/publish。
- `cloud`：由商用云端 profile 通过服务端鉴权解析，字段为 `channelId`。

规则：

- 禁止 `literal`、`plain`、`value` 这类明文 secret 形态。
- `secretRef` 可以出现在 profile channel、cloud auth、project adapter credential 中。
- 默认 CLI/API 输出只显示 secretRef 类型和脱敏标识，不回显 secret 值。
- `secretRef` 脱敏 summary 只允许暴露引用类型和非 secret 标识：`env` 可显示环境变量名，`keychain` 可显示 `service:account`，`cloud` 可显示 `channelId`，`file` 只能显示 `"<file>"`，不得回显本机私有文件绝对路径。

### Projects

```text
projects/
  proj_<ULID>/
    project.json
```

`project.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "project",
  "id": "proj_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "name": "example-project",
    "kind": "article",
    "adapter": "filesystem",
    "rootPath": "/absolute/path/to/project",
    "rootFingerprint": "rootfp_<sha256>",
    "capabilities": {
      "fs.read": true,
      "fs.write": false,
      "process.exec": false,
      "network.local": false,
      "artifact.write": true
    },
    "execution": {
      "allowGlobs": [],
      "denyGlobs": ["**/.env", "**/.env.*", "**/node_modules/**", "**/.git/**"]
    },
    "adapterMetadata": {},
    "credentialRefs": {
      "git": {
        "type": "env",
        "name": "GIT_TOKEN"
      }
    },
    "metadata": {}
  }
}
```

Capability model：

- `fs.read`：允许 adapter 读取 `rootPath` 内文件。
- `fs.write`：允许 adapter 写入 `rootPath` 内文件。
- `process.exec`：允许在 `rootPath` 内执行脚本或命令；默认关闭。
- `network.local`：允许 adapter 访问本机网络服务；默认关闭。
- `artifact.write`：允许 workflow 将产物写入 workspace `artifacts/`。

规则：

- `rootPath` 只保存在本地 workspace，不得默认上传云端。
- `rootFingerprint` 使用 workspace-local salt + normalized root path 计算，格式为 `rootfp_<sha256>`；它用于本地识别 root 是否变化，不可反推出真实路径，salt 保存于 `.opsc/project-root.salt` 并排除 export。
- 项目文件不复制进 workspace。
- `adapterMetadata` 只能保存可共享 adapter 元信息，不得保存 token、API key、password 等 secret 字段。
- `credentialRefs` 用于 project adapter 认证引用；只保存 `secretRef`，不保存明文 credential。
- project path helper 必须按 operation 检查 capability：`read` 需要 `fs.read`，`write` 需要 `fs.write`，`exec` 需要 `process.exec`。
- project path helper 只能接受 project-root-relative path；拒绝绝对路径、`..` escape、symlink escape，以及默认 deny globs 命中的 `.env`、`.git/`、`node_modules/`。
- `process.exec` 启用时必须沿用当前 `cmd/local-agent` 的安全边界：脚本路径必须是 project root 内相对路径，不能是绝对路径，不能逃逸到 root 外；建议同时配置收敛的 `allowGlobs`。
- adapter 可以扩展，例如 `filesystem`、`pdd-local`、`article-local`、`video-local`。

### Templates

```text
templates/
  tpl_<ULID>/
    template.json
```

`template.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "template",
  "id": "tpl_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "title": "Template Title",
    "workflowType": "generic",
    "version": 1,
    "nodes": [],
    "edges": [],
    "settings": {}
  }
}
```

规则：

- 私有模板默认只写本地。
- 公开模板必须通过显式 `publish` 上传。
- 模板 JSON 应保持可读、可 diff、可 git 管理。
- v1 template payload 可以映射现有 `model.WorkflowTemplateSpec` 的 `nodes`、`edges`、`settings`，但不直接复用服务端 DB row。

### Runs

```text
runs/
  run_<ULID>/
    run.json
    template.snapshot.json
    events.jsonl
    nodes/
      <path-safe-node-id>.json
    creative_canvas/
      product_<id>.canvas.json
    artifacts/
      art_<ULID>.ref.json
```

`run.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "run",
  "id": "run_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "templateId": "tpl_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "status": "pending",
    "profileId": "profile_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "projectId": "proj_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "input": {},
    "output": {},
    "artifactRefs": []
  }
}
```

Run status：

- `pending`
- `running`
- `success`
- `error`
- `canceled`

规则：

- `template.snapshot.json` 保存运行时模板快照，run 不依赖未来模板变更。
- 私有 run 默认不上云。
- 运行事件使用 append-only JSONL。
- 修改 run 汇总状态时必须同步追加事件。
- `nodes/*.json` 保存节点状态快照，文件名必须是 path-safe 派生名；当前实现使用 `base64.RawURLEncoding(nodeId) + ".json"`，`data.nodeId` 保留现有 workflow 原始节点 ID。
- v1 本地 run status 不使用当前 DB `workflow_runs.status=idle`；从现有 DB 导入时如未来需要，可把 `idle` 映射为 `pending`。当前已确认不迁移现有 PDD VPS run。

### Run Artifact Ref

run 内 `artifacts/*.ref.json` 只保存 artifact 引用，不复制二进制文件，也不复制完整 canonical artifact metadata：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "run_artifact_ref",
  "id": "art_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "artifactId": "art_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "role": "primary_output",
    "nodeId": "node_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "slot": "output",
    "order": 0
  }
}
```

规则：

- canonical artifact metadata 只在 `artifacts/art_<ULID>/artifact.json`。
- run 内 artifact ref 可以记录该 artifact 在本 run 中的 role、node、slot 和顺序。
- 删除 artifact 前必须检查 run refs 和 asset refs。

### Artifacts

```text
artifacts/
  art_<ULID>/
    artifact.json
    original.<ext>
    thumb.webp
```

`artifact.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "artifact",
  "id": "art_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "type": "image",
    "mime": "image/png",
    "title": "Generated Image",
    "sha256": "...",
    "bytes": 123456,
    "width": 1024,
    "height": 1024,
    "durationSeconds": null,
    "source": {
      "kind": "run_node",
      "runId": "run_01HZZZZZZZZZZZZZZZZZZZZZZZ",
      "nodeId": "node_01HZZZZZZZZZZZZZZZZZZZZZZZ"
    },
    "privacy": "private",
    "files": {
      "original": "original.png",
      "thumbnail": "thumb.webp"
    }
  }
}
```

Artifact type：

- `image`
- `video`
- `text`
- `audio`
- `file`

Rules：

- 生成产物必须复制进 `artifacts/`，不能只引用临时路径。
- `files.*` 必须是 artifact 目录内相对路径。
- `privacy` 取值为 `private`、`public`、`shared`；默认 `private`。
- v1 不做跨 artifact 去重；后续如需要，再新增 content-addressed `blobs/` 层。
- artifact 可被 run 引用，也可被 asset 收藏引用。

### Assets

```text
assets/
  asset_<ULID>/
    asset.json
    files/
      original.<ext>
      thumb.webp
```

`asset.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "asset",
  "id": "asset_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "type": "image",
    "mime": "image/png",
    "title": "Reference Image",
    "mediaType": "image",
    "category": "reference",
    "categoryPath": "image/reference",
    "purpose": "reference",
    "source": "workspace",
    "coverUrl": "",
    "description": "",
    "content": "",
    "sourceArtifactId": "art_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "privacy": "private",
    "tags": [],
    "files": {
      "original": "files/original.png",
      "thumbnail": "files/thumb.webp"
    },
    "metadata": {}
  }
}
```

规则：

- 个人素材默认本地。
- asset 是私有本地素材库对象；字段对齐现有 `model.Asset` 的常用筛选维度，但 canonical data 不写入现有服务端 DB。
- `files.*` 必须是 asset 目录内相对路径。
- 浏览器 `image_files/media_files` 只允许作为上传、导入或保存失败前的临时桥接；素材新增、更新或删除成功后，当前素材列表和画布项目都不再引用的 browser media blob 必须清理。
- `privacy` 取值为 `private`、`public`、`shared`；默认 `private`。
- 从 artifact 收藏为 asset 时，可以复制文件，也可以引用同 workspace 内 artifact；实现必须保证源 artifact 删除策略不会让 asset 静默失效。
- `sourceArtifactId` 指向 canonical artifact metadata；asset 可额外保存自己的 `files/` 副本，但 run 目录不复制 artifact metadata。

### Prompts

```text
prompts/
  prompt_<ULID>/
    prompt.json
    content.md
```

`prompt.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "prompt",
  "id": "prompt_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "title": "Prompt Title",
    "kind": "image",
    "coverUrl": "",
    "category": "image",
    "domain": "workflow",
    "stage": "draft",
    "provider": "local",
    "model": "gpt-image-2",
    "mode": "generation",
    "inputType": "text",
    "outputType": "image",
    "status": "active",
    "privacy": "private",
    "tags": [],
    "metadata": {}
  }
}
```

规则：

- prompt 正文写入 `content.md`。
- prompt 是私有本地 prompt 库对象；字段对齐现有 `model.Prompt` 的常用分类和模型筛选维度，但 canonical data 不写入现有服务端 DB。
- 个人 prompt 默认本地。
- `privacy` 取值为 `private`、`public`、`shared`；默认 `private`。
- 公共 prompt 必须显式发布或拉取。

### Canvas Projects

```text
canvas-projects/
  canvas_<ULID>/
    canvas-project.json
    files/
      node_image.png
      assistant_img_0.webp
```

`canvas-project.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "canvas_project",
  "id": "canvas_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "title": "Canvas Project",
    "nodes": [],
    "connections": [],
    "chatSessions": [],
    "activeChatId": "",
    "backgroundMode": "dots",
    "showImageInfo": true,
    "viewport": {
      "x": 0,
      "y": 0,
      "k": 1
    },
    "files": {
      "node_image": {
        "role": "image",
        "nodeId": "node_image",
        "mime": "image/png",
        "path": "files/node_image.png",
        "width": 1024,
        "height": 1024,
        "bytes": 123456
      }
    },
    "metadata": {}
  }
}
```

规则：

- 画布项目是 Web 无限画布库的 canonical data，不是 workflow run，也不替代 run/artifact 的 canonical metadata。
- `nodes`、`connections`、`chatSessions` 允许保存现有前端画布 JSON 结构，但对象 ID 必须是 path-safe string，不能包含 `/`、`\`、`..` 或空白 ID。
- `connections[].fromNodeId` / `toNodeId` 必须引用同一 `canvas-project.json` 内存在的节点 ID。
- 节点几何字段和 viewport 数值必须是有限数字；`backgroundMode` 只允许 `dots`、`lines`、`blank`。
- 图片/视频节点和助手媒体的 canonical 二进制文件必须写入同一项目目录下的 `files/`，`data.files[<file_key>]` 是唯一稳定 metadata；`file_key` 必须 path-safe，`path` 必须是 `files/` 内相对路径。
- 节点和助手消息通过 `metadata.workspaceFileKey` / `workspaceFileKey` 引用 `data.files`；`metadata.content`、`dataUrl`、`blob:` URL 和 `workspace://canvas-file/<key>` 只用于展示或占位，不是稳定文件标识。
- 旧浏览器 `storageKey` 只作为导入/过渡兼容输入；保存到 local workspace 时应尽量解析 Blob 并转换为 `workspaceFileKey`，不得把新画布媒体长期依赖浏览器 Blob 缓存。
- 画布保存、导入或删除成功后，应清理已被 canonical 化且当前画布状态不再引用的 `image:*` / `video:*` / `file:*` browser blob；如果保存期间用户继续编辑并且当前状态仍引用同一 storageKey，则必须保留该 blob 到下一次保存。
- 画布导出 zip 必须把项目引用的 `workspaceFileKey` 文件作为压缩包文件导出；导入 zip 时可临时写入浏览器 Blob 缓存作为上传桥接，但导入完成后的 canonical 文件必须重新写入目标 workspace `canvas-projects/<canvas_id>/files/`，并清理导入桥接用的临时 browser blob。
- `canvas-project.json` 不得保存明文 API key、token、password、secret 等敏感字段；需要引用凭据时必须使用 `secretRef`。
- 浏览器 IndexedDB/localforage 只能保存 `opsc:canvas_store_cache:v1` 展示缓存，不再作为画布项目事实源。

### Workbench Logs

```text
workbench-logs/
  wblog_<ULID>/
    workbench-log.json
    files/
      image_0.png
      video_0.mp4
```

`workbench-log.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "kind": "workbench_log",
  "id": "wblog_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "revision": 1,
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "data": {
    "modality": "image",
    "title": "Image generation",
    "createdAtMillis": 1710000000000,
    "status": "success",
    "model": "gpt-image-2",
    "prompt": "...",
    "media": [
      {
        "key": "image_0",
        "role": "result",
        "name": "image_0.png",
        "mime": "image/png",
        "path": "files/image_0.png",
        "width": 1024,
        "height": 1024,
        "bytes": 123456
      }
    ],
    "payload": {},
    "metrics": {},
    "metadata": {}
  }
}
```

规则：

- `modality` 取值为 `text`、`image`、`video`；`status` 取值为 `success`、`error`。
- `media[].key` 必须是 path-safe 唯一值；`media[].path` 必须是 workspace-relative，并且只能指向同一记录目录下的 `files/` 子目录。
- 记录正文、参数快照、展示布局等轻量 JSON 放入 `payload`；二进制图片/视频文件不得以 base64 或 blob URL 作为 canonical data 写进 JSON。
- `workbench-log.json` 不得保存明文 API key、token、password、secret 等敏感字段；需要引用凭据时必须使用 `secretRef`。
- 工作台生成记录是 text/image/video 工作台历史记录的 canonical data，不替代 workflow run、workflow artifact 或画布媒体节点的 canonical artifact metadata。
- 浏览器旧 key `text_generation_logs`、`image_generation_logs`、`video_generation_logs` 和对应历史测试媒体不迁移；新 Web UI 只通过 `opsc serve` 写入 `workbench-logs/`，图片/视频生成结果保存成功后当前结果卡片应改用 `/api/local/workbench-logs/<id>/files/<media_key>` 回显；上传参考图在保存成功后应改用同一记录的 workspace file URL 回显，保存成功、新会话、移除参考图或切换历史记录时应清理对应 `image:*` browser blob；生成中、上传参考图和失败未保存结果里的 `blob:`/localforage 只属于临时状态。

## Index Contract

`index.sqlite` 是派生索引：

- 必须可以从 canonical files 重建。
- 不得保存 canonical files 中没有的业务事实。
- 不得保存 secrets。
- 损坏时应允许 `workspace doctor` 报告，并允许后续 `workspace index rebuild` 重建。
- rebuild 必须跳过 `.opsc/`、`cache/`、`exports/`。

v1 public contract 不固定 SQLite 表结构；表结构是实现细节。CLI 和 `opsc serve` 的 JSON 输出才是外部稳定接口。当前实现已落地私有 `index.sqlite` 表，用于 profiles、projects、templates、runs、artifacts、assets、prompts、canvas-projects、workbench-logs、run artifact refs、run node states、run events 和 index metadata 的查询索引；canonical JSON/JSONL/文件目录仍是唯一事实源。`workspace index rebuild` 会扫描 canonical files 并重建索引，扫描计数只代表 canonical object entries，不等同于 sqlite 行数。

## CLI Contract

最小命令：

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
opsc serve
opsc executor
opsc executor --watch
opsc ecommerce import-template
opsc ecommerce create-run <tpl_id>
```

通用规则：

- 支持 `--workspace <path>`。
- agent 友好命令支持 `--json`。
- `--json` 模式下 stdout 不输出 ANSI 或人类进度文本。
- 成功时 stdout 输出 JSON。
- 失败时退出非 0；`--json` 模式下 stderr 输出 JSON error envelope。例外：`workspace doctor` 能产生命令结果但发现 workspace 不健康时，stdout 输出 machine-readable report，stderr 输出 human-readable report，退出码为 2。
- stderr 可输出人类日志、进度和诊断信息。
- 非 JSON 模式下，普通命令结果输出到 stdout；参数错误、打开失败等无法产生命令结果的错误输出到 stderr。
- `workspace doctor` 的 human-readable report 始终输出到 stderr；stdout 只在 `--json` 模式下输出 machine-readable report。`workspace doctor` 发现 workspace 不健康时退出码为 2，但仍应输出完整诊断报告。
- 默认输出不得包含 secrets。
- 默认输出不得包含敏感绝对路径；需要时使用显式参数。

成功 JSON envelope：

```json
{
  "ok": true,
  "data": {},
  "warnings": []
}
```

`workspace doctor --json` JSON envelope：

```json
{
  "ok": false,
  "data": {
    "ok": false,
    "workspaceId": "ws_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "schemaVersion": "local-workspace-v1",
    "checks": [
      {
        "name": "workspace_root",
        "ok": true,
        "severity": "info",
        "message": "workspace root exists"
      }
    ]
  },
  "warnings": []
}
```

失败 JSON envelope：

```json
{
  "ok": false,
  "error": {
    "code": "workspace_not_found",
    "message": "Workspace not found",
    "details": {}
  }
}
```

Exit code：

- `0`：成功。
- `1`：参数、输入或校验错误。
- `2`：workspace 缺失、对象不存在或本地数据损坏。
- `3`：权限、认证或本地 token 错误。
- `4`：外部服务、模型供应商或云端调用失败。
- `5`：未预期内部错误。

当前 workspace 基础命令示例：

```bash
opsc workspace init --workspace ./tmp/ws --name "Demo Workspace" --json
```

stdout：

```json
{
  "ok": true,
  "data": {
    "created": true,
    "workspace": {
      "id": "ws_01HZZZZZZZZZZZZZZZZZZZZZZZ",
      "name": "Demo Workspace",
      "schemaVersion": "local-workspace-v1",
      "revision": 1,
      "pathSource": "flag",
      "directories": [".opsc", "profiles", "projects", "templates", "runs", "artifacts", "assets", "prompts", "canvas-projects", "workbench-logs", "cache", "exports"],
      "runtime": {
        "active": false
      }
    }
  },
  "warnings": []
}
```

```bash
opsc workspace info --workspace ./tmp/ws --json
```

stdout：

```json
{
  "ok": true,
  "data": {
    "id": "ws_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "name": "Demo Workspace",
    "schemaVersion": "local-workspace-v1",
    "revision": 1,
    "pathSource": "flag",
    "directories": [".opsc", "profiles", "projects", "templates", "runs", "artifacts", "assets", "prompts", "canvas-projects", "workbench-logs", "cache", "exports"],
    "runtime": {
      "active": false
    }
  },
  "warnings": []
}
```

```bash
opsc workspace doctor --workspace ./tmp/ws --json
```

stdout：

```json
{
  "ok": true,
  "data": {
    "ok": true,
    "workspaceId": "ws_01HZZZZZZZZZZZZZZZZZZZZZZZ",
    "schemaVersion": "local-workspace-v1",
    "checks": [
      {
        "name": "workspace_root",
        "ok": true,
        "severity": "info",
        "message": "workspace root exists"
      }
    ]
  },
  "warnings": []
}
```

stderr：

```text
Workspace OK
- [ok] workspace_root: workspace root exists
- [ok] workspace_document: workspace document is valid
```

当前已实现的查询命令、本地服务和 MCP stdio 入口：

```bash
opsc workspace index rebuild --workspace ./tmp/ws --json
opsc workspace export plan --workspace ./tmp/ws --json
opsc workspace gc plan --workspace ./tmp/ws --json
opsc profile list --workspace ./tmp/ws --json
opsc project list --workspace ./tmp/ws --json
opsc template list --workspace ./tmp/ws --json
opsc run list --workspace ./tmp/ws --json
opsc run status run_01HZZZZZZZZZZZZZZZZZZZZZZZ --workspace ./tmp/ws --json
opsc run events run_01HZZZZZZZZZZZZZZZZZZZZZZZ --workspace ./tmp/ws
opsc run events run_01HZZZZZZZZZZZZZZZZZZZZZZZ --workspace ./tmp/ws --follow
opsc artifact list --workspace ./tmp/ws --json
opsc artifact list --run run_01HZZZZZZZZZZZZZZZZZZZZZZZ --workspace ./tmp/ws --json
opsc asset list --workspace ./tmp/ws --json
opsc prompt list --workspace ./tmp/ws --json
opsc serve --workspace ./tmp/ws --port 0 --origin http://127.0.0.1:3000 --json
opsc executor --workspace ./tmp/ws --json
opsc executor --workspace ./tmp/ws --watch --poll-interval 5s --json
opsc ecommerce import-template --workspace ./tmp/ws --remote-template <template_id> --profile <profile_id> --channel <channel_id> --json
opsc ecommerce create-run tpl_01HZZZZZZZZZZZZZZZZZZZZZZZ --workspace ./tmp/ws --input-file ./tmp/hybrid-input.json --json
opsc mcp --workspace ./tmp/ws
```

除 `run events` 外，这些命令使用同一个 success JSON envelope，默认不输出 workspace 绝对路径，也不读取或打印 secrets。`project list` 返回 `hasRootPath` 和 opaque `rootFingerprint`，不返回 `rootPath`；`profile list` 不返回 `secretRef`；`workspace export plan` 只返回相对路径和排除原因；`workspace gc plan` 只做 dry-run，返回待人工 review 的相对路径 candidates，不删除文件。`run events` 为 agent/streaming 友好的 JSONL 输出，每行一个 event envelope，不再额外包一层 success envelope；`--follow` 会先输出已有事件，再轮询追加事件直到调用方中断。`run status` 返回 run summary、node state summaries、node output/metadata summaries 和 `latestEventSequence`。`artifact list --run` 通过 index 查询 run refs，并返回对应 canonical `artifacts/art_<ULID>/artifact.json` 的摘要和 run 内 ref metadata。`opsc executor` 默认是 run-once 本地执行入口，可用 `--run <run_id>` 限定单个 run；`--watch` 是本地 worker 模式，会按 `--poll-interval` 循环处理 pending/running run，hybrid 远端非终态同步会短持锁完成一次 overview 同步后释放锁再等待下一轮；`--json` watch 模式在有处理结果时按行输出 JSON envelope，避免人类日志混入 stdout。workflow 失败会写入 run/node error 并在命令结果中返回该 run 状态，基础设施错误才作为 CLI 非 0 失败。`opsc ecommerce import-template` 只导入一个已确认远端 PDD template 的 local canonical copy，Web/watch 正式凭据必须来自 profile/channel `secretRef`；显式 env `secretRef` 仅用于 CLI smoke 诊断，CLI 输出不得包含 token、workspace 绝对路径或远端 run dir；导入 metadata 必须包含最小远端映射、`sourceFingerprint` 和 `credentialMode`，用于识别本地副本来源与凭据路径。`opsc ecommerce create-run` 只基于已导入的 local hybrid template 创建 pending local run、template snapshot、pending node states 和 `run.waiting_for_executor` event；`--input-file` 可是 JSON object 或 bare inputs array，命令不会调用 VPS API，真实远端执行仍由 `opsc executor` 完成。

Phase 11/12 的 `tools/hybrid_ecommerce_vps_smoke.py` 和 `tools/hybrid_ecommerce_browser_smoke.py` 是验证 helper，不是新的 contract surface。它们只编排 `opsc`、`opsc serve`、Web UI 和 fake/real VPS API 的黄金路径，不直接读写 workspace 文件，不打印 secret；真实 canonical 规则仍以 `opsc` core/serve/executor 为准。

## `opsc serve` Contract

`opsc serve` 是本地 Web UI 访问 workspace 的唯一 HTTP 入口；`opsc mcp` 是 agent 通过 stdio 访问本地 workspace 查询能力的入口。

启动参数：

```bash
opsc serve --workspace ~/OpsCanvas --host 127.0.0.1 --port 17680
```

规则：

- 默认监听 `127.0.0.1`，默认端口 `17680`，`--port 0` 使用系统分配端口。
- 拒绝非 loopback host；若未来支持非 loopback，必须新增显式 allow flag。
- Runtime/state 不写入 workspace，写入 `$XDG_STATE_HOME/opsc/workspaces/<workspaceId>-<rootHash>/`；无 XDG 时 fallback 到 `~/.local/state/opsc/workspaces/<workspaceId>-<rootHash>/`。
- CLI 或显式 HTTP 客户端可使用 `Authorization: Bearer <bearer.token>`；browser 使用一次性 `launch.secret` 交换 HttpOnly session cookie。`opsc mcp` 的查询/诊断/dry-run 工具复用同一进程内既有 CLI JSON 命令；唯一维护写工具 `opsc_workspace_index_rebuild` 必须读取当前 workspace runtime state 中的相对 `bearer.token`，并调用 active loopback `opsc serve` API，不得回退到直接 CLI 写入。
- 带 `Origin` 的请求只接受 session cookie，不接受 bearer token。
- CORS 只允许明确配置的 loopback Web UI origin，必须启用 credentials，不得使用 `*`。
- HTTP JSON API 使用现有业务响应 envelope：`{ "code": 0, "data": {}, "msg": "ok" }`；失败为 `{ "code": 1, "data": null, "msg": "..." }`。
- API 不得返回明文 secrets，默认不得返回敏感绝对路径；需要时必须显式请求，例如 `showPaths=1`。
- workspace 写入必须由 CLI/core/local service 统一完成，避免浏览器直接并发写文件。

当前实现：

- `cmd/opsc` 已支持 `opsc serve`，参数包括 `--workspace`、`--host`、`--port`、`--origin` 和 `--json`。
- `--json` 启动后 stdout 输出 CLI `{ ok, data, warnings }` runtime summary，不包含 token、launch secret 或 workspace 绝对路径；非 JSON 模式把监听地址、token file 和 launch secret file 的相对文件名输出到 stderr。
- runtime summary 和 `workspace info` 只返回 `bearer.token`、`launch.secret` 这类相对文件名，不返回 secret 明文。
- 当前 HTTP API 覆盖 workspace 查询、doctor、index rebuild、export plan、GC plan、templates/runs/artifacts/profiles/projects/assets/prompts/canvas-projects/workbench-logs summaries、local template create/get/update/delete、run create/get/update/status/events/node state/artifact ref、artifact create/get/update/import、artifact/asset/canvas-project/workbench-log 文件读取、prompt content 读取、`/api/local/ai/v1/*` OpenAI-compatible 本地代理，以及 profiles/projects/assets/prompts/canvas-projects 的 create/update/delete；asset、artifact 另支持 multipart file import，canvas-project 和 workbench-log 支持 multipart media import。
- Web UI 本地化时不得绕过 `opsc serve` 直接写 workspace 文件。当前 Web UI 已接入 `我的素材`、`我的提示词`、画布项目库、text/image/video 工作台生成记录、AI 本地 profile/proxy、电商工作流私有模板、模板 `material_lookup` 固定素材本地选择、固定素材启动时写入 run artifact ref、工作流入口自定义文件夹、本地项目引用面板和 local run/artifact 基础 UI，其它私有浏览器数据仍按兼容边界处理。

## `opsc mcp` Contract

`opsc mcp` 是首版 agent 集成入口，使用 MCP stdio transport。读取、诊断和 dry-run 工具把 MCP `tools/call` 映射到现有 `opsc` CLI JSON 命令；`opsc_workspace_index_rebuild` 作为唯一维护写工具，通过 active `opsc serve` single-writer HTTP API 重建派生索引。MCP 不能直接访问 workspace repository 或重新实现业务逻辑。

启动参数：

```bash
opsc mcp --workspace ~/OpsCanvas
```

当前工具：

- `opsc_workspace_info` -> `opsc workspace info --json`
- `opsc_workspace_doctor` -> `opsc workspace doctor --json`
- `opsc_workspace_index_rebuild` -> active `opsc serve` `POST /api/local/workspace/index/rebuild`
- `opsc_workspace_export_plan` -> `opsc workspace export plan --json`
- `opsc_workspace_gc_plan` -> `opsc workspace gc plan --json`
- `opsc_template_list` -> `opsc template list --json`
- `opsc_run_list` -> `opsc run list --json`
- `opsc_run_status` -> `opsc run status <run_id> --json`
- `opsc_run_events` -> `opsc run events <run_id>`；不暴露 `--follow`，避免工具调用不返回。
- `opsc_artifact_list` -> `opsc artifact list --json` 或 `opsc artifact list --run <run_id> --json`
- `opsc_profile_list` -> `opsc profile list --json`
- `opsc_project_list` -> `opsc project list --json`
- `opsc_asset_list` -> `opsc asset list --json`
- `opsc_prompt_list` -> `opsc prompt list --json`

规则：

- stdio stdout 只能输出合法 MCP JSON-RPC message；日志只允许输出到 stderr。
- 默认不输出 workspace 绝对路径、project `rootPath`、secret 明文、bearer token、launch secret 或 session id。
- `workspace` 参数可在 MCP server 启动时用 `--workspace` 指定，也可在单个 tool arguments 中覆盖；默认仍按 `OPSC_WORKSPACE` 或 `~/OpsCanvas` 解析。
- 当前 MCP 工具只覆盖读取、诊断、index rebuild、export/GC dry-run plan；`index rebuild` 只重建派生 `index.sqlite`，不写 canonical object，且必须经 active `opsc serve` loopback API 执行。MCP 不暴露 profiles/projects/assets/prompts/templates/runs/artifacts 的写入工具，不启动 local executor，不调用模型供应商。
- `run events` 返回 JSONL 文本；其它查询工具返回底层 CLI success envelope 的结构化内容。
- 工具执行失败使用 MCP `isError=true` 返回 CLI stdout/stderr 或 `opsc serve` 响应摘要；`opsc serve` 未启动时 `opsc_workspace_index_rebuild` 必须返回 tool error，不得直接绕过服务写索引。未知工具、参数格式错误或协议错误使用 JSON-RPC error。
- 未来新增写入型 MCP 工具前，必须先在 CLI/core 或 `opsc serve` 中已有同等能力，并继续复用 single-writer、atomic write、lock、revision、path guard 和 secret redaction 规则。

Runtime/state 文件：

```text
$XDG_STATE_HOME/opsc/workspaces/<workspaceId>-<rootHash>/
  serve.json
  serve.pid
  serve.port
  bearer.token
  launch.secret
  sessions.json
  serve.lock
  workspace.write.lock
  executor.json
  executor.pid
  executor.watch.lock
```

`serve.json`：

```json
{
  "schemaVersion": "local-workspace-v1",
  "pid": 12345,
  "host": "127.0.0.1",
  "port": 17680,
  "baseUrl": "http://127.0.0.1:17680",
  "workspaceId": "ws_01HZZZZZZZZZZZZZZZZZZZZZZZ",
  "workspacePath": "<redacted>",
  "startedAt": "<RFC3339>",
  "tokenFile": "bearer.token",
  "launchSecretFile": "launch.secret"
}
```

Runtime file rules：

- state directory 权限必须为 `0700`。
- `bearer.token`、`launch.secret`、`sessions.json` 权限必须为 `0600`。
- `serve.json`、`serve.pid`、`serve.port` 不得包含 token、launch secret 或 session id 明文。
- `bearer.token` 可跨 serve 重启复用；`launch.secret` 每次启动生成，成功交换 session 后立即消费。
- `serve.lock` 防止同一 workspace 重复启动冲突的 serve 实例；`workspace.write.lock` 保护 workspace 写入。
- `executor.watch.lock` 防止同一 workspace 同时运行多个 `opsc executor --watch` worker；`executor.json`/`executor.pid` 只保存 worker pid、模式、心跳时间、轮询间隔和最近一次处理摘要，不保存 workspace 绝对路径、token、launch secret 或 provider secret。
- stale pid/lock 只能在确认进程不存在后清理。

Authentication：

- `GET /health` 和 `GET /api/health` 可无鉴权，只返回 `ok`，不得泄露 workspace 路径、token、profile 或对象数据。
- `POST /api/local/bootstrap/session` 携带 `{ "launchSecret": "..." }`，成功后设置 `HttpOnly; SameSite=Lax; Path=/` session cookie，并消费 launch secret。
- 除 health 和 bootstrap 外，所有 `/api/local/*` API 默认都需要 session cookie 或 bearer token。
- `opsc serve` bearer token 与现有后端 `LOCAL_AGENT_TOKEN` 是两套机制，不能混用。

HTTP API：

- `GET /api/health`：免鉴权，只返回 `ok`。
- `POST /api/local/bootstrap/session`：一次性 launch secret 换 browser session cookie。
- `GET /api/local/runtime`：返回 runtime summary，不含 token、launch secret 和 workspace path。
- `GET /api/local/workspace`：返回 workspace info，默认不含 workspace 绝对路径。
- `GET /api/local/workspace/preferences`：返回 `{ "revision": <workspace_revision>, "preferences": { ... } }`，用于读取轻量 Web UI workspace preferences。
- `PUT /api/local/workspace/preferences`：update workspace preferences，body 为 `{ "revision": 1, "preferences": { ... } }`；revision 不匹配时拒绝覆盖，写入前拒绝明文 secret 字段。
- `GET /api/local/workspace/doctor`：返回 doctor report。
- `POST /api/local/workspace/index/rebuild`：重建派生 `index.sqlite`。
- `GET /api/local/workspace/export/plan`：返回 export dry-run plan。
- `GET /api/local/workspace/gc/plan`：返回 GC dry-run plan。
- `GET /api/local/templates`：返回 template summary 列表；`POST /api/local/templates`：create 本地模板，body 为 `{ "data": { ... } }`。
- `GET /api/local/templates/<template_id>`：返回 template canonical document；`PUT /api/local/templates/<template_id>`：update，body 为 `{ "revision": 1, "data": { ... } }`；revision 不匹配时拒绝覆盖；`DELETE /api/local/templates/<template_id>`：delete，删除对应 canonical object 目录并更新派生 index。
- `GET /api/local/runs`、`GET /api/local/artifacts`：返回对应 summary 列表。
- `POST /api/local/runs`：create run，body 为 `{ "data": { ... } }`；`GET /api/local/runs/<run_id>` 返回 run canonical document；`PUT /api/local/runs/<run_id>` update，body 为 `{ "revision": 1, "data": { ... } }`。
- `GET /api/local/runs/<run_id>/status`、`GET /api/local/runs/<run_id>/events?after=<sequence>`、`GET /api/local/runs/<run_id>/events/stream?after=<sequence>`、`GET /api/local/runs/<run_id>/artifacts`。
- `POST /api/local/runs/<run_id>/events`：append run event，body 为 `{ "event": { ... } }`；`POST|PUT /api/local/runs/<run_id>/nodes/<node_id>`：write run node state，body 为 `{ "revision": 1, "data": { ... } }`；`POST /api/local/runs/<run_id>/artifacts`：write run-local artifact ref，body 为 `{ "revision": 1, "data": { "artifactId": "art_..." } }`。
- `GET /api/local/profiles`、`GET /api/local/projects`、`GET /api/local/assets`、`GET /api/local/prompts`：返回本地对象 summary 列表。
- `GET /api/local/profiles/<id>`、`GET /api/local/projects/<id>`、`GET /api/local/assets/<id>`、`GET /api/local/prompts/<id>`：返回默认脱敏对象；`GET /api/local/projects/<id>?showPaths=1` 可在明确编辑动作中返回 `rootPath`，默认列表和详情不返回路径。
- `POST /api/local/profiles|projects|assets|prompts`：create，body 为 `{ "data": { ... }, "content": "..." }`；`content` 仅用于 prompt。
- `PUT /api/local/profiles|projects|assets|prompts/<id>`：update，body 为 `{ "revision": 1, "data": { ... }, "content": "..." }`；revision 不匹配时拒绝覆盖。
- `DELETE /api/local/profiles|projects|assets|prompts/<id>`：delete，删除对应 canonical object 目录并更新派生 index。
- `GET /api/local/canvas-projects`：返回本地画布项目 summary 列表。
- `GET /api/local/canvas-projects/<id>`：返回画布项目 canonical document。
- `POST /api/local/canvas-projects`：create，JSON body 为 `{ "data": { ... } }`；multipart body 字段为 `data` JSON 和一个或多个 `file:<file_key>` 文件，文件写入 `canvas-projects/<canvas_id>/files/` 并回填 `data.files[file_key].path`。
- `PUT /api/local/canvas-projects/<id>`：update，JSON body 为 `{ "revision": 1, "data": { ... } }`；multipart body 字段为 `revision`、`data` JSON 和一个或多个 `file:<file_key>` 文件；revision 不匹配时拒绝覆盖；未提交 `data.files` 时保留已有文件 metadata。
- `DELETE /api/local/canvas-projects/<id>`：delete，删除对应 canonical object 目录并更新派生 index。
- `GET /api/local/canvas-projects/<id>/files/<file_key>`：按 `data.files[file_key]` 读取项目内文件，拒绝目录逃逸。
- `GET /api/local/workbench-logs?modality=text|image|video`：返回工作台生成记录 summary 列表。
- `GET /api/local/workbench-logs/<id>`：返回工作台生成记录 canonical document。
- `POST /api/local/workbench-logs`：create，JSON body 为 `{ "data": { ... } }`；multipart body 字段为 `data` JSON 和一个或多个 `file:<media_key>` 文件。
- `DELETE /api/local/workbench-logs/<id>`：delete，删除对应 canonical object 目录并更新派生 index。
- `GET /api/local/workbench-logs/<id>/files/<media_key>`：按 `media[].key` 读取记录内文件，拒绝目录逃逸。
- `GET|POST|PUT /api/local/ai/v1/*`：本地 AI proxy。`opsc serve` 从 workspace profile channel 读取 `baseUrl`，通过 `secretRef` 解析 env/file secret，向 OpenAI-compatible target 注入 `Authorization: Bearer <secret>`；不会转发浏览器传来的 bearer/session/cookie，也不会把 secret 写回响应。当前实现支持 `secretRef.type=env` 和绝对路径 `file`；`keychain/cloud` 保留为后续 resolver。
- `POST /api/local/assets/import`：create asset with file，multipart 字段为 `data` JSON、`file` 和可选 `fileKey`；文件写入 `assets/<asset_id>/files/<fileKey>.<ext>`，metadata 中保存 `files[fileKey]`。
- `PUT /api/local/assets/<id>/import`：update asset with file，multipart 字段为 `revision`、`data` JSON、`file` 和可选 `fileKey`；revision 不匹配时拒绝覆盖。
- `POST /api/local/artifacts/import`：create artifact with file，multipart 字段为 `data` JSON、`file` 和可选 `fileKey`；文件写入 `artifacts/<artifact_id>/files/<fileKey>.<ext>`，metadata 中保存 `files[fileKey]`。
- `PUT /api/local/artifacts/<id>/import`：update artifact with file，multipart 字段为 `revision`、`data` JSON、`file` 和可选 `fileKey`；revision 不匹配时拒绝覆盖。
- `GET /api/local/artifacts/<artifact_id>/files/<file_key>`、`GET /api/local/assets/<asset_id>/files/<file_key>`：按 metadata 中的 `files[file_key]` 读取 workspace 内文件，拒绝目录逃逸。
- `GET /api/local/prompts/<prompt_id>/content`：读取 `content.md`。

Artifact/asset file endpoint 返回原始文件内容；prompt content endpoint 返回 `text/markdown` 原文；run events stream 使用 SSE，不包 JSON envelope。

Runtime metadata：

- `opsc workspace info --json` 可以读取 runtime metadata，但默认不得返回 token、launch secret 或 session id。
- Browser 不得把长期 bearer token 写入 `localStorage`；只使用一次性 launch secret 换 HttpOnly session。

Web UI local adapter 当前约定：

- 顶部导航提供本地工作区连接入口，只保存 loopback `baseUrl`，不保存 bearer token、launch secret 或 session id。
- browser 调 `POST /api/local/bootstrap/session` 后依赖 HttpOnly session cookie；`launch.secret` 用完即清空输入状态。
- `我的素材`、`我的提示词`、画布项目库、工作台 text/image/video 生成记录、电商工作流私有模板、本地 run/artifact 基础记录、工作流入口自定义文件夹和本地项目引用的 canonical data 通过 `opsc serve` 写入 `assets/`、`prompts/`、`canvas-projects/`、`workbench-logs/`、`templates/`、`runs/`、`artifacts/`、`opsc-workspace.json.data.preferences` 和 `projects/`；本地项目列表不得默认展示 `rootPath`，编辑时才可通过 `showPaths=1` 读取路径，含 `credentialRef` 的项目不得用脱敏 summary 写回；本地模板中的 `material_lookup` 固定素材选择应读取同一 workspace 的 image assets，启动本地 run 时固定素材文件应复制为全局 canonical artifact，run 目录只保存 artifact ref；素材包导入必须通过 `opsc serve` 写入 `assets/` 和 asset files，不得把包内媒体恢复到浏览器 `image_files/media_files` 作为事实源；素材新增、更新或删除成功后必须清理当前素材和画布都不再引用的 browser media blob；画布图片/视频节点和助手媒体写入 `canvas-projects/<canvas_id>/files/`，项目 JSON 只保留 `workspaceFileKey` 与 metadata；画布导入/导出 zip 会带上这些 `workspaceFileKey` 文件并在导入后重新上传到目标 workspace，导入桥接用 browser blob 必须清理；浏览器 localforage 只使用 `opsc:asset_store_cache:v1`、`opsc:prompt_store_cache:v1`、`opsc:canvas_store_cache:v1` 作为展示/临时缓存，其中素材/提示词 cache 必须绑定当前 workspace 并不得持久化私有列表或正文，画布缓存不再保存项目列表，工作台历史不再使用旧 generation log key 作为事实源；`opsc:local_workspace_connection` 只能保存 loopback `baseUrl`，旧版本已持久化的 `workspace/runtime` 字段必须通过 store version migration 丢弃；工作流入口不再使用旧 `ops-canvas-workflow-folders` localStorage key 作为事实源；图片/视频工作台保存成功后的当前预览和参考图应从 workbench-log canonical file 读取，保存成功、新会话、移除参考图和切换历史记录时应清理已被 canonical 化的 browser reference blob。
- AI 本地模式通过 `profiles/<profile_id>/profile.json` 保存可共享 channel 配置和 env `secretRef`，Web 请求模型列表、图片、图片编辑、文本问答和视频接口时调用 `/api/local/ai/v1/*` 并依赖 HttpOnly session cookie；浏览器启动时会清空旧 `infinite-canvas:ai_config_store` 内容，不再长期保存 API key；切换到无 profile 或断开的 workspace 时，Web 内存中的 local profile 配置必须清空，避免跨 workspace 串用 Base URL、模型列表或 `SecretRef`。
- Hybrid ecommerce VPS 后端只能由 `opsc` 本地进程调用；Web/watch 正式路径必须使用 profile/channel `secretRef`，显式 env `secretRef` 仅用于 CLI smoke 诊断。浏览器通过 `opsc serve` 创建/查看 local run，不直接调用 VPS admin API，不保存或发送 VPS admin token/cookie/secret。同步回 workspace 的数据仅限远端 run id、状态/阶段进度摘要、append-only events、node state output/metadata 和已下载的 canonical artifact/ref；远端错误文本和 artifact path 进入 workspace 前必须脱敏和相对路径校验，VPS run dir 不进入 local canonical metadata。
- 旧浏览器 key `infinite-canvas:asset_store`、`infinite-canvas:prompt_store`、`infinite-canvas:canvas_store` 不迁移。
- 素材文件读取默认通过带 credentials 的 fetch 转成 `blob:` object URL 后展示，避免把本地 session token 或 workspace 文件路径放进普通业务数据。
- 当前 Web UI 已迁移素材、提示词、画布项目、画布项目内媒体文件、工作台 text/image/video 生成记录、AI 本地直连 profile/proxy、电商工作流私有模板 CRUD、模板 `material_lookup` 固定素材本地选择、工作流入口自定义文件夹、本地 run 列表/状态页/artifact 预览、本地模板创建 run 草稿、固定素材 artifact ref，以及 profile/channel `secretRef` 下的 hybrid run 创建和进度展示。Phase 12 `opsc executor` 已支持 run-once 与 `--watch`，可执行固定本地素材、文本生成、图片生成、条件分支、受 project capability/path guard 约束的本地脚本节点，以及单条已确认电商模板的 hybrid VPS backend；local run 仍为 canonical，远端 PDD run 只做执行后端和 artifact 下载来源；PDD/VPS run 历史迁移、运行时 `material_lookup` 自动匹配本地素材、`image_edit`/`video_generation` 和专用文章/视频/电商 project adapter 仍未完成；现有 PDD/VPS run 仍不是 local workspace canonical data。

## Cloud Boundary

默认不上云：

- 私有模板。
- 私有 run。
- 生成 artifact。
- 个人素材。
- 个人 prompt。
- 本地项目路径。
- 本地日志和事件流。
- API key、token、cookie、session、模型密钥。

允许上云：

- 账号、授权、角色、计费。
- 官方模板、公共模板和用户显式发布的模板。
- 公共素材和后台素材。
- 商用 profile 的模型中转、限流、扣费和审计。
- 用户显式上传、发布或分享的数据。

云端动作必须显式：

- `publish`
- `upload`
- `share`
- `sync --cloud`

禁止隐式后台上传私有 run、artifact、asset、prompt 或项目路径。

## Write Safety

实现写入 workspace 时必须遵守：

- 写 JSON 使用同目录临时文件、flush/fsync 和 atomic rename。
- JSON 使用稳定缩进，便于 diff。
- 写入 run 事件使用 append-only JSONL。
- 写入 artifact 文件后记录 `sha256` 和 `bytes`。
- workspace 写操作通过 runtime/state 目录中的 `workspace.write.lock` 做短时排他锁。
- serve 启动和 runtime 文件更新通过 runtime/state 目录中的 `serve.lock` 做排他锁。
- secrets 不写入普通 JSON、日志、event data 或默认 CLI 输出。
- 删除对象时不得静默删除其它对象仍引用的文件；当前已实现 `opsc workspace gc plan` dry-run，只输出 orphan artifact、broken run/asset refs、缺失 asset file、缺失 prompt content、缺失 canvas-project media file、缺失 workbench-log media file 等候选项，动作固定为 `review`，不删除任何 canonical object 或文件。

## Cache And Export Rules

`cache/`：

- 只保存可重建内容，例如缩略图、远程模板缓存、临时下载。
- 不得作为 canonical data。
- `workspace index rebuild` 必须跳过。
- 可被 `workspace doctor` 建议清理。

`exports/`：

- 只保存用户显式导出的备份或分享包。
- export/share/publish 默认排除 `.opsc/`、`cache/`、`exports/`、token、pid、port、lock、secrets 和本地绝对路径。
- 当前 `opsc workspace export plan --json` 已实现默认规划：包含 canonical 相对路径，排除 `.opsc/`、`cache/`、`exports/`、`index.sqlite`、包含本地 `rootPath` 的 project document、包含 `secretRef.type=file` 的 JSON document 和 symlink。
- 当前 `opsc workspace gc plan --json` 已实现默认 dry-run 规划：只返回相对路径 candidates 和 warnings，不执行删除；candidate 包括无 run/asset 引用的 artifact、指向缺失 artifact 的 run/asset ref、缺失 asset file、缺失 prompt content、缺失 canvas-project media file、缺失 workbench-log media file。
- 如未来需要导出 project path，必须先定义脱敏格式；v1 默认不导出本地绝对路径。

## Compatibility Boundary

### Existing Go API And DB

- 当前后端业务接口继续使用 `docs/api-response.md` 中的 `{ code, data, msg }`。
- `opsc` CLI 使用本 contract 定义的 `{ ok, data, warnings }` / `{ ok:false, error }` envelope；`opsc serve` HTTP API 使用现有 Go API 风格的 `{ code, data, msg }` envelope，以降低后续 Web UI adapter 成本。
- 当前 `repository/db.go` 的 `users`、`credit_logs`、`prompts`、`assets`、`settings`、`workflow_templates`、`workflow_runs` 仍是服务端 DB 表，不自动写入 local workspace。
- 当前 `workflow_templates.spec` 可作为本地 `template.data` 的迁移参考，但 v1 不自动迁移服务端模板。
- 当前 `workflow_runs.run_dir` 指向 VPS/PDD 文件目录；v1 local run 不使用 `runDir` 作为 canonical artifact 位置。

### Existing System Settings

- 当前 `settings.public` 仍是前端公开配置来源。
- 当前 `settings.private.channels[].apiKey` 是服务端私有密钥字段；local workspace profile 必须使用 `secretRef`，不能复制 `apiKey` 明文。
- `allowCustomChannel` 的当前语义是浏览器可否配置本地直连；local workspace profile 后续会成为更稳定的本地直连配置来源。

### Existing Browser Data

- 当前 AI 本地直连配置已迁到 local workspace profile，API key 由 `opsc serve` 根据 `secretRef` 解析；工作台图片/视频结果和参考图保存成功后会从 workspace 文件端点回显，并清理已保存的 browser reference blob；生成中、上传参考图、保存失败或保存过程中用户已切换当前参考图时，临时 Blob 仍可能短暂存在浏览器内存或 IndexedDB。
- 当前画布项目、画布项目内媒体文件、画布导入/导出 zip 内媒体、`我的素材`、`我的提示词`、工作台 text/image/video 生成记录和 AI 本地 profile/proxy 已先迁到 `opsc serve` / local workspace；浏览器只保留 `opsc:*_cache:v1` 缓存、local workspace loopback `baseUrl` 或即时预览/导入桥接临时状态；素材写入成功和画布保存成功后会清理已写入 workspace 且当前状态不再引用的 browser media blob。
- v1 不迁移旧浏览器测试数据。
- 旧画布节点中的 `storageKey` 仍可能是浏览器本地 Blob key；新保存流程会尽量把可读取 Blob 转成 `workspaceFileKey`，但无法读取的旧 Blob 仍可能保留为兼容数据，属于人工清理/重新导入范围。
- `blob:` URL、`content`、`coverUrl`、`dataUrl` 只适合展示，不是长期文件标识。
- 后续 Web UI 本地化时，应继续把 IndexedDB 降级为 UI 缓存或临时状态，canonical data 写入 `opsc serve`。

### Existing VPS/PDD Data

- 当前 PDD run、creative canvas、素材和 prompt 仍在 `PDD_RUNS_ROOT`、`PDD_MATERIALS_ROOT`、`PDD_PROMPTS_ROOT` 等服务端/VPS 路径。
- v1 不迁移现有 PDD VPS run。
- 后续本地化 PDD 时，应作为 `pdd-local` project adapter 接入 local workspace，而不是把 VPS run 目录当成 workspace canonical data。

### Existing Local Agent

- 当前 `cmd/local-agent` 使用 `LOCAL_AGENT_TOKEN` 连接 VPS 控制台领取脚本 job，并限制脚本路径在 `--root` 内。
- `opsc serve` token 不复用 `LOCAL_AGENT_TOKEN`。
- 后续 project `process.exec` capability 应沿用 local-agent 的相对路径和 root escape 防护。

## Phase 1 Implementation Defaults

后续实现阶段默认选择：

- CLI 二进制名：`opsc`。
- 实现位置：优先新增 `cmd/opsc`，复用 Go 项目已有后端能力；如后续改用其它语言，必须保持本 contract 的 CLI 和文件格式不变。
- 最小实现顺序：`workspace init` -> `workspace info --json` -> `workspace doctor` -> 本地模板列表 -> 本地 run/artifact 只读列表 -> 本地写入。
- 当前已实现：`workspace init/info/doctor`、`workspace index rebuild`、`workspace export plan`、`workspace gc plan`、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log file-backed repository、run node state、run artifact ref、run events JSONL/follow、project root fingerprint/path guard、index-backed list/status/artifact/private-object 查询、`opsc serve` loopback HTTP API 和 AI proxy、`opsc executor` run-once/watch worker、project-aware `condition`/`script`/output mapping 最小执行、hybrid ecommerce 单模板导入与 VPS PDD API 执行/状态/阶段进度/artifact 同步、`opsc mcp` stdio 查询/诊断薄封装和 serve-backed index rebuild 工具，以及 Web UI `我的素材` / `我的提示词` / 画布项目库 / 工作台生成记录 / AI 本地 profile / 本地项目引用面板 / 电商工作流私有模板 / 模板固定素材本地选择 / 本地固定素材 artifact ref / 本地 run-artifact 基础 UI / hybrid run 创建和进度展示 local adapter。
- MCP 后续：首版已完成 CLI/core 薄封装和 index rebuild 的 `opsc serve` 调用；canonical object 写入工具、local executor 工具、canvas/workbench 工具尚未实现。

## Document Change Summary

- 明确 `opsc serve` 启动参数、默认端口、鉴权、workspace 外 runtime/state metadata 和 token/session 文件位置。
- 明确 workspace common object envelope、`revision`、event envelope、canonical artifact metadata 和 run 内 artifact ref 规则。
- 明确 profile channel 与 `secretRef` 结构，避免复制现有服务端 `settings.private.channels[].apiKey` 明文。
- 明确 project capability model，并把 `process.exec` 安全边界对齐当前 `cmd/local-agent`。
- 明确 atomic write、lock、index rebuild、cache/exports 排除规则。
- 明确 project root salted fingerprint、path escape 防护、capability gate、GC dry-run plan 和默认不删除策略。
- 明确 Web UI `我的素材` / `我的提示词` / 画布项目库 / 工作台生成记录 / 电商工作流私有模板 adapter、delete/import HTTP API 和浏览器缓存 key 边界。
- 明确 local workspace v1 与现有 DB、VPS/PDD、browser IndexedDB/localforage 和 Go API response 的兼容边界。

## Non-blocking Open Questions

- `secretRef.keychain` 的具体跨平台 keychain backend 需要在实现阶段选择。
- asset 从 artifact 收藏时，v1 默认复制文件还是引用 artifact，需要在写入型 CLI 或 UI 接入前最终确定；当前 GC plan 同时识别 `sourceArtifactId` 引用和 asset 自有 `files/`。
- content-addressed `blobs/` 是否在后续版本加入去重；v1 暂不实现。
- 云端 `publish/share` 的脱敏导出格式需要另开 contract，不在 v1 local-only contract 内完成。
- `opsc serve` 已提供 SSE event stream；如未来增加 WebSocket，应保持同一 event envelope。
- PDD/VPS run 历史迁移、运行时 `material_lookup` 自动匹配本地素材、专用文章/视频/电商 project adapter、真实 worker 安装/自启动、更完整 retry/recovery 和 `image_edit`/`video_generation`/复杂 loop/guardrail 仍需下一阶段细化。

## Phase 0 Acceptance Checklist

Phase 0 完成必须满足：

- 本 contract 已保存到 `docs/local-workspace-v1-contract.md`。
- `docs/local-workspace-data-separation-plan.md` 指向本 contract。
- `docs/ai/` 项目记忆记录该 architecture decision。
- 只修改 Markdown/Mermaid 文档，不修改业务代码。
- `git diff --check` 通过。

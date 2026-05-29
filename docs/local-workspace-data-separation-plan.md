# Local Workspace 数据分离计划

本文档记录 local-first 数据分离的已确认决策和后续落地顺序。目标是把私有模板、运行结果、个人素材、个人 prompt、生成产物和本地项目引用从云端/VPS 存储边界中拆出来，形成后续 CLI、MCP、Web UI 都能复用的本地 workspace 底座。

## 已确认决策

- 默认 workspace 根目录为 `~/OpsCanvas`。
- 支持多个 workspace，CLI 通过 `--workspace` 指定目标目录。
- 项目文件不复制进 workspace，只保存外部路径、类型、权限和 adapter metadata。
- 生成 artifact 复制进 workspace，由系统统一管理。
- secrets 不明文写入普通 JSON；优先使用系统 keychain，开发初期可引用环境变量名。
- `opsc serve` 是 Web UI / MCP 访问本地数据的唯一入口。
- `opsc serve` 即使只监听 `127.0.0.1`，也使用本地随机 token。
- 浏览器旧数据不迁移，现有 IndexedDB/localforage 测试数据直接丢弃。
- 现有 PDD VPS run 不迁移；后续本地化后按新 workspace 结构重新接入。
- CLI 是核心接口，MCP 后续封装 CLI/core，不重复实现业务逻辑。

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
  cache/
  exports/
```

目录职责：

- `opsc-workspace.json`：workspace 元信息，例如 schema version、workspace id、默认 profile。
- `index.sqlite`：查询索引，用于快速列表、搜索、状态查询；不是唯一事实源。
- `profiles/`：本地 profile 配置，例如 local、cloud、hybrid；不直接保存明文 secrets。
- `projects/`：本地项目引用，保存外部项目路径、类型、权限和 adapter metadata。
- `templates/`：私有工作流模板，使用可读、可 diff、可 git 管理的 JSON。
- `runs/`：工作流运行记录，保存 run snapshot、节点状态、事件日志和结果引用。
- `artifacts/`：生成结果，每个 artifact 一个目录，保存 metadata、原始文件和可选缩略图。
- `assets/`：个人素材库，保存用户主动收藏或导入的素材。
- `prompts/`：个人 prompt 库。
- `cache/`：可重建缓存，例如缩略图、远程模板缓存、临时下载。
- `exports/`：导出包，用于备份、分享和迁移。

设计原则：

- JSON、JSONL 和文件目录是事实源。
- `index.sqlite` 只做索引，可通过扫描 workspace 文件重建。
- 浏览器 IndexedDB/localforage 只能作为 UI 缓存或临时状态，不再作为长期事实源。

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
opsc template list --json
opsc run list --json
opsc run status <run_id> --json
opsc run events <run_id> --follow
opsc artifact list --run <run_id> --json
opsc serve
```

CLI 输出约定：

- agent 友好命令都支持 `--json`。
- stdout 输出机器可读 JSON。
- stderr 输出人类日志、进度和诊断信息。
- 默认输出不暴露 secrets。
- 默认输出避免暴露敏感绝对路径；需要时通过显式参数查看。
- 所有核心对象使用稳定 ID，例如 `proj_*`、`tpl_*`、`run_*`、`node_*`、`art_*`、`asset_*`、`prompt_*`。

`opsc serve` 设计约定：

- 只监听本机地址，默认 `127.0.0.1`。
- 启动时生成或读取本地随机 token。
- Web UI 和 MCP 通过 localhost API 访问 workspace。
- 浏览器不直接写 `~/OpsCanvas`。
- workspace 文件统一由 CLI/core/local service 写入，避免并发写坏数据。

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
4. 实现模板、run、artifact 的本地写入。
5. 实现 `opsc serve` 本地 API。
6. Web UI 改为通过 `opsc serve` 访问本地 workspace。
7. 后续再接 MCP，MCP 只封装 CLI/core，不重复业务逻辑。

## 不做事项

- 不迁移浏览器历史测试数据。
- 不迁移现有 PDD VPS run。
- 不把浏览器 IndexedDB/localforage 作为长期事实源。
- 不把私有 run、artifact、asset、prompt 默认上传云端。
- 不在本阶段实现 MCP。
- 不在普通 JSON、日志或默认 CLI 输出中保存或打印 secrets。

## 验收标准

- 文档位于 `docs/local-workspace-data-separation-plan.md`。
- 文档覆盖已确认决策、workspace 目录、数据边界、CLI 设计和落地顺序。
- 本阶段只新增设计文档，不修改业务代码。
- 后续实现时以本文档作为 local-first 数据分离基线。

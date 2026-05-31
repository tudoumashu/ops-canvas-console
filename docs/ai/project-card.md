# 项目卡片

## Project Goal

`ops-canvas-console` 当前是一个以“无限画布 / infinite-canvas”为产品基础的图片创作与工作流控制台。

- fact：README 将产品定义为“面向图片创作的开源工作台”，把画布编排、AI 图片生成、参考图编辑、对话助手、提示词中心和素材沉淀放在同一个界面。
- fact：当前工作区还包含 PDD / 电商工作流控制台、自定义工作流模板画布、视频创作台、算力点与账号体系。
- inferred：本仓库是基于 `basketikun/infinite-canvas` 的项目化分支，当前 remote 指向 `tudoumashu/ops-canvas-console.git`，用途更偏向用户自己的 PDD / 电商工作流控制台。

## Product Scope

- 用户侧：无限画布、图片/文本/视频创作台、画布助手、提示词中心、素材中心、工作流入口。
- 管理侧：用户、算力点流水、系统设置、模型渠道、提示词库、素材库、PDD 工作流模板与运行控制。
- 工作流侧：读取 VPS 上 `PDD_RUNS_ROOT` 下的 run 产物，展示 DAG、商品流程、创作画布、实时日志，并允许管理员执行受控 allowlist 动作。
- 本地 agent：`cmd/local-agent` 可从 VPS 控制台领取 `script` 节点任务，在本地指定 root 内执行脚本后回传输出。

## Tech Stack

- Frontend：Next.js App Router、React、TypeScript、Ant Design、Tailwind CSS、Zustand、TanStack Query、localforage。
- Backend：Go、Gin、GORM、JWT、cron。
- Storage：SQLite / MySQL / PostgreSQL；浏览器 IndexedDB/localforage；服务器本地文件系统；PDD workflow run 目录。
- Packaging：Docker multi-stage build，Bun 构建/运行 Next.js，Go 编译 API server。
- CI/CD：GitHub Actions 在 tag `v*` 或手动触发时构建并推送 GHCR Docker image。

## Repo

```text
repo: /home/shiyi/Apps/VScode/auto-workflow/ops-canvas-console
remote: https://github.com/tudoumashu/ops-canvas-console.git
main branch: main
```

## Main Entry Points

- Backend process：`main.go`
- API router：`router/router.go`
- Config：`config/config.go`
- Database：`repository/db.go`
- Frontend app：`web/src/app/layout.tsx`
- Frontend API proxy：`web/src/app/api/[...path]/route.ts`
- Frontend API clients：`web/src/services/api/`
- Canvas state：`web/src/app/(user)/canvas/stores/use-canvas-store.ts`
- Browser stores：`web/src/stores/`
- Local agent：`cmd/local-agent/main.go`

## External Services

- OpenAI-compatible model APIs：通过用户本地直连或后端远程渠道调用。
- Flow2API：后端可把图片/视频请求适配到 `/chat/completions` 式媒体返回。
- Linux.do OAuth：可选第三方登录。
- GitHub prompt repositories：内置远程提示词源由后台/cron 同步。
- Render：README 和 `render.yaml` 提供一键部署入口。
- GHCR：GitHub Actions 推送 Docker image。
- PDD workflow VPS：默认路径 `/opt/pdd-workflow`，控制台读取 run、素材、prompt 和执行受控动作。

## Current Status

- status：active
- fact：最近 Git 历史和 `CHANGELOG.md` 显示 2026-05 下旬仍在密集开发视频、账号/算力点、PDD 工作流和 Flow2API 适配。
- fact：Phase 0 已接受并补全 local workspace v1 设计 contract，见 `docs/local-workspace-v1-contract.md` 和 `docs/ai/decisions/ADR-0002-local-workspace-v1-contract.md`；Phase 6 已新增并加固 `internal/localworkspace` foundation、template/run/artifact/profile/project/asset/prompt/canvas-project/workbench-log file-backed repository、run artifact ref、run node state、run events、project root salted fingerprint、project path capability guard、`index.sqlite` 增量更新/扫描重建、默认 export plan 排除规则、GC dry-run plan，以及 `cmd/opsc` 的 `workspace init/info/doctor/index rebuild/export plan/gc plan`、`template list`、`run list/status/events`、`artifact list`、`profile list`、`project list`、`asset list`、`prompt list`、`serve` 和 `mcp`。`opsc serve` 当前使用 workspace 外 XDG state runtime、HTTP bearer token、browser 一次性 launch secret + HttpOnly session，并已暴露 profiles/projects/assets/prompts/canvas-projects/workbench-logs、local templates 和 workspace preferences 的本地查询、写入、删除、asset/canvas-project/workbench media import/read API 和 `/api/local/ai/v1/*` AI proxy；`opsc mcp` 当前是 stdio 薄封装，读取/诊断/dry-run 工具映射到现有 CLI JSON 命令，唯一维护写工具 `opsc_workspace_index_rebuild` 经 active `opsc serve` 重建派生索引；Phase 7 已把 Web UI 的 `我的素材`、`我的提示词`、画布项目库、画布项目内图片/视频媒体、画布导入/导出 zip 媒体文件、工作台 text/image/video 生成记录、AI 本地 profile/proxy、电商工作流私有模板和工作流入口自定义文件夹先接入 local workspace，图片/视频工作台结果保存成功后会从 workspace 文件端点回显。
- fact：Phase 8 只做 local workspace 稳定化验证和文档同步，未迁移 PDD/VPS run、未扩大 MCP 写能力；新增回归覆盖 `opsc serve` auth/redaction/session 文件权限、AI proxy `secretRef` 与浏览器 header 隔离、本地模板草稿 run 到 canonical artifact ref happy path，并以 `go test ./cmd/opsc` 继续覆盖 MCP stdio wrapper smoke。Phase 13 已在 `opsc executor` run-once MVP 上补齐 project-aware `condition`/`script`、hybrid ecommerce VPS backend、`opsc executor --watch` worker/runtime 模式、Web local hybrid run profile/channel `secretRef` 路径、远端阶段进度同步、canonical artifact/ref 回写、doctor 可诊断建议、安装/自启动文档和可复用浏览器 smoke 入口；现有 PDD/VPS run 历史仍不迁移。
- unknown：当前线上生产部署的真实域名、数据持久化策略、PDD VPS 当前健康状态、模型渠道和密钥配置无法从仓库证明。

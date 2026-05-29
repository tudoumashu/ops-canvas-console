# AI 项目记忆变更记录

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

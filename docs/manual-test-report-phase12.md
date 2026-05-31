# Phase 12 Manual Test Report

## Scope

Phase 12 收口 Hybrid Ecommerce Web + worker 黄金路径：local workspace 继续是 canonical source；VPS API 只作为执行后端；浏览器只连接 `opsc serve`，不持有、不保存、不直连 VPS credential。

## Automated Evidence

- Go integration：`go test ./internal/localworkspace ./cmd/opsc` 覆盖 `opsc executor --watch`、hybrid 远端非终态重复同步、阶段进度写入 node output/metadata、artifact/ref 写回、resume 去重、secret 不泄露，以及 CLI watch JSON stream。
- TypeScript：`cd web && npx tsc --noEmit` 覆盖 Web local adapter 类型改动。
- Python helper：`python3 -m py_compile tools/hybrid_ecommerce_browser_smoke.py tools/local_workspace_browser_smoke.py tools/hybrid_ecommerce_vps_smoke.py` 覆盖 smoke helper 语法。
- Browser smoke：`tools/hybrid_ecommerce_browser_smoke.py` 已在临时 workspace 中通过，覆盖真实浏览器 bootstrap session、`opsc serve` profile/template fixture、模板编辑页“运行模板”、`opsc executor --watch`、fake VPS dispatch/sync、run 状态页 success、artifact modal 预览和 `localStorage` credential 脱敏。

## Fake Browser Smoke Helper

新增 `tools/hybrid_ecommerce_browser_smoke.py`。该 helper 会启动 fake VPS API，通过真实浏览器和 `opsc serve` 创建 profile 与 hybrid template fixture，随后在真实模板编辑页点击“运行模板”发起 local run，再启动 `opsc executor --watch` 等待 run 状态页到达终态并打开 artifact 预览，同时检查浏览器 `localStorage` 不包含 fake credential material。

本轮已启动 Next dev server、`opsc serve` 和 Chrome 执行该 helper。结果：PASS；template `tpl_01KSY0SN40JBS9218TJTH7ZWV7`、profile `profile_01KSY0SN1EENESQ0R9NXKCV5HM`、run `run_01KSY0ST8268A4DZ1ZQV7DAAR1`；fake VPS overview 调用 2 次，run 状态页到达 `success`，artifact 预览 modal 可见。执行中没有把 launch secret、bearer token、fake VPS token 或 workspace/project 绝对路径写入报告。

## Real VPS Status

Phase 11 已完成 headless 真实 VPS smoke，证据见 `docs/manual-test-report-phase11.md`。Phase 12 又使用同一条已确认 remote template/API 黄金路径做了一次真实 Web UI + `opsc executor --watch` smoke：

- 本地临时 workspace 通过 `opsc serve` 建立 browser bootstrap session。
- Web UI 打开已导入的 hybrid ecommerce local template，在模板编辑页点击“运行模板”创建 local run。
- `opsc executor --watch --poll-interval=5s` 通过 workspace profile/channel `secretRef` 调用 VPS API，浏览器没有接触 VPS token/cookie/secret。
- 远端 run 同步到 local run `success`，本地 node state 记录阶段进度，关键 artifact 导入 canonical artifacts 并写入 run artifact refs。
- 本地 run 状态页刷新后显示 `success`，artifact preview modal 可打开；浏览器 `localStorage` 检查未发现 VPS token、bearer token、launch secret、runtime token 文件名或 configured secret env name。

结果：PASS。为避免在仓库文档中固化远端模板 ID、VPS 细节或凭证材料，本节只记录脱敏结论；临时 evidence 保留在本机 smoke 工作目录，不作为仓库事实源。

## Remaining Manual Checklist

- 真实长期个人 workspace 下重复一条低成本 hybrid run，确认长期 workspace 中的历史本地数据不会影响 template/run/artifact 回显。
- 在非临时浏览器 profile 中 spot check local workspace 连接状态、刷新回显和 `localStorage` 脱敏。
- 后续 worker 安装/自启动产品化完成后，再验证系统重启后的 watch worker 恢复行为。

## Non-goals

- 不迁移历史 PDD/VPS run。
- 不把 VPS run dir 作为 canonical source。
- 不扩大 MCP 写面。
- 不新增通用远程模板平台。
- 不实现 `image_edit`、`video_generation`、复杂 loop/guardrail 或完整 Full GC。

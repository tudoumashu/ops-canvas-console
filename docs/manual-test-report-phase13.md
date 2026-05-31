# Phase 13 Operational Hardening 验证报告

## 范围

本阶段只收口已打通的 hybrid ecommerce Web + `opsc executor --watch` 黄金路径：

- local workspace 继续是 canonical source；
- VPS API 继续只是执行后端；
- 浏览器继续只连接 `opsc serve`，不保存、不持有、不直连 VPS credential；
- 不迁移旧 PDD/VPS run，不扩大 MCP 写面，不做 Full GC。

## 自动化结果

- PASS：`go test ./internal/localworkspace ./cmd/opsc`
  - 覆盖 executor watch runtime metadata、单 worker lock、取消后 runtime 清理。
  - 覆盖 `workspace doctor` 的 index freshness、stale executor worker 和 hybrid run 等待 executor 修复建议。
  - 继续覆盖 Phase 8/9/10/11/12 的 local workspace、MCP、executor、hybrid ecommerce 回归。
- PASS：`python3 -m py_compile tools/local_workspace_browser_smoke.py tools/hybrid_ecommerce_browser_smoke.py tools/hybrid_ecommerce_vps_smoke.py`
  - 确认 smoke helper 语法可用。

## 本轮收口复核

- PASS：再次使用 Docker `golang:1.25-alpine` 运行 `go test ./internal/localworkspace ./cmd/opsc`，确认 executor watch、doctor、hybrid sync 和既有 local workspace 回归仍通过。
- PASS：再次运行 `python3 -m py_compile tools/local_workspace_browser_smoke.py tools/hybrid_ecommerce_browser_smoke.py tools/hybrid_ecommerce_vps_smoke.py`。
- FIXED：`docs/local-workspace-regression.md` 中真实 VPS smoke 示例已对齐当前 helper 参数：`--remote-url`、`--remote-template`、`--secret-env`、`--input-file`、`--evidence`。

## 半自动回归入口

已补 `docs/local-workspace-regression.md`：

- `tools/local_workspace_browser_smoke.py` 支持 `--user-data-dir` 和 `--evidence`，用于非临时浏览器 profile 的 local workspace adapter 回归。
- `tools/hybrid_ecommerce_browser_smoke.py` 支持 `--user-data-dir`、`--success-timeout-ms` 和 `--evidence`，用于 fake VPS + 真实 Web UI + `opsc executor --watch` 回归。
- `tools/hybrid_ecommerce_vps_smoke.py` 保持真实 VPS headless smoke 入口，真实 secret 通过环境变量读取，不写入 evidence。

## 本轮半自动验收

- PASS：当前机器未发现已有 `~/OpsCanvas` 或其它 `opsc-workspace.json`，因此无法直接验证用户历史长期 workspace 数据。本轮改用隔离的非 `/tmp` 长期回归 workspace 和非临时浏览器 profile 执行 smoke，避免污染个人数据。
- PASS：`tools/local_workspace_browser_smoke.py` 使用非临时 browser profile 通过，evidence 写入 `~/.local/share/opsc-phase13-regression/local-browser-smoke.json`；结果包含 `ok=true`、`persistentProfile=true`，并完成 local template/run/artifact 创建、run 状态页回显和 artifact 预览。
- PASS：`tools/hybrid_ecommerce_browser_smoke.py` 使用非临时 browser profile、fake VPS API 和真实 `opsc executor --watch` 通过，evidence 写入 `~/.local/share/opsc-phase13-regression/hybrid-browser-smoke.json`；结果包含 `ok=true`、`persistentProfile=true`、`overviewCalls=2`，并完成 Web UI 发起 run、worker dispatch/sync、run success 和 artifact 预览。
- PASS：上述回归 workspace 的 `workspace doctor` 报告 `errors=0`、`warnings=0`，并确认已写入 2 个 templates、2 个 runs、2 个 artifacts 和 1 个 profile。浏览器 smoke 已检查 `localStorage` 未保存 launch secret、bearer token、runtime token 文件名或 hybrid fake credential。
- PASS：browser smoke helper 已增强为前后快照校验：执行前记录既有 templates、runs、artifacts、profiles 的计数和少量 id，执行后确认计数不回退并重新读取既有 id；这条证据链可直接用于用户真实长期 workspace 的半自动验收。
- PASS：增强后的 helper 已在同一隔离长期回归 workspace 复跑通过。`local-browser-smoke-v2.json` 从 2 templates / 2 runs / 2 artifacts / 1 profile 增长到 3 / 3 / 3 / 1；`hybrid-browser-smoke-v2.json` 从 3 / 3 / 3 / 1 增长到 4 / 4 / 4 / 2，并重新读取了执行前的抽样 id。复跑后的 `workspace doctor` 报告 `errors=0`、`warnings=0`。

## 剩余人工确认项

- 当前机器未发现用户真实长期 workspace，因此未能在用户真实历史模板/run/artifact 数据上执行 smoke。已提供可重复命令和 evidence 输出路径；后续用户指定真实 workspace/profile 后，可直接按 `docs/local-workspace-regression.md` 重跑并对比 evidence。
- 未实际安装 `systemd --user` service；已在 `docs/opsc-installation.md` 提供 unit 示例和排查命令，仍需在目标机器上人工确认自启动与日志。

## 结论

Phase 13 的代码级 hardening、文档化安装路径和可复用 smoke 入口已完成；本轮还用非临时浏览器 profile 和隔离长期回归 workspace 验证了 local workspace UI 与 hybrid Web + `opsc executor --watch` 黄金路径。剩余风险集中在用户真实历史 workspace 和真实自启动环境的人工确认。

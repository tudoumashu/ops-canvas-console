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

## 半自动回归入口

已补 `docs/local-workspace-regression.md`：

- `tools/local_workspace_browser_smoke.py` 支持 `--user-data-dir` 和 `--evidence`，用于非临时浏览器 profile 的 local workspace adapter 回归。
- `tools/hybrid_ecommerce_browser_smoke.py` 支持 `--user-data-dir`、`--success-timeout-ms` 和 `--evidence`，用于 fake VPS + 真实 Web UI + `opsc executor --watch` 回归。
- `tools/hybrid_ecommerce_vps_smoke.py` 保持真实 VPS headless smoke 入口，真实 secret 通过环境变量读取，不写入 evidence。

## 本轮未执行的人工项

- BLOCKED：未在用户真实长期 `~/OpsCanvas` 和用户日常浏览器 profile 上执行 smoke，以避免在未知个人数据目录中写入测试模板/run/artifact。已提供可重复命令和 evidence 输出路径，后续由用户选择真实 workspace/profile 后执行。
- BLOCKED：未实际安装 `systemd --user` service；已在 `docs/opsc-installation.md` 提供 unit 示例和排查命令，仍需在目标机器上人工确认自启动与日志。

## 结论

Phase 13 的代码级 hardening、文档化安装路径和可复用 smoke 入口已完成。剩余风险集中在真实长期个人 workspace、非临时浏览器 profile 和真实自启动环境的人工确认。

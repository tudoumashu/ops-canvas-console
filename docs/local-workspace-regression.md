# Local Workspace 回归入口

本文档记录 Phase 13 之后可复用的本机/CI smoke helper。它们只验证 local workspace 与 Web UI/worker 的黄金路径，不迁移旧 PDD/VPS run，不扩大 MCP 写能力。

## 通用前置条件

- 已构建或可运行 `opsc`。
- 已启动 Web UI，例如 `http://127.0.0.1:3000`。
- 已启动 `opsc serve --workspace <workspace> --origin <web-url>`。
- 已安装 Python Playwright，并且本机有 Chrome/Chromium。
- 浏览器 bootstrap 使用 `opsc serve` 本次生成的 `launch.secret`；secret 不写入 evidence。

浏览器 helper 支持：

- `--user-data-dir <dir>`：使用非临时浏览器 profile，适合验证真实长期浏览器状态不会污染 local workspace adapter。
- `--evidence <file>`：写入 JSON evidence，成功退出码为 `0`，失败退出码为 `1`。
- 对已有 workspace，browser helper 会在执行前后记录 templates、runs、artifacts、profiles 的计数和少量既有 id，并在结束前重新读取这些既有 id，确认历史对象仍可访问且计数没有回退。

## 本地 workspace UI smoke

```bash
python3 tools/local_workspace_browser_smoke.py \
  --web-url http://127.0.0.1:3000 \
  --serve-url http://127.0.0.1:17680 \
  --launch-secret "$(cat <state-dir>/launch.secret)" \
  --user-data-dir ~/.cache/opsc/browser-smoke-profile \
  --evidence /tmp/opsc-local-browser-smoke.json
```

覆盖内容：

- browser bootstrap session；
- 通过 `opsc serve` 创建本地 template/run/artifact；
- 真实 run 状态页读取 success；
- artifact modal 预览；
- `localStorage` 不包含 launch secret、bearer token 文件名或 runtime token 字段。
- evidence 中包含 `historyBefore` / `historyAfter`，用于确认既有本地对象没有被 smoke 破坏。

## Hybrid ecommerce UI smoke

```bash
python3 tools/hybrid_ecommerce_browser_smoke.py \
  --workspace ~/OpsCanvas \
  --web-url http://127.0.0.1:3000 \
  --serve-url http://127.0.0.1:17680 \
  --launch-secret "$(cat <state-dir>/launch.secret)" \
  --opsc-bin ~/.local/bin/opsc \
  --user-data-dir ~/.cache/opsc/hybrid-browser-smoke-profile \
  --evidence /tmp/opsc-hybrid-browser-smoke.json
```

该 helper 会启动 fake VPS API，Web UI 通过真实模板编辑页发起 hybrid run，`opsc executor --watch` 负责 dispatch/sync，最后验证 run 状态页、artifact 预览和浏览器持久化脱敏。fake credential 只放在 executor 子进程环境变量中。

执行前后同样会校验既有 templates、runs、artifacts、profiles 仍可访问，并把 `historyBefore` / `historyAfter` 写入 evidence。

## 真实 VPS smoke

```bash
python3 tools/hybrid_ecommerce_vps_smoke.py \
  --workspace ~/OpsCanvas \
  --remote-url <vps-api-base-url> \
  --remote-template <confirmed-template-id> \
  --secret-env <env-name> \
  --input-file /path/to/hybrid-input.json \
  --evidence /tmp/opsc-hybrid-vps-smoke.json
```

退出码约定：

- `0`：smoke 通过。
- `1`：调用链路失败。
- `2`：缺少必要参数、环境变量或前置条件。

真实 VPS smoke 只应使用低成本输入，不调用 host 级运维动作，不把 token/cookie/远端绝对路径写入 evidence 或文档。

## 长期 workspace 半自动验收

对真实 `~/OpsCanvas` 和非临时浏览器 profile，建议按顺序执行：

1. `opsc workspace doctor --workspace ~/OpsCanvas --json`，若提示 index stale，先运行 `opsc workspace index rebuild --workspace ~/OpsCanvas --json`。
2. 启动或确认 `opsc serve` 与 `opsc executor --watch` 正在运行。
3. 使用 `--user-data-dir` 运行本地 workspace UI smoke。
4. 使用 `--user-data-dir` 运行 hybrid ecommerce UI smoke。
5. 检查 evidence 中 `ok=true`，浏览器 `localStorage` 未保存 secret/token/runtime 文件名，`historyAfter` 中 templates、runs、artifacts、profiles 计数不小于 `historyBefore`，且 helper 未报告既有 id 读取失败。

若 `workspace doctor` 提示 executor worker stale，重启 `opsc executor --watch`；若 hybrid run 长时间处于 pending/running，先查看 `opsc run events <run_id>`，再检查 credential/profile/channel 和 VPS API 可用性。

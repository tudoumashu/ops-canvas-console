# Phase 10 手工验证记录

## 范围

本轮目标是收口 Local Workflow Executor 的 project-aware MVP，不迁移现有 PDD/VPS run，不扩大 MCP 写面，不新增 canonical object 类型。

## 已自动化覆盖

- Docker `go test ./internal/localworkspace ./cmd/opsc` 已通过。
- `cd web && npx tsc --noEmit` 已通过。
- `python -m py_compile tools/local_workspace_browser_smoke.py` 已通过。
- `git diff --check` 已通过。
- `opsc executor` 领取 `run.waiting_for_executor` 的 local run。
- `condition` 节点、`script` 节点、`source/target` edge fallback、`fromHandle`/condition 路由跳过。
- run `projectId` 读取 `projects/<proj_id>/project.json`，并执行 adapter、root fingerprint、capability、path safety 和 `artifact.write` 校验。
- 节点级 retry、project output mapping、project root 脱敏、secret 不进入 node output/event。
- executor 写入后的 `index.sqlite` rebuild、run status、events 和 artifact refs 兼容。

## 浏览器 Smoke

已新增脚本：`tools/local_workspace_browser_smoke.py`。

用途：在真实浏览器里使用已有 `opsc serve --origin <web-url>` 与 Next dev server，验证 browser bootstrap session、本地模板/run 状态页和 artifact 预览。

本轮已用临时 workspace 执行通过：

```bash
python tools/local_workspace_browser_smoke.py \
  --web-url http://127.0.0.1:3000 \
  --serve-url http://127.0.0.1:17680 \
  --launch-secret <launch.secret>
```

结果：

```json
{"ok": true, "runId": "run_01KSWMHAKRHETP87ME7GSAW9NQ", "templateId": "tpl_01KSWMHA9E78AKX86H2TSPR0X2"}
```

验证内容：

- browser session 使用一次性 `launch.secret` bootstrap。
- 浏览器上下文中通过 `opsc serve` 创建本地模板与本地 run。
- 打开真实 `/workflows/ecommerce/<run_id>` 状态页，先观察 pending，再写入 success。
- 写入 canonical image artifact 与 run artifact ref 后，在状态页点击“预览”并等待 modal 图片出现。
- smoke 期间 Next dev server 因本机未启动旧 Go API `127.0.0.1:8080` 对 `/api/settings` 返回 502；该错误不影响 local workspace browser smoke 的通过结论。

后续手工命令示例：

```bash
python tools/local_workspace_browser_smoke.py \
  --web-url http://127.0.0.1:3000 \
  --serve-url http://127.0.0.1:17680 \
  --launch-secret <launch.secret>
```

## 结论

Phase 10 executor core 和最小 Web UI local workspace smoke 已通过；真实模型账号、真实长期个人 workspace、专用文章/视频/电商 project adapter 和完整浏览器回归仍保留为后续人工回归项。

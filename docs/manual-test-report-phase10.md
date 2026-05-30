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

本轮未执行真实浏览器 smoke。原因是当前收口优先使用 Go 集成测试覆盖 executor 语义，未在本地同时启动持久 `opsc serve`、Next dev server 和真实浏览器会话。

后续手工命令示例：

```bash
python tools/local_workspace_browser_smoke.py \
  --web-url http://127.0.0.1:3000 \
  --serve-url http://127.0.0.1:17680 \
  --launch-secret <launch.secret>
```

## 结论

Phase 10 executor core 可通过自动化收口；真实浏览器 session、真实 local workspace、真实模型账号和真实项目 adapter 仍保留为后续人工回归项。

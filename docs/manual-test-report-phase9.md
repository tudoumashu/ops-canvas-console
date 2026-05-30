# Phase 9 手工验收报告

测试日期：2026-05-30
测试范围：Local Workflow Executor MVP 的 fake provider 最小 happy path。仅验证本地 CLI / canonical workspace 链路，不执行真实外部模型账号 live call，不迁移 PDD/VPS run。
结论：CLI 级 fake provider happy path 通过；真实浏览器 session 与 Web UI 端到端仍保留为后续人工回归项。

## 测试环境

- 仓库：`/home/shiyi/Apps/VScode/auto-workflow/ops-canvas-console`
- 临时测试目录：`/tmp/opsc-phase9-manual`
- 临时 workspace：`/tmp/opsc-phase9-manual/workspace`
- Fake OpenAI-compatible server：`http://127.0.0.1:19091`
- Executor 入口：`/tmp/opsc-phase9-manual/bin/opsc executor --workspace /tmp/opsc-phase9-manual/workspace`
- 外部 live call：未执行；fake provider 覆盖 `/v1/chat/completions` 和 `/v1/images/generations`

证据文件：

- `/tmp/opsc-phase9-manual/evidence-seed.json`
- `/tmp/opsc-phase9-manual/evidence-executor.json`
- `/tmp/opsc-phase9-manual/evidence-status.json`
- `/tmp/opsc-phase9-manual/evidence-artifacts.json`
- `/tmp/opsc-phase9-manual/evidence-events.jsonl`
- `/tmp/opsc-phase9-manual/evidence-index-rebuild.json`
- `/tmp/opsc-phase9-manual/evidence-summary.json`
- `/tmp/opsc-phase9-manual/fake-provider-requests.jsonl`

## 执行命令

```bash
OPSC_PHASE9_FAKE_KEY=<fake test secret> \
  /tmp/opsc-phase9-manual/bin/opsc executor \
  --workspace /tmp/opsc-phase9-manual/workspace \
  --run <run_id> \
  --json

/tmp/opsc-phase9-manual/bin/opsc run status <run_id> \
  --workspace /tmp/opsc-phase9-manual/workspace \
  --json

/tmp/opsc-phase9-manual/bin/opsc artifact list \
  --run <run_id> \
  --workspace /tmp/opsc-phase9-manual/workspace \
  --json

/tmp/opsc-phase9-manual/bin/opsc run events <run_id> \
  --workspace /tmp/opsc-phase9-manual/workspace

/tmp/opsc-phase9-manual/bin/opsc workspace index rebuild \
  --workspace /tmp/opsc-phase9-manual/workspace \
  --json
```

## 验收结果

| 项 | 结果 | 证据 |
| --- | --- | --- |
| 创建 local workspace、profile、固定图片素材、template、pending run 和 `run.waiting_for_executor` event | PASS | `evidence-seed.json` |
| `opsc executor` 领取 run 并执行 `input`、fixed `material_lookup`、`text_generation`、`image_generation` | PASS | `evidence-executor.json` |
| run 状态变为 `success`，4 个 node state 均为 `success` | PASS | `evidence-status.json` |
| 固定素材、文本输出、图片输出写入 canonical artifact，并通过 run artifact ref 关联 | PASS | `evidence-artifacts.json` |
| events append-only，包含 `run.waiting_for_executor`、`executor.run.claimed`、`executor.node.started/completed`、`executor.run.completed` | PASS | `evidence-events.jsonl` |
| fake provider 收到 `/v1/chat/completions` 与 `/v1/images/generations`，未收到 browser cookie | PASS | `fake-provider-requests.jsonl` |
| `workspace index rebuild` 后仍可重建派生索引 | PASS | `evidence-index-rebuild.json` |

关键摘要：

```json
{
  "executorProcessed": 1,
  "executorStatus": "success",
  "runStatus": "success",
  "nodeStatuses": ["success", "success", "success", "success"],
  "artifactCount": 3,
  "latestEventSequence": 14,
  "providerPaths": ["/v1/chat/completions", "/v1/images/generations"],
  "providerAuthOk": [true, true],
  "providerCookies": [null, null]
}
```

## 限制与剩余项

- 本轮未启动真实 Web UI 浏览器 session；未覆盖从模板编辑器点击“运行模板”到 run 状态页自动轮询的浏览器交互。
- 本轮未使用真实 OpenAI-compatible 账号；不会证明真实供应商参数、额度、延迟和错误格式。
- 本轮不测试 `image_edit`、`video_generation`、condition、script、复杂 loop、project adapter、完整 retry/cancel 或 PDD/VPS run 迁移。

## 结论

Phase 9 executor MVP 的本地 CLI / canonical workspace / fake provider happy path 可通过。剩余风险集中在真实浏览器 session、真实 workspace Web UI 操作、真实模型供应商差异和下一阶段 project adapter 接入。

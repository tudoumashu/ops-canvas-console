# Phase 8 手工验收报告

测试日期：2026-05-30
Phase 8.1 复验日期：2026-05-30
测试范围：执行 `docs/pending-test.md` 中 Phase 8 剩余手工验收项，不执行真实外部 OpenAI 兼容服务 Key 的 live call；Phase 8.1 只复验 MCP 目标项。
结论：A-F 通过，Phase 8 可关闭；剩余真实 agent 客户端展示层 spot check、真实模型账号 live call、提示词显式导入/导出 UI 确认均为非阻塞后续项。

## 测试环境

- 仓库：`/home/shiyi/Apps/VScode/auto-workflow/ops-canvas-console`
- 临时测试目录：`/tmp/opsc-phase8`
- Web：`http://127.0.0.1:13081`
- Backend API：`http://127.0.0.1:18180`
- `opsc serve` 工作区 A：`http://127.0.0.1:17680`
- Fake OpenAI-compatible server：`http://127.0.0.1:19090`
- 浏览器测试：Playwright / Chromium，使用一次性 browser profile 与一次性 workspace
- 外部 live call：未执行，按本轮范围只使用 fake OpenAI-compatible server 覆盖代理、模型列表、图片和视频链路
- Phase 8.1 MCP 复验目录：`/tmp/opsc-phase8-1`

主要启动命令：

```bash
node ./node_modules/.bin/next dev --webpack -H 127.0.0.1 -p 13081
docker run --rm --network host ... PORT=18180 ... go run .
XDG_STATE_HOME=/tmp/opsc-phase8/state /tmp/opsc-phase8/bin/opsc serve --workspace /tmp/opsc-phase8/workspaces/A --port 17680 --origin http://127.0.0.1:13081
XDG_STATE_HOME=/tmp/opsc-phase8/state /tmp/opsc-phase8/bin/opsc --workspace /tmp/opsc-phase8/workspaces/A mcp
XDG_STATE_HOME=/tmp/opsc-phase8-1/state /tmp/opsc-phase8-1/bin/opsc --workspace /tmp/opsc-phase8-1/workspace mcp
```

证据目录：

- JSON 证据：`/tmp/opsc-phase8/evidence-*.json`
- 截图：`/tmp/opsc-phase8/screenshots/`
- fake provider 请求日志：`/tmp/opsc-phase8/fake-openai-requests.jsonl`
- Phase 8.1 MCP 复验证据：`/tmp/opsc-phase8-1/evidence-F-mcp-phase8-1.json`
- Phase 8.1 MCP 目标重跑证据：`/tmp/opsc-phase8-1-rerun/evidence-F-mcp-rerun.json`

## 验收结果

| 项 | 结果 | 证据 |
| --- | --- | --- |
| A. 本地连接、鉴权、运行时文件、旧本地数据清理 | PASS | `/tmp/opsc-phase8/evidence-A.json`、`/tmp/opsc-phase8/evidence-A-disconnect.json` |
| B. workspace 切换与 profile 隔离 | PASS | `/tmp/opsc-phase8/evidence-B1-profile-A.json`、`/tmp/opsc-phase8/evidence-B2-switch-B.json`、`/tmp/opsc-phase8/evidence-B3-switch-back-A.json` |
| C. 素材、提示词、画布、workbench | PASS，提示词显式导入/导出 UI 未覆盖 | `/tmp/opsc-phase8/evidence-C-local-data-workbench.json` |
| D. 项目面板、preferences、断连阻断 | PASS | `/tmp/opsc-phase8/evidence-D-projects-preferences.json` |
| E. 模板、run、artifact、workspace 缓存隔离 | PASS | `/tmp/opsc-phase8/evidence-E-templates-runs-artifacts.json` |
| F. MCP 只读 client 回归 | PASS | 原始失败证据：`/tmp/opsc-phase8/evidence-F-mcp.json`；Phase 8.1 复验证据：`/tmp/opsc-phase8-1/evidence-F-mcp-phase8-1.json`；目标重跑证据：`/tmp/opsc-phase8-1-rerun/evidence-F-mcp-rerun.json` |

## 关键检查记录

### A. 连接与本地清理

- 覆盖未启动服务、等待授权、错误 launch secret、正确 launch secret、连接成功、断连后重连。
- `opsc_session` cookie 为 `HttpOnly`、`SameSite=Lax`。
- runtime token 与 launch secret 保存在本地运行时文件中，前端 `localStorage` 未保存明文。
- 旧 `infinite-canvas:*`、generation logs、workflow folders 已清理。
- `opsc:*_cache:v1` 与临时 bridge media 未被误删。

### B. Workspace 切换

- 工作区 A 保存 profile，`secretRef` 指向环境变量 `PHASE8_OPENAI_KEY`，profile JSON 未写入明文 key。
- 切换到工作区 B 后没有出现 A 的 profile/config，默认值回到 `https://api.openai.com` 与 `OPENAI_API_KEY`。
- 切回工作区 A 后恢复 A 的 OpenAI-compatible base URL 与 env secretRef。

### C. 本地数据与 Workbench

- 素材覆盖 text/image/video 的创建、更新、删除、导入、导出、公有复制和文件落盘。
- 提示词覆盖创建、更新、删除、`content.md` 落盘和公有复制。
- 画布覆盖创建、更新、删除和持久化。
- Workbench 覆盖文本、图片、视频生成记录保存、刷新、删除与临时 blob 清理。
- fake provider 覆盖 `/v1/models`、`/v1/chat/completions`、`/v1/images/edits`、`/v1/videos`、`/v1/videos/{id}`、`/v1/videos/{id}/content`。
- 限制：提示词中心未发现显式导入/导出 UI，本次只能验证 CRUD、文件落盘和公有复制。

### D. 项目与 Preferences

- 项目 UI 创建、编辑、删除通过。
- 默认项目列表不返回 `rootPath`；`showPaths=1` 时返回路径。
- `rootFingerprint` 与 capabilities 存在。
- 含 `credentialRef` 的项目在 UI 中展示警告，并阻止保存脱敏信息回写。
- workflow folder preference 保存、刷新持久化通过。
- revision conflict 返回 422。
- 断连后不刷新页面，`workflows` 创建被阻止，`assets`、`prompts`、`canvas`、`workbench` 都展示本地工作区断连提示。

### E. 模板、运行与 Artifact

- Web UI 创建模板后可从列表继续编辑、保存、复制、删除副本。
- 固定素材节点选择 image asset，未看到 text/video 素材混入当前选择结果。
- 本地 run 创建成功，reference 节点生成 artifact，artifact 文件落盘并可在 UI 预览。
- run event 包含 `run.created`、`run.material_lookup.fixed_assets_prepared`、`run.waiting_for_executor`。
- 同一前端 origin 切换到工作区 B 后，`assets`、`prompts`、`canvas`、`workbench`、`templates`、`runs` 页面未泄露工作区 A 的标记数据。

### F. MCP Client

- `tools/list` 返回 14 个只读工具：
  `opsc_artifact_list`、`opsc_asset_list`、`opsc_profile_list`、`opsc_project_list`、`opsc_prompt_list`、`opsc_run_events`、`opsc_run_list`、`opsc_run_status`、`opsc_template_list`、`opsc_workspace_doctor`、`opsc_workspace_export_plan`、`opsc_workspace_gc_plan`、`opsc_workspace_index_rebuild`、`opsc_workspace_info`。
- 未暴露 create/update/delete/write/import/attach/append 类 mutation 工具。
- 常规只读 tool call、active workspace index rebuild、inactive workspace index rebuild 阻断均符合预期。
- 未泄露 workspace 绝对路径、project root、bearer token、launch secret、fake secret value。
- Phase 8.1 复验通过：`opsc_workspace_info` 默认输出中的 `runtime` 只保留 `active=true`，未暴露 `runtime.baseUrl`、`runtime.host`、`runtime.port`、`pid`、`tokenFile`、`launchSecretFile` 或任何可重建本地 serve URL 的字段。

## 问题与建议

### Resolved：MCP `workspace_info` 泄露本地 serve URL

原现象：`/tmp/opsc-phase8/evidence-F-mcp.json` 中 `redactionLeaks.workspaceInfo` 包含 `serveUrl`。其他敏感项已正确规避，但 MCP 默认输出仍包含 `runtime.baseUrl`。

根因：

- `cmd/opsc/mcp.go` 中 MCP tool 直接包装 `opsc workspace info --json`，没有对 `opsc_workspace_info` 做 MCP 专用脱敏。
- `cmd/opsc/main.go` 的 workspace info JSON 来自 `workspace.Info(opts.ShowPaths)`。
- `internal/localworkspace/workspace.go` 的 `Info` 默认包含 `Runtime: w.readRuntimeInfo()`，其中 `readRuntimeInfo` 会返回 `BaseURL`。
- 当前测试覆盖了 active `workspace_index_rebuild` 不泄露 runtime base URL，但 `opsc_workspace_info` 只覆盖了 workspace path 不泄露，缺少 runtime URL 的断言。

修复：只在 MCP 包装层为 `opsc_workspace_info` 增加 stdout JSON 脱敏转换，默认把 `runtime` 缩减为 `{ "active": true|false }`；不改 `opsc serve`、CLI `workspace info`、Web UI 本地连接或 `opsc_workspace_index_rebuild` 读取 active serve runtime 的行为。

验证：

- 自动化：新增 `TestMCPWorkspaceInfoRedactsActiveRuntimeURL`，覆盖 active `opsc serve` 下 MCP `workspace_info` 不泄露 `baseUrl/host/port/pid/tokenFile/launchSecretFile`，且 structuredContent 中 runtime 只剩 `active=true`。
- 手工：使用真实 `opsc` 二进制和临时 workspace 复验 MCP `initialize`、`tools/list`、workspace info/doctor/export plan/GC dry-run、template/run/artifact/profile/project/asset/prompt list、active index rebuild 和 inactive index rebuild；结果见 `/tmp/opsc-phase8-1/evidence-F-mcp-phase8-1.json`。
- 目标重跑：再次使用临时 workspace 与真实 `opsc` 二进制重跑 F 项 MCP stdio 证据链，`tools/list` 仍为 14 个只读/诊断工具，未出现对象 create/update/delete/import/append 类 mutation tool；`opsc_workspace_info` 的 `runtime` 仍只包含 `active=true`，active index rebuild 成功，inactive index rebuild 按预期返回 tool error；结果见 `/tmp/opsc-phase8-1-rerun/evidence-F-mcp-rerun.json`。

### Non-blocking：提示词导入/导出 UI 验收标准需要确认

现象：提示词中心未发现显式导入/导出 UI。本次已验证提示词 CRUD、`content.md` 落盘和公有复制，但没有覆盖“提示词导入/导出”。

建议：确认该验收项是否仍然要求独立 UI。如果要求，应作为后续功能单独排期；不作为 Phase 8 关闭阻塞项。

### Non-blocking：模板新建下拉在自动化里路由不稳定

现象：Playwright/CDP 点击新建模板下拉时，模板对象已持久化，但自动化中的页面跳转不稳定。本次从模板列表继续打开该 UI 创建出的模板，并完成编辑、复制、删除和 run 验证。

建议：如果后续要做浏览器回归，可以给模板新建入口和创建结果增加稳定 `data-testid` 或更确定的 e2e 等待条件。

## Phase 9 前建议

1. Phase 8.1 已修复并复验 MCP `opsc_workspace_info` 的 serve URL 泄露；Phase 8 可以关闭。
2. 下一阶段如进入真实 local executor / agent 集成，继续沿用现有 MCP 工具面冻结、`opsc serve` single-writer、auth/redaction/path safety 约束。
3. 提示词显式导入/导出 UI、真实外部模型账号 live call、真实 Codex / Claude Code 客户端展示层 spot check 可作为非阻塞后续项分别处理。

## 文档与工作区说明

- Phase 8.1 已更新 `docs/pending-test.md`，明确 Phase 8 手工验收可关闭，剩余事项为非阻塞跟进项。
- 既有工作区改动 `web/next-env.d.ts` 与本次测试无关，未修改。

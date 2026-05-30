# TODO

本文档用来记录当前项目后续比较值得处理的事项。

- 继续扩展 Local Workflow Executor：hybrid ecommerce 已先接入单条确认模板的 VPS PDD API backend，并提供 `opsc ecommerce import-template` / `create-run` 的本地 headless CLI 路径；后续补真实可达 VPS API smoke、远端事件更细粒度增量同步、`image_edit`、`video_generation`、复杂 loop/guardrail、多轮质检修复、自动素材匹配和更完整的失败恢复；继续不复用现有 PDD/VPS run 目录作为事实源。
- 深化本地项目 adapter：Phase 10 已让 `projects/<proj_id>/project.json` 的 capability guard、path safety 和 adapter metadata 参与 `condition`/`script`/project output mapping；后续需要为文章、视频、电商项目补专用 adapter 和真实业务脚本模板。
- 为 `opsc` 补安装/打包文档和真实 agent 客户端配置示例，覆盖 Codex / Claude Code MCP client 的本机 smoke。
- 把 `tools/local_workspace_browser_smoke.py` 接入可选本机/CI 浏览器回归，重点覆盖 bootstrap session、浏览器不保存 secret、本地素材/提示词/画布/工作台/workflow 模板和 run/artifact 预览。
- 补充 `workspace doctor` 的 index 新鲜度提示和可操作修复建议；Full GC 仍保持未来设计，不在当前 v1 自动删除文件。

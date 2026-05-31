# TODO

本文档用来记录当前项目后续比较值得处理的事项。

- 继续扩展 Local Workflow Executor：hybrid ecommerce 黄金路径已经具备 `opsc executor --watch` worker、Web UI local run 启动、远端阶段同步、canonical artifact/ref 回写、worker runtime/doctor 诊断、安装/自启动文档和可复用 smoke helper。后续优先补 `image_edit`、`video_generation`、复杂 loop/guardrail、多轮质检修复、自动素材匹配、更完整失败恢复和长期真实 workspace 回归；继续不复用现有 PDD/VPS run 目录作为事实源。
- 深化本地项目 adapter：Phase 10 已让 `projects/<proj_id>/project.json` 的 capability guard、path safety 和 adapter metadata 参与 `condition`/`script`/project output mapping；后续需要为文章、视频、电商项目补专用 adapter 和真实业务脚本模板。
- 为真实 agent 客户端补配置示例和本机 smoke，覆盖 Codex / Claude Code MCP client 展示层。
- 将 `tools/local_workspace_browser_smoke.py`、`tools/hybrid_ecommerce_browser_smoke.py` 和 `tools/hybrid_ecommerce_vps_smoke.py` 接入可选 CI；当前已可作为本机回归入口，CI runner 的浏览器、workspace 和 fake/real backend 前置条件仍需单独配置。
- Full GC 仍保持未来设计，不在当前 v1 自动删除文件。

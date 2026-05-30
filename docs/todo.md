# TODO

本文档用来记录当前项目后续比较值得处理的事项。

- 设计并实现真实 local workflow executor，把当前 local run 草稿接入可执行队列、事件语义、失败恢复和 artifact 写入，不复用现有 PDD/VPS run 目录作为事实源。
- 设计本地项目 adapter，让 `projects/<proj_id>/project.json` 的 capability guard、path safety 和 adapter metadata 真正参与文章/视频/电商等本地项目工作流。
- 为 `opsc` 补安装/打包文档和真实 agent 客户端配置示例，覆盖 Codex / Claude Code MCP client 的本机 smoke。
- 增加 Web UI local workspace 的浏览器自动化回归，重点覆盖 bootstrap session、浏览器不保存 secret、本地素材/提示词/画布/工作台/workflow 模板和 run/artifact 预览。
- 补充 `workspace doctor` 的 index 新鲜度提示和可操作修复建议；Full GC 仍保持未来设计，不在当前 v1 自动删除文件。

# Mermaid 图索引

本目录保存当前仓库的项目记忆图。图文件使用 Mermaid `.mmd`，可在支持 Mermaid 的 Markdown 渲染器中查看。

## Diagram List

- `system-architecture.mmd`：回答“浏览器、Next.js、Go API、数据库、文件系统、模型渠道和 PDD workflow 如何连接”。
- `request-flow.mmd`：回答“用户通过后端远程渠道发起 AI 请求时，鉴权、计费、渠道选择和失败返还如何流动”。
- `data-flow.mmd`：回答“画布、本地素材/提示词/私有模板/工作台记录缓存、local workspace、公共提示词、服务器素材、PDD run 和视频文件分别存在哪里”。
- `deployment-flow.mmd`：回答“本地开发、普通 Docker、PDD console Docker、Render 和 GHCR 发布路径是什么”。
- `local-workspace-v1.mmd`：回答“local-first workspace、opsc CLI、opsc serve runtime、opsc executor、Web UI、MCP 和云端边界如何连接”。
- `user-flow-canvas-generation.mmd`：回答“画布生成节点在本地直连和后端远程渠道下如何生成并保存结果”。
- `user-flow-pdd-workflow.mmd`：回答“管理员如何用模板启动 PDD custom run，并在运行页查看结果或走 local agent 脚本节点”。

## Update Triggers

- 新增或删除主要运行时组件。
- API 代理边界、鉴权边界、数据库/文件存储边界变化。
- Local workspace contract、CLI/MCP、本地服务或数据归属边界变化。
- PDD workflow、local agent、模型渠道或部署方式变化。
- 浏览器本地持久化 key、画布数据结构或素材/提示词归属变化。

## Rendering Notes

- 标签尽量短，避免 Mermaid 在窄屏中挤压。
- 图只表达稳定架构，不记录一次性调试路径。
- 不能从代码确认的内容在正文文档中标记 `unknown` 或 `inferred`，图中不做未经证明的外部拓扑承诺。

# ADR-0001 Durable Project Memory

## Context

当前仓库已经从基础无限画布扩展出账号/算力点、模型渠道、视频工作台、PDD / 电商工作流控制台、自定义模板 DAG、local agent 和多种部署路径。仅靠 Git diff 很难让后续 AI/Codex 任务快速理解当前边界、运行方式和风险点。

## Decision

在仓库内新增 `docs/ai/` 作为项目本地 AI 记忆包：

- `project-card.md` 保存项目定位、技术栈、入口、外部服务和状态。
- `architecture.md` 保存稳定架构边界。
- `diagrams/` 保存 Mermaid 架构图和关键流程图。
- `runbook.md` 保存安装、运行、部署、验证和常见故障。
- `handoff.md` 保存当前上下文、缺口和后续建议。
- `gotchas.md` 保存容易踩坑的事实。
- `change-log.md` 保存语义级项目记忆变更，不复制 Git diff。

中央 LLM Wiki 只保存轻量 project entity，指回仓库内 `docs/ai/`，不复制完整项目记忆。

## Consequences

- 后续 AI 任务可以先读 `docs/ai/`，减少重复探索和错误假设。
- 架构、部署、数据边界和不确定项有固定位置更新。
- 文档需要随实质性架构/运行方式变化维护。
- 该记忆包不是代码事实源；精确 diff 仍以 Git 和当前代码为准。

## Alternatives Considered

- 只更新 README：README 应保持简洁，不能承载架构、运行手册、风险和 handoff。
- 只写中央 Wiki：会把项目细节从 repo 中剥离，不利于随代码演进。
- 只依赖 Git 历史：Git 适合精确 diff，不适合保存语义、边界、风险和不确定项。

## Status

Accepted.


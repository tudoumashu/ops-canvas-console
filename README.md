<p align="center">
  <img src="web/public/logo.svg" width="96" alt="Ops Canvas Console logo">
</p>

<h1 align="center">Ops Canvas Console</h1>

Ops Canvas Console 是一款面向本地创作、工作流编排和电商内容生成的开源工作台。它把画布编排、AI 图片生成、参考图编辑、对话助手、提示词中心、素材沉淀、local workspace 和 hybrid ecommerce worker 放在同一套控制台里。

> [!CAUTION]
> 项目目前处于开发阶段，不保证历史数据兼容。各种数据库结构和存储格式都可能直接调整，欢迎关注后续更新，当前更适合个人/本地部署，不建议直接公网多人共用。
>
> 如果你需要稳定维护自己的分支，建议自行 fork 后独立开发。二次开发与 PR 请保留原作者信息和前端页面标识。

## 核心功能

- 无限画布：多画布项目、节点拖拽缩放、连线、小地图、撤销重做、导入导出。
- AI 创作：支持 OpenAI 兼容接口的文生图、图生图、参考图编辑和文本问答。
- 画布助手：围绕选中节点和上游节点对话、生图，并把结果插回画布。
- 提示词中心：抓取多个 GitHub 开源项目，按案例整理数百个图片提示词，并支持 local workspace 的“我的提示词”。
- 本地工作区：通过 `opsc workspace`、`opsc serve` 和 `opsc mcp` 管理本机私有画布、素材、提示词、模板、run、artifact、profile 和项目引用。
- 电商工作流：支持查看 VPS 运行结果，并用模板画布定义可复用的商品生成 DAG。

完整功能说明见 [docs/features.md](docs/features.md)。

如果你在为担心没有合适的生图API来发愁，可以查看该免费生图项目：[chatgpt2api](https://github.com/basketikun/chatgpt2api)

## 技术栈

- 前端：Next.js、React、TypeScript、Tailwind CSS、Ant Design、Zustand、TanStack Query。
- 后端：Go、Gin、GORM。
- 部署：Docker。

## 快速开始

```bash
git clone https://github.com/tudoumashu/ops-canvas-console.git
cd ops-canvas-console
cp .env.example .env
# 修改默认账号密码等信息
docker-compose up -d
```

本地源码构建运行：

```bash
cp .env.example .env
docker compose -f docker-compose.local.yml up -d --build
```

运行后默认端口3000，可访问 `http://localhost:3000`。

如需要拉取提示词，可前往:`http://localhost:3000/admin/prompts`

### 本地工作区

个人自用模式建议先初始化 local workspace，并让浏览器只通过本机 `opsc serve` 访问私有数据：

```bash
go run ./cmd/opsc workspace init --workspace ~/OpsCanvas
go run ./cmd/opsc serve --workspace ~/OpsCanvas --origin http://localhost:3000
# 另开一个终端运行本地 worker
go run ./cmd/opsc executor --workspace ~/OpsCanvas --watch --poll-interval 5s
```

`opsc serve` 默认只监听 `127.0.0.1`，runtime metadata、`bearer.token`、一次性 `launch.secret` 和 session 文件写在 workspace 之外的 XDG state 目录。浏览器只保存 loopback `baseUrl`，不保存 token、launch secret 或 workspace 绝对路径，也不直接写 `~/OpsCanvas`。

`opsc executor` 是当前唯一正式的本地工作流执行入口。`--watch` 模式会持续领取 local workspace 中 `run.waiting_for_executor` 的 run，并把节点状态、事件和产物写回 workspace canonical files；hybrid ecommerce 模板会通过 workspace profile/channel `secretRef` 调用已确认的 VPS PDD API backend，并把远端状态与关键 artifact 同步回本地 canonical objects。

MCP 集成使用 `opsc mcp --workspace <path>`。MCP 只是 CLI/core/`opsc serve` 的薄封装，不是新的事实源；当前主要暴露只读、诊断和通过 active `opsc serve` 重建派生索引的维护能力。

## 效果展示

<table width="100%">
  <tr>
    <td width="50%"><img src="https://i.ibb.co/TDFvGWDT/image.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/zVwJq3YS/image.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/PvY3qhhK/image.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/7D04LwN/image.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/bj30FtS5/5.png" alt="5" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/hxRvjw51/image.png" alt="image" border="0"></td>
  </tr>
</table>

## 文档

- [功能介绍](docs/features.md)
- [部署说明](docs/deployment.md)
- [opsc 本地安装与自启动](docs/opsc-installation.md)
- [Local Workspace 回归入口](docs/local-workspace-regression.md)
- [电商工作流](docs/pdd-workflow.md)
- [画布节点操作手册](docs/canvas-node-manual.md)
- [画布快捷键](docs/canvas-shortcuts.md)
- [待办事项](docs/todo.md)
- [后端数据库说明](docs/backend-database.md)
- [系统配置数据结构](docs/system-settings.md)
- [接口响应约定](docs/api-response.md)
- [Local Workspace 数据分离计划](docs/local-workspace-data-separation-plan.md)
- [Local Workspace v1 Contract](docs/local-workspace-v1-contract.md)

## 社区支持

学 AI，上 L 站：[LinuxDO](https://linux.do/)

点击链接加入群聊【AI开源交流】：https://qm.qq.com/q/DFnKzZ807u

## 开源协议

本项目使用 GNU Affero General Public License v3.0，见 [LICENSE](LICENSE)。


## Star History

<a href="https://www.star-history.com/?repos=tudoumashu%2Fops-canvas-console&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=tudoumashu/ops-canvas-console&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=tudoumashu/ops-canvas-console&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=tudoumashu/ops-canvas-console&type=date&legend=top-left" />
 </picture>
</a>

# Gotchas

## 当前工作区不是干净基线

项目记忆初始化时 `git status --short` 显示大量已修改和未跟踪业务文件。本次项目记忆只记录当前工作区事实，不代表这些改动已经提交、发布或测试通过。

## 浏览器本地数据不是云同步

画布项目、“我的素材”、“我的提示词”和用户本地 AI 配置主要保存在浏览器 localforage/IndexedDB。登录账号后不表示这些本地业务数据自动同步到服务器。

## AI Key 可能在两处

- 本地直连模式：用户 API Key 保存在浏览器本地配置中，并由前端直接请求 OpenAI-compatible 接口。
- 远程渠道模式：管理员渠道密钥保存在后端 `settings.private.channels`，前端不应读取真实密钥。

文档和 UI 说明需要区分这两种模式。

## GORM AutoMigrate 不是显式迁移系统

项目启动时自动迁移表结构，没有独立 migration / rollback CLI。表结构或字段变更需要同步 `docs/backend-database.md`，但不要假设可以精细回滚生产数据。

## Local Agent 任务不持久

`service/local_agent.go` 使用内存 map 保存任务。服务重启会丢失 pending/running local agent job。不要把它当 durable queue。

## PDD Console 权限很高

`docker-compose.pdd-console.yml` 使用 host network、host pid、privileged，并挂载 `/opt/pdd-workflow`。修改 PDD allowlist 动作、nsenter 或脚本执行边界前必须先确认安全影响。

## PDD sync_local 依赖反向 SSH

自定义工作流里的 `sync_local` / `scripts/trigger_local_receive_and_upload.sh` 需要 VPS 能连到 `127.0.0.1:22222`，通常由用户本地建立 `ssh -R 22222:127.0.0.1:22 ...`。未建立或被重置时应让脚本节点失败并提示“本地同步通道不可用”，不要把它伪装成仍在运行。

## PDD 创作画布轮询不能覆盖本地交互

结果页“创作画布”会同时有浏览器本地拖拽/缩放/生成状态、debounced 保存和后端 live workflow 状态。后台轮询只能非破坏性合并原始 run 节点状态，不能因 `updatedAt` 变化整张 hydrate，也不能重放服务器 viewport；否则会出现缩放抽搐、模型配置回退、替换图旧内容复活、多角度/裁剪节点消失和布局跳变。

## Render 免费版文件不持久

`docs/deployment.md` 已说明 Render 免费 Web Service 空闲休眠，且本地文件不是持久化存储。默认 SQLite 不适合长期正式数据。

## Flow2API 视频要落盘到持久卷

Flow2API 视频结果会写入 `VIDEO_STORAGE_ROOT`。Docker 部署时应让该目录在 `data/` 持久卷内，否则重启后视频工作台历史结果可能无法继续预览/下载。

## PDD Run 文件读取需要路径约束

PDD run id 和文件路径都应保持受限解析，避免从 API 读取 run 根目录外文件。新增 run/file 接口时沿用现有 safe path 和 run id 校验。

## 提示词定时同步会访问外部 GitHub

默认 prompt sync cron 是 `*/5 * * * *`，会同步内置 GitHub 远程提示词源。离线、本地演示或网络受限环境中，启动后可能出现同步失败日志。

## Docker 同时跑 Next.js 和 Go

运行镜像中 shell `CMD` 先启动 Go API，再启动 Next.js。`API_BASE_URL`、`API_PORT`、`WEB_PORT` 和 `APP_HOSTNAME` 的组合决定容器内代理和对外监听，排查 `/api/*` 时需要同时看两个进程。

## README 品牌与 repo remote 不完全一致

README 仍写 `infinite-canvas` 和上游 `basketikun/infinite-canvas` 部署链接；当前 repo path 和 remote 是 `ops-canvas-console` / `tudoumashu/ops-canvas-console.git`。对外发布前需要确认品牌和部署按钮是否应保持上游或改为当前 fork。

# Handoff

## Current Objective

正在按用户要求继续修复 PDD/电商工作流结果页“创作画布”的缩放稳定性、白色悬浮工具栏功能、裁剪/多角度生成节点保留，以及同步到 VPS 后的浏览器回归。

## Completed Work

- 结果页创作画布移除左上角说明气泡，连接线恢复 `ConnectionPath` 默认贝塞尔曲线。
- 结果页媒体节点通过 `CanvasNode mediaFrame` 启用带背景和边框的节点包裹感，不改变原生画布默认媒体节点样式。
- 创作画布图片/视频节点会根据保存的 `naturalWidth/naturalHeight` 或前端探测到的媒体尺寸适配节点宽高；后端初始化图片 artifact 时也会尝试写入自然尺寸。
- 本轮继续补齐结果页创作画布：小地图使用紧凑节点标识，新增节点按外框寻找空位；悬浮工具栏延迟隐藏，鼠标从节点移动到工具栏时不应闪退；工具栏已接入文本编辑、文本生图、图片/视频替换、下载、存素材、锁比例、裁剪、多角度、查看大图、失败重试、编辑面板和“应用副本并重跑下游”。
- 后端新增创作画布输出应用接口：结果页生成/裁剪/多角度/替换后的节点可覆盖对应工作流节点输出，并沿原模板 DAG 重跑下游。
- 已排查 `custom_20260529_073257`：VPS run 状态文件为 failed，失败点是 `sync_local`，日志显示连接 `127.0.0.1:22222` 被重置；未发现该 run 仍有执行进程。
- `sync_local` 脚本节点现在会先预检 VPS 到 `127.0.0.1:22222` 的反向 SSH 通道，不可用时返回明确错误；自定义 run 若无 running 商品但存在失败商品，会收敛为 `error`。
- 模板编辑器为模型调用节点增加“失败重试”配置：启用、重试次数、间隔秒数；默认启用、次数 0 无限、间隔 0 系统退避。
- 后端执行器对模型调用节点读取 `node.retry`；guardrail 生成/修复/复检瞬时重试改为同一语义，不再默认固定 100 次。
- 本轮新增修复：创作画布不再因后台轮询的 `updatedAt` 变化整张重灌节点或重放服务器 viewport；后续轮询只非破坏合并 live 状态，避免缩放抽搐、模型配置回退和生成节点消失。
- 本轮新增修复：保存逻辑改为串行保存最新 refs，避免保存过程中新增的裁剪/多角度/生成结果没有被再次保存。
- 本轮新增修复：图片/视频替换会清理旧生成 metadata、标记 `source=user_upload` 并立即更新节点内容和尺寸；锁比例/自由比例对齐原生画布；裁剪和多角度弹窗增加提交 loading 并保留结果节点。
- 本轮新增修复：后端 creative canvas merge 不再覆盖用户上传/creative 派生节点内容，也不再覆盖已保存的 `prompt/model/size/quality/count` 等配置；新增 `service/pdd_test.go` 覆盖关键 merge 语义。
- 本轮新增修复：旧保存创作画布如果只有 run 派生节点且媒体真实尺寸导致节点矩形重叠，后端 live merge 会自动重排一次；包含用户上传或 creative 派生节点时不自动重排，避免打乱用户本地结果。
- `docs/pending-test.md` 已记录需要人工回归的前端交互和重试行为。
- 本地 `npx tsc --noEmit` 已通过；本地 Docker `golang:1.25-alpine` 执行 `gofmt` 和 `go test ./...` 已通过。
- 已运行 `git diff --check`。
- 本轮相关文件已通过 tar-over-ssh 同步到 VPS `/opt/ops-canvas-console`；VPS 执行 gofmt、`docker compose -f docker-compose.pdd-console.yml up -d --build app`，API `/api/health` 返回 `ok`，Next `/workflows/pdd` 返回 `HTTP/1.1 200 OK`。
- 本轮最终浏览器回归已通过：`custom_20260529_073257` 创作画布 8 个节点无重叠、5 张图片加载完成、工具栏按视口夹取且从节点移动到工具栏后仍可点击、缩放后等待 6.5 秒最大位移为 0；API 确认该 run 已收敛为 `error`。

## In Progress

- 无。

## Blockers

- 当前本机没有 `go` / `gofmt`，Go 格式化需通过 Docker 或 VPS 环境执行。
- fact：VPS 上本项目部署目录为 `/opt/ops-canvas-console`，compose 文件为 `docker-compose.pdd-console.yml`，容器名为 `pdd-run-console`。
- fact：VPS `0.0.0.0:443` 当前监听进程是 `sshd`；应用容器内/host network 暴露的服务监听在 `127.0.0.1:18080` API 和 `127.0.0.1:13000` Next。
- unknown：当前工作区已有大量未提交业务改动，不能回滚或覆盖。

## Files Or Areas To Avoid

- 不要回滚或覆盖初始化前已经存在的未提交业务改动。
- 不要读取或记录 `.env`、真实密钥、模型 API Key、Linux.do Client Secret、PDD 账号/会话。
- 不要在未确认前改 `docker-compose.pdd-console.yml` 的 privileged / host network / nsenter 行为。
- 不要把浏览器本地画布项目和“我的素材”误写成服务器云同步。

## Next Recommended Steps

- 对真实产物写入类动作继续人工回归：替换图片旧内容不残留、自由比例/锁比例、裁剪确认、多角度生成节点保留、artifact 预览、应用副本后下游重跑。
- 如后续要求公网 Web 直接通过 `https://96.9.225.98` 访问，需要先单独确认反向代理/域名/端口方案；本轮未修改部署配置或 `.env`。

## Validation Status

- passed：`cd web && npx tsc --noEmit`。
- passed：`docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine gofmt -w service/pdd.go service/pdd_test.go`。
- passed：`docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine go test ./...`。
- passed：`git diff --check`。
- passed on VPS：tar-over-ssh 同步本轮相关文件；`docker compose -f docker-compose.pdd-console.yml up -d --build app`；`curl -fsS http://127.0.0.1:18080/api/health`；`curl -fsSI http://127.0.0.1:13000/workflows/pdd`。
- passed browser：Playwright 登录后打开 `custom_20260529_073257` 创作画布，确认节点不重叠、图片加载、悬浮工具栏稳定、节点信息按钮可点击、缩放后等待轮询不抖动。
- not run：真实多角度生成和应用副本重跑会消耗模型额度或改写 run 产物，保留在 `docs/pending-test.md` 做人工回归。

## Recent Important Commits

- `f9e4c92 refactor(video): 重构视频配置参数标准化逻辑`
- `1be6b4f feat(video): 新增视频创作台页面`
- `7968820 feat(account): 新增用户账号与算力点体系`
- `8c506f9 feat(video): 对齐 grok-imagine-video 接口并优化视频生成功能`
- `b21f8c3 feat(admin): 优化管理后台渠道模型选择功能`
- `d8cb1d6 fix(image): 修复图像生成中quality参数传递问题`
- `b8e50c1 feat(auth): 添加用户注册开关功能`
- `030541b feat(canvas): 添加画布图片信息显示功能`
- `2934b1d refactor(ai): 重构AI接口算力点扣费逻辑并添加失败返还机制`
- `01f2a4d feat(ai): 实现AI模型调用的积分计费功能`

# 功能介绍

本文档记录当前项目已经实现的主要功能。

## 画布项目

- 支持创建多个画布项目。
- 支持项目重命名、删除、批量选择和批量删除。
- 支持单个画布项目导出为 JSON，也支持从 JSON 导入画布。
- 连接 local workspace 后，画布项目写入 `canvas-projects/<canvas_id>/canvas-project.json`，图片/视频节点和助手媒体写入同目录 `files/` 并通过 `workspaceFileKey` 引用。
- 浏览器存储只作为展示缓存和上传/导入前的临时桥接，不再作为画布项目长期事实源；旧浏览器测试数据不迁移。

## 电商工作流

- 支持部署在 VPS 上直接读取 `/opt/pdd-workflow/runs`。
- 支持工作流列表、阶段 DAG 总览、商品级详情画布和右侧文件详情面板。
- 支持预览 `generated/` 源图、`待上架/规格图`、`待上架/主图`。
- 支持查看每个商品的 `pipeline_status.json`、质检、修复、标题、最终主图和最终复检日志。
- 支持管理员执行 allowlist 运维动作：续跑、停止、健康检查、Docker 状态、重启指定服务和 WARP 重连。
- 控制台不负责拼多多本地上传，VPS workflow 启动动作固定追加 `--skip-upload`。
- 支持启动时导入工作流素材，素材以服务器侧图片加入“素材库”，并通过结构化用途、分类路径、来源和元数据供工作流匹配。
- 支持电商工作流模板画布：用素材、文字、图片、视频节点定义可复用 DAG，运行时把一批输入主题按模板批量执行，并将结果写回 run 目录。

## 无限画布

- 支持拖动画布、滚轮缩放、缩放滑杆和重置视图。
- 支持小地图定位，可开关小地图。
- 支持点阵、网格线、空白三种背景。
- 支持浅色和深色主题。
- 支持框选、多选、全选、取消选择、删除选中。
- 支持复制粘贴节点和节点之间的连线。
- 支持撤销和重做节点、连线、视口、背景和助手会话变化。
- 支持节点连线，并高亮当前节点相关的上下游节点和连线。
- 支持快捷键帮助，覆盖缩放、框选、全选、复制粘贴、撤销重做、删除、退出选择和拖入图片。

## 节点

目前画布中有三类节点：

- 图片节点：展示上传图片、生成图片或素材库图片。
- 文本节点：保存提示词、说明文案、AI 文字回答等文本内容。
- 生成配置节点：汇总上游文本和图片，统一配置模型、比例、数量后批量生成图片或文本。

节点支持：

- 拖拽移动。
- 四角缩放。
- 图片节点等比缩放或自由比例切换。
- 查看节点基础信息和 JSON。
- 删除、复制、粘贴。
- 通过左右连接点建立上下游关系。

## 图片工作流

- 支持上传图片到新节点。
- 支持拖拽图片文件到画布。
- 支持替换已有图片节点内容。
- 支持下载图片节点。
- 支持把图片节点保存到“我的素材”。
- 支持图片裁剪，并把裁剪结果生成为新的图片节点。
- 支持本地多角度变换，并把结果生成为新的图片节点。
- 支持生成失败后重试。
- 批量生成多张图片时会先展示为图片组节点，支持叠卡预览、展开查看全部结果并设置主图。

## AI 生成

项目支持两种模型调用方式：默认通过后端 `/api/v1/*` 代理到管理员配置的模型渠道；连接 local workspace 后，用户可在本地 profile 中配置 OpenAI 兼容渠道，并通过 `opsc serve` 的 `/api/local/ai/v1/*` 本机代理调用。

- `/v1/images/generations`：文生图。
- `/v1/images/edits`：图生图/参考图编辑。
- `/v1/chat/completions`：文本问答和带图问答。
- `/v1/videos`：视频生成。
- `/v1/models`：读取模型列表。

管理员模型渠道支持 `openai` 和 `flow2api` 协议。`flow2api` 渠道通过 `/chat/completions` 返回媒体链接，后端会适配为现有图片和视频接口，工作台和模板工作流不需要关心上游差异。

local workspace 模式下，模型渠道写入 `profiles/<profile_id>/profile.json`，只保存 Base URL、模型列表、默认模型和 `secretRef`；真实 API Key 通过环境变量或本机 secret 文件解析，不写入浏览器长期存储，也不由浏览器直接转发给供应商。

可配置项：

- Base URL。
- API Key。
- 默认模型。
- 默认图片、文本、视频模型。
- 图片质量。
- 图片比例。
- 生成数量。

工作台提供图片、文本、视频三种创作模式，使用统一的生成记录、模型选择和结果区布局。普通图片/文本节点可以直接输入提示词生成结果。生成配置节点可以读取上游节点内容，并按节点自己的配置批量生成多个图片或文本结果。生成配置节点支持预览当前提示词和参考图输入，并调整输入顺序。

## 画布助手

画布右侧助手面板支持：

- 文本问答。
- 生图。
- 读取当前选中节点作为引用。
- 自动把选中节点的上游节点也纳入引用。
- 粘贴图片到助手输入框并插入画布。
- 历史会话。
- 删除单条或多条会话。
- 重试回答。
- 把助手生成的文本插入画布。
- 把助手生成的图片插入画布。
- 折叠和展开助手面板。

## Local Workspace v1

Local Workspace v1 面向个人/本机自用场景，是私有数据的本地事实源。

- `opsc workspace init/info/doctor` 支持初始化、机器可读信息和结构诊断。
- `opsc serve` 默认监听 `127.0.0.1`，使用 workspace 外 XDG state 保存 runtime metadata、`bearer.token`、一次性 `launch.secret`、HttpOnly session、port/pid 和 lock 文件。
- `opsc executor --watch` 是正式本地 worker 入口；它使用 workspace 外 executor runtime metadata 和单 worker lock 提供最小健康检查，重启后按 canonical run/node/artifact 状态恢复。
- Web UI 通过 `opsc serve` 访问本地 profiles、projects、assets、prompts、canvas projects、workbench logs、templates、runs 和 artifacts；浏览器不直接写 `~/OpsCanvas`。
- 写操作走 workspace core/service、atomic write、lock、revision 检查、path escape 防护和默认脱敏输出。
- canonical artifact metadata 写在 `artifacts/<art_id>/artifact.json`，run 目录只保存 artifact ref，避免同一产物 metadata 双写漂移。
- `index.sqlite` 是派生索引，可通过扫描 canonical JSON/JSONL/files 重建。
- `workspace doctor` 会提示 index 可能过期、executor worker stale、hybrid run 等待/卡住等可操作修复建议，但不会执行 Full GC。
- `opsc mcp` 是 stdio 薄封装，复用 CLI/core/active `opsc serve`；当前不提供独立 repository、独立 writer 或新的对象 schema。
- 当前不会迁移旧浏览器测试数据，也不会迁移现有 PDD/VPS run。

## 提示词中心

前台提示词中心分为“提示词库”和“我的提示词”两个分区。提示词库保存服务器公共提示词；连接 local workspace 后，“我的提示词”写入 `prompts/<prompt_id>/prompt.json` 和 `content.md`。

提示词中心支持：

- 按标题搜索。
- 按媒体分类、用途、来源和自由标签筛选。
- 多个自由标签同时选择时只展示全部命中的结果。
- 查看提示词详情。
- 查看封面和结果图。
- 复制提示词。
- 把公共提示词加入“我的提示词”。

后台提示词管理支持：

- 查询提示词。
- 新增、编辑、删除提示词。
- 按分组和自由标签筛选。
- 编辑领域、阶段、模型、模式、输入输出类型和状态等结构化字段。
- 查看远程提示词源。
- 同步内置远程提示词源。

当前内置远程源包括多个 GPT Image / GPT-4o / Nano Banana Pro 相关提示词仓库。电商工作流生产提示词不再自动导入前台提示词库。

## 素材中心

“素材中心”分为“我的素材”和“素材库”两个分区。

连接 local workspace 后，“我的素材”保存在 `assets/<asset_id>/asset.json` 和 `files/`，支持：

- 新增文本素材和图片素材。
- 编辑素材标题、封面、标签、用途、来源、备注和内容。
- 删除素材。
- 按关键词搜索。
- 按媒体分类、用途、来源和自由标签筛选。
- 多个自由标签同时选择时只展示全部命中的结果。
- 分页浏览。
- 复制文本素材。
- 下载图片素材。
- 从画布节点和素材库复制素材。
- 在画布中插入素材。

“素材库”保存在服务器，支持：

- 按标题搜索。
- 按媒体分类、用途、来源和自由标签筛选。
- 查看素材详情。
- 复制文本或图片链接。
- 复制到“我的素材”。
- 普通用户只读展示，可下载、插入画布、复制到“我的素材”。
- 管理员可在前台素材中心新增、编辑、删除服务器素材。
- 在画布中插入素材。

后台素材库管理支持：

- 查询素材。
- 新增、编辑、删除素材。
- 按类型和自由标签筛选。
- 编辑分类路径、用途和来源等结构化字段。

## 账号和后台

- 注册功能暂时关闭。
- 仅允许管理员账号登录。
- 支持 JWT 会话。
- `/api/auth/me` 可读取当前用户，未登录时返回访客用户。
- 首次启动时可根据环境变量创建默认管理员。
- 管理员后台目前包含提示词管理和素材库管理。
- 后端已有用户管理接口，但前端暂未实现用户管理页面。

## 后端能力

- Gin 提供 API 服务。
- Docker 运行时由 Next.js 提供页面入口，`/api/*` 请求代理到内部 Go 服务。
- GORM 管理数据库连接和自动迁移。
- 支持 SQLite、MySQL、PostgreSQL。
- 数据库保存用户、提示词分组、提示词和服务器素材。
- 业务接口统一返回 `{ code, data, msg }`。

## 当前限制

- local workspace 当前仍是本机自用能力，不提供云同步；“素材库”、公共提示词和管理员配置仍保存在服务器/DB。
- 浏览器旧 localforage/IndexedDB 测试数据不迁移；连接 local workspace 后浏览器只保留 cache、temporary state 和展示层补水。
- local run 已接入 `opsc executor` MVP，可执行固定本地素材、`text_generation`、`image_generation`、`condition` 和受 project capability/path guard 约束的本地 `script` 最小节点集；hybrid ecommerce 已先接通单条已确认 PDD 模板的 VPS API backend，可把远端执行结果同步为本地 canonical artifact/ref。`image_edit`、`video_generation`、专用文章/视频/电商 project adapter、完整失败策略、通用远端模板平台和 PDD/VPS 历史 run 迁移仍未完成。
- 现有 PDD/VPS run 不迁移，VPS run 查看和本地 workspace run 是两条边界清晰的路径。
- MCP 当前是薄封装，主要暴露只读/诊断/dry-run 和通过 active `opsc serve` 重建索引；不暴露批量对象写入和执行器能力。
- 服务器素材库目前主要保存 URL 或文本，暂未提供文件上传接口。
- 画布更适合桌面端使用，移动端触控体验还未系统完善。

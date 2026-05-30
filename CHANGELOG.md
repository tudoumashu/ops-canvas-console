# CHANGELOG

## Unreleased

+ [新增] Local Workspace hybrid ecommerce：新增 `opsc ecommerce import-template`、`opsc ecommerce create-run`、executor 的 VPS PDD API backend 和 `tools/hybrid_ecommerce_vps_smoke.py` smoke helper，可把已确认远端电商模板重建为本地 canonical template，创建 pending local run，并把远端 run 状态和关键 artifact 同步回 local workspace。
+ [新增] Local Workflow Executor Phase 10：接入 run `projectId`、project capability/path guard、`condition`/`script`、节点级 retry、项目输出映射和 project-aware artifact 写入校验，并补充 Web UI local workspace browser smoke 脚本。
+ [新增] Local Workflow Executor MVP：新增 `opsc executor`，可领取 local workspace 中 `waiting_for_executor` run，执行固定本地素材、文本生成和图片生成节点，并写回 canonical node state、events、artifact 与 run artifact ref。
+ [修复] MCP `opsc_workspace_info` 默认输出不再暴露本地 `opsc serve` URL、host 或 port，Phase 8 手工验收已收口为可关闭状态。
+ [优化] Local Workspace v1 Phase 8 稳定化：补充 `opsc serve` 鉴权/redaction、CLI 输出脱敏、AI `secretRef` proxy、MCP stdio wrapper 工具面冻结和本地模板草稿 run/artifact ref 的回归验证。
+ [文档] 同步 README、功能说明、待测试和 TODO，明确 local workspace 是本机私有事实源，浏览器只保留缓存/临时状态，旧浏览器测试数据和现有 PDD/VPS run 不迁移。
+ [新增] PDD 自定义工作流运行页支持人工编辑节点图片副本，并可应用后重跑该商品后续节点。
+ [修复] Flow2API 视频结果改为落盘保存，避免容器重启后临时视频缓存丢失。
+ [修复] PDD 模板工作流运行时图片节点参数规范化对齐原生画布，`quality/size/count` 不再直传非法值。
+ [优化] 合并上游 v0.1.0：我的画布、我的素材导出能力改为 ZIP 包，并合入画布撤销、配置节点、图片/视频设置面板等修复。
+ [新增] 新增视频创作台页面。
+ [新增] PDD 工作流模板画布，支持用素材、文字、图片、视频节点定义可复用 DAG，并按批量输入执行自定义 run。
+ [优化] PDD 工作流模板运行页按模板节点动态展示，并支持多行 JSON 对象输入；Run 运行页改为单一主画布、右侧抽屉和底部居中工具栏。
+ [新增] PDD 商品主图模板支持固定 mockup 底版素材，并在源图和最终主图之间增加 Image 2 mockup 生成节点。
+ [新增] PDD 工作流启动任务支持 `console_workflow_spec.json`，可在页面输入/导入主题，并配置源图/最终主图 Image 2 prompt 与生成数量。
+ [新增] 新增 PDD 工作流，支持 VPS run 可视化、商品详情和受控运维动作。
+ [新增] PDD 工作流素材可自动导入到服务器侧“我的素材”，并保留 `PDD素材` 标签供工作流匹配。
+ [新增] 新增视频生成节点。

## v0.1.0 - 2026-05-26

+ [优化] 优化我的画布、我的素材导出功能。
+ [修复] 修复画布撤销、配置节点等问题。

## v0.0.9 - 2026-05-26

+ [新增] 新增视频创作台页面。
+ [修复] 修复图片节点 size 参数传递问题。

## v0.0.8 - 2026-05-24

+ [新增] 新增用户账号与算力点体系，支持账号密码注册登录、Linux.do OAuth。
+ [新增] 管理后台公开配置支持设置模型算力点、支持计费查询。
+ [新增] 画布右上角展示用户算力点余额，生成按钮会展示本次预计消耗算力点。

## v0.0.7 - 2026-05-23

+ [新增] 管理后台提示词管理支持多选批量删除。
+ [新增] 新增定义拉取GitHub提示词源功能。
+ [新增] 新增awesome-gpt-image2-prompts提示词来源。
+ [优化] 优化模型下拉选择样式、优化生图编辑设置

## v0.0.6 - 2026-05-22

+ [新增] 管理后台支持配置模型渠道，前端当前无需鉴权即可直接使用后端渠道能力。
+ [优化] 统一整理后端错误提示、AI 代理、图片节点生成与重试、参考图缺失处理等细节。
+ [优化] 后端模型代理路径调整为 OpenAI 风格。

## v0.0.5 - 2026-05-20

+ [新增] 右上角版本号支持点击查看版本更新弹窗，展示当前版本、最新版本和按时间线整理的更新日志。
+ [新增] 设置弹窗支持配置系统提示词，AI 生图、编辑图和文本请求会自动携带。

## v0.0.4 - 2026-05-20

+ [调整] Docker 运行入口改为 Next.js 对外提供页面，`/api/*` 由 Next.js 代理到内部 Go 服务。
+ [修复] 文本复制在局域网 IP 访问时可能失败的问题。

## v0.0.3 - 2026-05-19

+ [修复] 更新 nanoid 依赖并修改 ID 生成方式，防止其他ip无法使用crypto模块导致的ID生成失败问题。

## v0.0.2 - 2026-05-19

+ [新增] 增加生图工作台功能，支持文生图、图生图、查看历史记录，并增加移动端适配。
+ [修复] 画布生成尺寸控件支持选择更多常用比例，并可直接输入自定义比例。
+ [修复] 生成配置节点恢复拖拽操作，避免面板控件拦截整块节点拖动。
+ [文档] 增加 Render 部署说明。

## v0.0.1 - 2026-05-19

+ [新增] 首次开源版本，包含无限画布能力：多画布项目、节点拖拽缩放、连线、小地图、撤销重做、导入导出。
+ [新增] AI 创作能力：支持 OpenAI 兼容接口的文生图、图生图、参考图编辑和文本问答。
+ [新增] 画布助手能力：支持围绕选中节点和上游节点对话、生图，并把结果插回画布。
+ [新增] 提示词库能力：抓取多个 GitHub 开源项目，按案例整理数百个图片提示词。

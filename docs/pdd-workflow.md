# 电商工作流

电商工作流用于在 VPS 上可视化查看 `pdd-auto-workflow` 的运行结果，并执行少量受控运维动作。

## 部署方式

控制台应部署在美国 VPS 上，直接读取 `/opt/pdd-workflow/runs`：

```bash
cp .env.example .env
docker compose -f docker-compose.pdd-console.yml up -d --build
```

`docker-compose.pdd-console.yml` 使用 `network_mode: host`、`pid: host` 和 `privileged: true`，目的是让容器内后端可以通过 `nsenter` 执行宿主机上的 allowlist 命令。

默认前端监听 VPS 的 `127.0.0.1:13000`，建议本地通过 SSH 隧道访问：

```bash
ssh -p 443 -L 13000:127.0.0.1:13000 root@96.9.225.98
```

然后打开 `http://127.0.0.1:13000/workflows/ecommerce`。

## 必要配置

核心环境变量：

```env
PDD_WORKFLOW_ROOT=/opt/pdd-workflow
PDD_RUNS_ROOT=/opt/pdd-workflow/runs
PDD_MATERIALS_ROOT=/opt/pdd-workflow/materials
PDD_PROMPTS_ROOT=/opt/pdd-workflow/prompts
CONSOLE_ASSETS_ROOT=/app/data/assets
PDD_PYTHON=/opt/pdd-venv/bin/python
PDD_WORKFLOW_CONFIG=config/provider/workflow.remote-chatgpt2api.example.json
PDD_WORKFLOW_ENV_FILE=/opt/pdd-workflow/.pdd-console.env
PDD_ACTION_NSENTER=true
PDD_CONSOLE_READ_ONLY=false
```

如果需要从控制台启动或续跑 workflow，应在 VPS 上准备 `PDD_WORKFLOW_ENV_FILE`，写入 `CHATGPT2API_API_KEY` 等私有环境变量。该文件不应提交到 Git。

## 本地库导入

服务启动时会自动扫描 `PDD_MATERIALS_ROOT`；路径不存在时会尝试相邻仓库的 `../pdd/materials`，仍找不到时只记录日志并跳过，不影响控制台启动。

- `PDD_MATERIALS_ROOT` 下的图片会作为服务器素材导入到“素材库”。导入后使用通用素材分类体系：`categoryPath` 标记为 `角色参考图/标准参考图`、`角色参考图/官方参考图` 或 `通用图片`，`purpose` 标记为 `standard_reference`、`official_reference` 或 `generic`，IP 和角色信息写入 `metadata`，不再作为筛选标签直接混入标签云。
- `/api/assets/pdd-materials/file?path=<relative_path>` 只允许读取 `PDD_MATERIALS_ROOT` 下的图片文件，供素材中心的“素材库”分区预览。
- `CONSOLE_ASSETS_ROOT` 下的固定控制台素材会作为服务器素材导入，当前包含 `pdd-mockup-sku-artwork-base` 这类 Image 2 规格图底版；这类素材使用 `categoryPath=规格图模板`、`purpose=spec_template`、`source=cloud_asset`。`/api/assets/local/file?path=<relative_path>` 只允许读取该目录下的图片。
- 电商工作流生产提示词不再自动导入前台提示词库。
- 导入使用稳定 ID，可重复执行，不会产生重复素材。

## 页面层级

- `我的工作流`：工作流文件夹入口，当前内置 `电商工作流`，后续文章/视频工作流可作为独立文件夹接入。
- `工作流列表`：列出 `/opt/pdd-workflow/runs` 下所有 PDD run。
- `Run Overview`：固定阶段 DAG，只显示阶段级状态和聚合计数。
- `商品流程`：选择单个商品后展示源图、质检、修复、标题、规格图、主图、最终复检和待上架目录，只负责流程查看。
- `创作画布`：选择单个商品后展示可创作产物节点，支持按原生画布方式拖动、连线和保存排布。
- `详情面板`：展示原始 JSON、prompt/log 文件、错误摘要、图片预览和路径。

## 控制台启动任务

`工作流列表` 页的 `启动工作流` 会先选择工作流模板，再填写输入商品和并发/重试参数。控制台启动仍在 VPS 上运行，且不会在 VPS 上执行拼多多上传。

当前输入支持：

- 每行一个 JSON 对象，例如 `{"theme":"《原神》","character":"七七","presentation":"feminine"}`。
- JSON 数组，或包含 `themes` / `items` 数组的 JSON 对象。
- 纯文本行会按 `{ "theme": "<文本>" }` 处理。

模板的节点、prompt、模型、上游引用和输出路径都在模板画布中预先定义；启动时只填写本次商品输入和运行参数。

## 工作流模板画布

`/workflows/ecommerce/templates` 提供可复用的电商工作流模板。它和 `启动工作流` 弹窗不同：

- 模板画布只定义流程，不会在编辑阶段调用模型。
- 模板编辑器支持“自由画布”和“流程图视图”。自由画布保留原生拖拽、缩放、连线和坐标保存；流程图视图用于复杂 DAG，可把质检/判定/修复循环折叠成组合节点，并用正交线展示主线、参考输入、通过分支和修复回环。
- 输入可以是逐行主题，也可以是 JSON 数组或对象数组。
- 节点之间的有向连线代表下游节点会读取上游节点的文字、图片或视频结果。
- 模板运行时由控制台后端执行，不调用原 `run_workflow.py` 的固定阶段流水线。
- 同一个图片节点可以用一个 prompt 生成多张图；如果需要多条 prompt 各生成一张图，可以创建多个图片节点并分别配置 prompt。

当前支持的节点类型：

| 节点类型 | 常用操作 | 说明 |
| --- | --- | --- |
| `material` | `material_lookup` | 默认按输入里的 `theme`、`character` 从服务器素材库查找 PDD 参考图，不读取浏览器本地“我的素材”；也可以在节点配置中绑定固定素材，例如 mockup 底版 |
| `text` | `input`、`text_static`、`text_generation`、`condition`、`script` | 输出输入对象、静态 prompt、调用文本模型、按 JSONPath 分支，或执行受控脚本 |
| `image` | `image_generation`、`image_edit`、`image_select` | 调用图片模型；如果需要使用上游图片作为参考，操作必须选择 `image_edit`；`image_select` 用于从上游图片中选择当前有效图，常用于修复循环后的结果收敛 |
| `video` | `video_generation` | 调用视频模型；第一版不允许把视频结果继续作为下游模型输入 |

## 无代码图片节点

新模板优先使用“复合图片节点”。复合图片节点在画布上仍然是一个普通图片节点，但可以在节点配置里开启 `内置质检 / 修复`：

- `PDD 源图`：生成后自动质检，major/critical 问题才修复，minor 只记录；修复耗尽后可重新生成。
- `PDD Mockup`：检查产品结构、抱枕套形态和规格图可读性；默认更偏向重试或人工复查。
- `PDD 最终主图`：检查主体占比、SKU 一致性、中文文案和电商可读性；可自动修复主图。
- `通用图片`：给后续文章、视频等非 PDD 工作流复用。

复合图片节点的内部流程不会默认铺到主画布上：

```text
图片生成/编辑 -> 质检 -> 修复/重生 -> 最终输出
```

内部产物写入：

```text
runs/<run_id>/logs/custom_workflow/products/<product>/nodes/<node_id>/guardrail/
```

节点状态文件会记录 `internal_steps` 和 `guardrail` 摘要，运行页可用它展示内部轮次。下游节点只接收最终有效图片。

新增控制流能力：

- `text_generation` 节点可以选择输出格式为普通文本或 JSON；选择 JSON 时，后端会要求模型结果可解析为 JSON，适合质检、复检、分类和审核。
- `text_generation` 节点可以配置 `titleProvider=true`。节点输出 JSON 中的 `title` / `productTitle` / `product_title` / `name` 会更新当前商品标题，供后续 `{{productTitle}}` 输出路径使用。
- `condition` 节点读取上游 JSON，通过节点配置里的 JSONPath 规则输出 `decision`，下游连线可按 `fromHandle` 或连线条件分流。
- 带 `fromHandle` 或 `condition` 的连线会作为目标节点的门控边；门控失败时，下游节点不会仅因为其它素材/图片输入已就绪而提前运行。
- 连线可配置为受控循环边，并必须设置最大轮次，用于 `质检 -> 条件 -> 修复 -> 质检` 这类流程。
- `script` 节点只允许运行仓库内相对路径脚本。`executor=vps` 在 VPS 上执行；`executor=local_agent` 会等待本地 agent 领取任务，适合本地 PDD 上传。

默认 PDD 商品主图模板包含：

- 素材库参考图匹配。
- 源图生成、源图质检、源图判定、源图修复、当前源图选择。
- `Mockup生成`：读取当前源图和固定 `规格图模板` 素材，用 Image 2 生成规格图 mockup。
- `标题生成`：用文本模型输出 JSON 标题并更新 `{{productTitle}}`。
- 最终主图、主图质检、主图判定、主图修复、当前主图选择。
- `产物打包`：调用 `pdd/scripts/package_custom_workflow_product.py`，把源图、规格图、主图写回传统 PDD 目录结构。
- `同步本地/可选上传`：调用 `pdd/scripts/trigger_local_receive_and_upload.sh`，在打包完成后通过本地反向 SSH 触发本机拉回产物；如需同步后立即本地上传，可在该脚本节点参数中追加 `--upload`。

模板 prompt 和输出路径支持变量：

```text
{{input.theme}}
{{input.character}}
{{productTitle}}
{{productTitleRaw}}
{{productKey}}
{{sourceTitle}}
{{sourceProduct}}
{{input.index}}
{{node.<node_id>.text}}
{{node.<node_id>.first_file}}
{{node.<node_id>.files_json}}
{{index}}
{{index1}}
{{index4}}
```

模板运行产物会写入：

```text
runs/<run_id>/logs/custom_workflow/template_snapshot.json
runs/<run_id>/logs/custom_workflow/inputs.json
runs/<run_id>/logs/custom_workflow/products/<index>_<product_key>/nodes/<node_id>/
runs/<run_id>/logs/custom_workflow.log
runs/<run_id>/logs/workflow_status.json
runs/<run_id>/logs/product_pipeline_summary.json
runs/<run_id>/logs/product_pipeline/<product_key>/pipeline_status.json
```

如果节点配置了输出路径映射，结果还会复制到模板指定的位置。当前默认 PDD 商品主图模板不直接使用图片节点映射业务目录，而是在最后通过 `产物打包` 脚本统一落盘：

```text
generated/<原主题 - 角色>/0001.png
待上架/<生成商品标题>/规格图/1_规格图.png
待上架/<生成商品标题>/主图/1_主图.png
```

内部节点原始输出只用于画布详情、日志和调试；业务产物目录保持和传统 PDD workflow 一致，源图进入 `generated/`，规格图和主图进入 `待上架/`。

`产物打包` 只负责 VPS 上的业务目录落盘，不负责同步本地。同步和可选上传由后续独立的 `同步本地/可选上传` 节点执行，画布运行链路会明确显示：

```text
产物打包 -> 同步本地/可选上传
```

该节点默认参数只同步本地：

```json
["--run-id", "{{runId}}"]
```

如果需要同步后立即在本地执行 PDD 上传，可以把参数改为：

```json
["--run-id", "{{runId}}", "--upload"]
```

执行该节点前，本地需要提前建立反向 SSH，使 VPS 能访问本地 SSH：

```bash
autossh -M 0 -N \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  -o ExitOnForwardFailure=yes \
  -p 443 \
  -R 22222:127.0.0.1:22 \
  root@96.9.225.98
```

默认脚本会从 VPS 连接 `127.0.0.1:22222`，进入本地 `/home/shiyi/Apps/VScode/auto-workflow/pdd` 后执行 `scripts/local_receive_and_maybe_upload_from_vps.sh`。

模板执行器会复用后台模型渠道配置。当前 VPS 控制台使用 chatgpt2api 渠道，模型名按后台配置写为 `gpt-5-5`、`gpt-image-2`、`sora-2` 等。

## 本地 Agent 脚本节点

当 `script` 节点选择 `executor=local_agent` 时，VPS 控制台只负责创建脚本任务，本地 agent 负责在本机仓库目录执行脚本并回传输出。长连接/轮询不会消耗模型 token，只消耗少量网络心跳。

VPS 需要配置：

```env
LOCAL_AGENT_TOKEN=一段足够长的随机令牌
```

本地启动示例：

```bash
go run ./cmd/local-agent \
  --server http://127.0.0.1:13000 \
  --token "$LOCAL_AGENT_TOKEN" \
  --root /home/shiyi/Apps/VScode/auto-workflow/pdd
```

脚本路径必须是 `--root` 内的相对路径，例如 `vendor/pdd_uploader_app/scripts/upload_products.sh`。`.sh` 会用 `bash` 执行，`.py` 会用本地 `python` 执行，其他文件按可执行文件处理。

## 受控动作

控制台只允许后端内置的 allowlist 动作，不接受任意 shell：

- 续跑当前 run：固定追加 `--skip-upload`。
- 停止当前 run。
- 健康检查。
- 查看 Docker 状态。
- 重启 `chatgpt2api`、`sub2api`、`cli-proxy-api-vps`。
- WARP 重连。

所有动作会写入 `PDD_ACTION_AUDIT_LOG`。

## 边界

- 控制台不负责拼多多本地上传。
- VPS workflow 仍固定跳过上传。
- 本地 PDD 上传仍按原项目脚本执行。
- 控制台启动任务会写入模板快照、输入、节点状态和映射后的业务产物；查看页面本身只读取和展示 run 目录。

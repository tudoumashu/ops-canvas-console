# 后端数据库说明

本文档只记录后端当前已经使用的主要数据表。

## 数据库

后端使用 GORM 管理数据库连接和表结构迁移。

支持的存储驱动：

- `sqlite`
- `mysql`
- `postgresql`

当前启动时执行 `AutoMigrate`，自动维护以下表：

- `users`
- `credit_logs`
- `prompts`
- `assets`
- `settings`
- `workflow_templates`
- `workflow_runs`

后续新增表时再同步补充本文档，未实际使用的规划表不提前写入。

### users

系统用户表。用户基础信息、角色、算力点余额和第三方登录标识放在该表中。

| 字段              | 类型     | 说明                       |
|-----------------|--------|--------------------------|
| `id`            | string | 主键                       |
| `username`      | string | 用户名，唯一索引                 |
| `password`      | string | 密码哈希                     |
| `email`         | string | 邮箱                       |
| `display_name`  | string | 昵称                       |
| `avatar_url`    | string | 头像地址                     |
| `role`          | string | 角色：`user`、`admin`        |
| `credits`       | number | 算力点余额                    |
| `aff_code`      | string | 用户自己的邀请码，唯一索引            |
| `aff_count`     | number | 已邀请用户数量，冗余统计字段           |
| `inviter_id`    | string | 邀请人用户 ID                 |
| `github_id`     | string | GitHub 用户 ID               |
| `linux_do_id`   | string | Linux.do 用户 ID            |
| `wechat_id`     | string | 微信用户 ID                   |
| `status`        | string | 用户状态：`active`、`ban`       |
| `last_login_at` | string | 最近登录时间                   |
| `extra`         | json   | 扩展信息，第三方资料按平台命名空间保存，如 `linuxDo` |
| `created_at`    | string | 创建时间                     |
| `updated_at`    | string | 更新时间                     |

### prompts

提示词表。用于保存公开提示词、内置 GitHub 系统提示词、分类和预览内容。

| 字段           | 类型     | 说明                           |
|--------------|--------|------------------------------|
| `id`         | string | 主键                           |
| `title`      | string | 标题                           |
| `cover_url`  | string | 封面图                          |
| `prompt`     | string | 提示词内容                        |
| `tags`       | json   | 标签列表                         |
| `category`   | string | 分类标识                         |
| `domain`     | string | 领域：`general`、`image`、`text`、`video`、`workflow` 等 |
| `stage`      | string | 阶段：`source_generation`、`quality_review`、`main_image` 等 |
| `provider`   | string | 模型提供方或来源标识                  |
| `model`      | string | 推荐模型名                         |
| `mode`       | string | 模式或工作流模式                      |
| `input_type` | string | 输入类型：`text`、`image`、`images`、`json`、`video` 等 |
| `output_type` | string | 输出类型：`text`、`image`、`images`、`json`、`video` 等 |
| `status`     | string | 状态：`production`、`draft`、`deprecated`、`test` 等 |
| `metadata`   | json   | 扩展元数据                         |
| `preview`    | text   | Markdown 展示内容，可包含文本、图片、视频链接等 |
| `created_at` | string | 创建时间                         |
| `updated_at` | string | 更新时间                         |

`github_url` 仅用于接口返回，不写入数据库。PDD 工作流生产提示词不再在服务启动时写入该表。

### assets

素材表。当前用于后台素材库。

| 字段               | 类型     | 说明                            |
|------------------|--------|-------------------------------|
| `id`             | string | 主键                            |
| `title`          | string | 标题                            |
| `type`           | string | 素材类型：`text`、`image`、`video` 等 |
| `media_type`     | string | 媒体类型，供筛选使用                    |
| `scope`          | string | 作用域：`library` 等                 |
| `category_path`  | string | 结构化分类路径，如 `角色参考图/标准参考图`    |
| `purpose`        | string | 用途：`generic`、`standard_reference`、`official_reference`、`spec_template` 等 |
| `source`         | string | 来源：`cloud_asset`、`local_upload`、`ai_generated` 等 |
| `cover_url`      | string | 封面图                           |
| `tags`           | json   | 标签列表                          |
| `category`       | string | 分类标识                          |
| `description`    | string | 描述                            |
| `content`        | text   | 文本或 Markdown 内容               |
| `url`            | string | 图片、视频等媒体地址                    |
| `metadata`       | json   | 扩展元数据，如原始路径、IP、角色名等          |
| `created_at`     | string | 创建时间                          |
| `updated_at`     | string | 更新时间                          |

工作流素材启动时会以稳定 ID 写入该表。角色参考图使用通用分类路径 `角色参考图/标准参考图` 或 `角色参考图/官方参考图`，`purpose` 分别为 `standard_reference` 或 `official_reference`；控制台固定规格图底版使用 `categoryPath=规格图模板`、`purpose=spec_template`。`url`/`cover_url` 指向受限的本地素材文件读取接口。

### workflow_templates

自定义工作流模板表。当前主要用于 PDD 工作流模板画布，只保存定义，不在编辑画布时调用模型。

| 字段            | 类型     | 说明                                      |
|---------------|--------|-----------------------------------------|
| `id`          | string | 主键                                      |
| `workflow_type` | string | 工作流类型，当前为 `pdd`                     |
| `title`       | string | 模板标题                                    |
| `description` | string | 模板说明                                    |
| `spec`        | json   | 模板 DAG，包含节点、连线、模型、prompt、输出路径和并发设置 |
| `created_at`  | string | 创建时间                                    |
| `updated_at`  | string | 更新时间                                    |

`spec.nodes` 当前支持 `material`、`text`、`image`、`video` 四类节点；`operation` 当前支持 `input`、`material_lookup`、`text_static`、`text_generation`、`condition`、`script`、`image_generation`、`image_edit`、`video_generation`。`text_generation` 可通过 `extra.outputFormat=json` 输出结构化 JSON；`spec.edges` 可保存 `fromHandle`、`condition` 和 `loop`，用于 JSONPath 条件分流和受控循环。

### workflow_runs

自定义工作流运行记录表。它只保存运行状态和模板快照；图片、日志和 JSON 产物仍写入 `PDD_RUNS_ROOT/<run_id>`。

| 字段              | 类型     | 说明                               |
|-----------------|--------|----------------------------------|
| `id`            | string | run_id，主键                        |
| `workflow_type` | string | 工作流类型，当前为 `pdd`                |
| `template_id`   | string | 来源模板 ID                         |
| `template_title` | string | 来源模板标题冗余                       |
| `status`        | string | 状态：`idle`、`running`、`success`、`error` |
| `run_dir`       | string | VPS 上的 run 目录                    |
| `input_count`   | number | 输入商品数量                          |
| `completed_count` | number | 已完成商品数量                         |
| `failed_count`  | number | 失败商品数量                          |
| `error`         | string | 失败信息，可为空                        |
| `spec_snapshot` | json   | 启动时的模板快照，用于复现                |
| `created_at`    | string | 创建时间                            |
| `updated_at`    | string | 更新时间                            |

### settings

系统配置表，只保存两行数据：`public` 放前端可读取的公开配置，`private` 放仅后端和管理员可读取的私有配置，配置值都用 JSON。

| 字段           | 类型     | 说明                    |
|--------------|--------|-----------------------|
| `key`        | string | 主键：`public`、`private` |
| `value`      | json   | 配置内容                  |
| `created_at` | string | 创建时间                  |
| `updated_at` | string | 更新时间                  |

`public.value` 常放前端展示和可公开读取的配置，例如模型列表、登录开关等。
`private.value` 常放渠道密钥、登录密钥、后台内部开关等。

当前系统设置接口会按后端结构体序列化和反序列化已知字段；数据库 JSON 中额外存在的旧字段会被忽略。

`public.value` 当前字段：

| 字段                | 类型       | 说明             |
|-------------------|----------|----------------|
| `modelChannel` | object | 模型渠道公开配置组 |
| `auth` | object | 公开登录配置 |

`modelChannel` 当前字段：

| 字段                | 类型       | 说明             |
|-------------------|----------|----------------|
| `availableModels` | string[] | 系统可用模型列表       |
| `modelCosts` | object[] | 模型算力点配置       |
| `defaultModel`    | string   | 默认模型           |
| `defaultImageModel` | string | 默认图片模型         |
| `defaultVideoModel` | string | 默认视频模型         |
| `defaultTextModel` | string  | 默认文本模型         |
| `systemPrompt`    | string   | 系统提示词          |
| `allowCustomChannel` | bool    | 是否允许用户自定义渠道，默认允许，关闭后前端只提供走后端渠道的模式 |

`modelCosts` 每项字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 模型名称 |
| `credits` | number | 每次后端模型接口调用前预扣的算力点，未配置默认不扣除 |

`auth.linuxDo` 当前字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | bool | 是否开启 Linux.do 登录 |

`private.value` 当前字段：

| 字段         | 类型       | 说明       |
|------------|----------|----------|
| `channels` | object[] | 模型渠道配置列表 |
| `promptSync` | object | GitHub 远程提示词定时同步配置 |
| `auth` | object | 私有登录配置 |

`channels` 每项字段：

| 字段       | 类型       | 说明       |
|----------|----------|----------|
| `protocol` | string | 协议，当前支持 `openai` |
| `name`   | string   | 渠道名称     |
| `baseUrl` | string  | 渠道接口地址   |
| `apiKey` | string   | 渠道密钥     |
| `models` | string[] | 渠道可用模型列表 |
| `weight` | number   | 渠道权重，同一模型命中多个渠道时按权重随机 |
| `enabled` | bool    | 是否启用     |
| `remark` | string   | 备注       |

`promptSync` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | bool | 是否开启定时同步，默认开启 |
| `cron` | string | Cron 表达式，默认每 5 分钟 |

`auth.linuxDo` 当前字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `clientId` | string | Linux.do OAuth App Client ID |
| `clientSecret` | string | Linux.do OAuth App Client Secret，后台返回时隐藏 |

后端请求模型时，先按模型名筛选启用且包含该模型的渠道，再按 `weight` 加权随机选择一个渠道。

### credit_logs

用户算力点变更流水表。当前记录后台手动调整、模型调用预扣和模型调用失败返还。

| 字段           | 类型     | 说明                       |
|--------------|--------|--------------------------|
| `id`         | string | 主键                       |
| `user_id`    | string | 关联用户 ID                  |
| `type`       | string | 类型：`admin_adjust`、`ai_consume`、`ai_refund` |
| `amount`     | number | 本次变动数量，增加为正，扣减为负         |
| `balance`    | number | 变动后的用户算力点余额              |
| `related_id` | string | 关联业务 ID，可为空                |
| `remark`     | string | 备注                       |
| `extra`      | json   | 扩展信息                     |
| `created_at` | string | 创建时间                     |

`type` 当前取值：

| 值 | 说明 |
| --- | --- |
| `admin_adjust` | 后台手动调整 |
| `ai_consume` | 调用后端模型接口消费 |
| `ai_refund` | 后端模型接口调用失败返还 |

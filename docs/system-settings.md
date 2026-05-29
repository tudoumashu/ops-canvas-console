# 系统配置数据结构

系统配置保存在 `settings` 表中，目前只使用两行：

| key | 说明 |
| --- | --- |
| `public` | 公开配置，前端可以读取 |
| `private` | 私有配置，只给后端和管理员使用 |

## public.value

```json
{
  "modelChannel": {
    "availableModels": ["gpt-5.5", "gpt-image-2"],
    "modelCosts": [
      { "model": "gpt-5.5", "credits": 1 },
      { "model": "gpt-image-2", "credits": 10 }
    ],
    "defaultModel": "gpt-image-2",
    "defaultImageModel": "gpt-image-2",
    "defaultVideoModel": "veo_3_1_t2v_fast_landscape",
    "defaultTextModel": "gpt-5.5",
    "systemPrompt": "",
    "allowCustomChannel": true
  },
  "auth": {
    "allowRegister": true,
    "linuxDo": {
      "enabled": false
    }
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `modelChannel` | object | 模型渠道公开配置组 |
| `auth` | object | 认证相关公开配置 |

`modelChannel` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `availableModels` | string[] | 系统可用模型，由管理员手动选择；页面下拉选项可来自私有渠道模型 |
| `modelCosts` | object[] | 模型算力点配置，后端模型接口调用前按模型预扣，上游失败时返还；未配置默认不扣除 |
| `defaultModel` | string | 默认模型，从 `availableModels` 中选择 |
| `defaultImageModel` | string | 默认图片模型，从 `availableModels` 中选择 |
| `defaultVideoModel` | string | 默认视频模型，从 `availableModels` 中选择 |
| `defaultTextModel` | string | 默认文本模型，从 `availableModels` 中选择 |
| `systemPrompt` | string | 系统提示词 |
| `allowCustomChannel` | boolean | 是否允许用户在配置弹窗中切换为本地直连渠道，默认允许 |

`modelCosts` 每项字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 模型名称 |
| `credits` | number | 每次后端模型接口调用前预扣的算力点 |

用户侧请求模式：

| 模式 | 说明 |
| --- | --- |
| 云端渠道 | 使用后端 `/api/v1/*` 代理接口，请求会按模型名匹配 `private.value.channels` 中的可用渠道 |
| 本地直连 | 默认可选；`allowCustomChannel` 关闭后不可选，用户在浏览器本地配置 `baseUrl`、`apiKey` 和模型列表后直接请求模型接口 |

`auth` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `allowRegister` | boolean | 是否允许用户注册，默认允许；关闭后注册入口隐藏，注册接口拒绝新用户创建 |
| `linuxDo.enabled` | boolean | 是否开启 Linux.do 登录 |

## private.value

```json
{
  "channels": [
    {
      "protocol": "openai",
      "name": "默认渠道",
      "baseUrl": "https://api.example.com",
      "apiKey": "sk-xxx",
      "models": ["gpt-5.5", "gpt-image-2"],
      "weight": 1,
      "enabled": true,
      "remark": ""
    }
  ],
  "promptSync": {
    "enabled": true,
    "cron": "*/5 * * * *"
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `channels` | object[] | 模型渠道列表 |
| `promptSync` | object | GitHub 远程提示词定时同步配置 |

`channels` 每项字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `protocol` | string | 协议；`openai` 表示 OpenAI 兼容接口，`flow2api` 表示通过 Flow2API 的 `/chat/completions` 媒体返回适配为图片/视频接口 |
| `name` | string | 渠道名称 |
| `baseUrl` | string | OpenAI 兼容接口地址 |
| `apiKey` | string | 渠道密钥 |
| `models` | string[] | 该渠道可用模型 |
| `weight` | number | 渠道权重；同一模型有多个可用渠道时按权重随机 |
| `enabled` | boolean | 是否启用 |
| `remark` | string | 备注 |

后端调用模型时，会从已启用、已配置 `baseUrl` 和 `apiKey`、且 `models` 包含目标模型的渠道中选择一个。`flow2api` 渠道目前用于图片和视频模型：后端会把控制台现有的 `/api/v1/images/*`、`/api/v1/videos` 请求转换为 Flow2API 的聊天式媒体请求，并解析返回的图片或视频链接。

Flow2API 视频生成结果会写入 `VIDEO_STORAGE_ROOT`，默认 `data/video`。Docker 部署时该目录应位于持久化 `data/` 卷内，避免容器重启后视频工作台生成结果无法继续预览或下载。

`promptSync` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | boolean | 是否开启定时同步，默认开启 |
| `cron` | string | Cron 表达式，默认每 5 分钟 |

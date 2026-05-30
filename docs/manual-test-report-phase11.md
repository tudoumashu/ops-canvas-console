# Phase 11 Manual Test Report

## Scope

本报告记录 hybrid ecommerce local workspace -> VPS PDD API backend 的目标验收状态。

已实现的黄金路径：

- `opsc ecommerce import-template`：从已确认 VPS PDD template API 导入远端 template，重建为本地 canonical template。
- `opsc ecommerce create-run`：基于已导入 hybrid template 创建 pending local run、template snapshot、pending node state 和 `run.waiting_for_executor` event。
- `opsc executor --run <run_id>`：对 `metadata.hybridEcommerce.backend=vps_pdd` 模板创建远端 run、轮询 overview/product-detail、下载 key artifact，并写回本地 canonical artifact/ref/events/node state。
- `tools/hybrid_ecommerce_vps_smoke.py`：只编排上述 `opsc` 命令，不直接读写 workspace 文件，不直接调用 VPS API，不打印 secret；支持显式 env `secretRef` 或已有 workspace profile/channel 两种凭据路径。

非目标保持不变：

- 不迁移现有 PDD/VPS run。
- 不把 VPS run dir 当 local workspace canonical source。
- 不扩大 MCP 写面。
- 不新增危险 ops、stop/restart 或 host allowlist 能力。
- 不做 generic sync platform。

## Current Result

状态：`PASS`

已确认环境：

- VPS：`96.9.225.98`。
- 访问方式：本机 SSH key 连接 VPS `443` 端口，并通过 SSH tunnel 把本地 `127.0.0.1:18180` 转发到 VPS 本机 API `127.0.0.1:18080`。
- API health：VPS 本机 `/api/health` 返回 `ok`。
- 已确认远端模板：`workflow-template-381c428b-fc1c-43b4-9b2f-7ce885e3e29e`，标题 `电商商品主图模板 v2`，`workflowType=pdd`。
- 凭证来源：从 VPS 已部署画布项目的 server-side admin login 路径生成临时 admin token，并只作为本地 `OPSC_HYBRID_VPS_TOKEN` env `secretRef` 注入 smoke；未输出 token，未写入 workspace JSON 或文档。

真实 VPS smoke 结果：

- 本地导入模板成功，生成本地 template `tpl_01KSWZBMGT7TN7A0PMV137XA93`。
- 本地创建 run 成功，生成本地 run `run_01KSWZBMRTMT9V5H8MB9BEBK2C`。
- executor 成功 dispatch 远端 run `hybrid_run_01KSWZBMRTMT9V5H8MB9BEBK2C`。
- local run 最终状态为 `success`。
- 8 个模板节点均回写为 `success`：`input`、`reference`、`source`、`mockup_base`、`mockup`、`main`、`package`、`sync_local`。
- 本地 canonical artifact/ref 同步成功，`artifactCount=5`。
- 远端日志显示 run 完成，耗时约 `1744.8s`；其中 `source` 的 `image_edit` 节点耗时最长。

## Evidence

### First Probe

第一次真实 smoke 使用了低成本占位输入 `white pillow`。该请求成功完成本地导入、run 创建、远端 dispatch 和错误同步，但远端素材匹配失败：

```text
素材库未匹配到输入角色：white pillow
```

结论：这不是 local workspace/hybrid executor bug，而是已确认模板需要使用 VPS 素材库中存在的角色字段。

### Successful Smoke Input

第二次 smoke 使用 VPS 素材库中已存在的角色：

```json
{
  "inputs": [
    {
      "productTitle": "羽川翼抱枕 Phase11 Smoke",
      "theme": "物语系列",
      "character": "羽川翼",
      "style": "clean ecommerce product image"
    }
  ],
  "productConcurrency": 1,
  "maxRetries": 0
}
```

Command shape：

```bash
OPSC_HYBRID_VPS_TOKEN=<redacted> \
OPSC_HYBRID_VPS_URL=http://127.0.0.1:18180 \
OPSC_HYBRID_REMOTE_TEMPLATE_ID=workflow-template-381c428b-fc1c-43b4-9b2f-7ce885e3e29e \
OPSC_BIN=/tmp/opsc-phase11 \
python3 tools/hybrid_ecommerce_vps_smoke.py \
  --workspace <tmp-workspace> \
  --input-file <tmp-input.json> \
  --evidence <tmp-evidence.json> \
  --timeout 1800
```

Observed redacted summary：

```json
{
  "workspace": "<redacted>",
  "remoteUrl": "http://127.0.0.1:18180",
  "remoteTemplateId": "workflow-template-381c428b-fc1c-43b4-9b2f-7ce885e3e29e",
  "profile": "",
  "channel": "",
  "secretEnv": "OPSC_HYBRID_VPS_TOKEN",
  "credentialSource": "envSecretRef",
  "ok": true,
  "templateId": "tpl_01KSWZBMGT7TN7A0PMV137XA93",
  "runId": "run_01KSWZBMRTMT9V5H8MB9BEBK2C",
  "runStatus": "success",
  "artifactCount": 5,
  "executorProcessed": 1
}
```

Final local status summary：

```json
{
  "run": "run_01KSWZBMRTMT9V5H8MB9BEBK2C",
  "status": "success",
  "artifactCount": 5,
  "latestEventSequence": 274
}
```

Sanitized remote log tail：

```text
node completed product=羽川翼抱枕 Phase11 Smoke node=source files=1
node completed product=羽川翼抱枕 Phase11 Smoke node=mockup files=1
node completed product=羽川翼抱枕 Phase11 Smoke node=main files=1
node completed product=羽川翼抱枕 Phase11 Smoke node=package files=1
node completed product=羽川翼抱枕 Phase11 Smoke node=sync_local files=1
product completed key=羽川翼抱枕 Phase11 Smoke
run completed products=1 duration=1744.8s
```

Artifact sample：

```json
{
  "artifact": {
    "id": "art_01KSX117CPH48SZ4T3MPYGQM82",
    "type": "image",
    "mime": "image/png",
    "title": "output_01.png",
    "bytes": 1046120,
    "width": 1024,
    "height": 1536,
    "privacy": "private",
    "original": "files/original.png"
  },
  "ref": {
    "artifactId": "art_01KSX117CPH48SZ4T3MPYGQM82",
    "role": "primary_output",
    "nodeId": "reference",
    "slot": "artifact"
  }
}
```

## Remaining Manual Checks

- 仍需在真实 Web UI 中用 local workspace 连接入口发起同一个 hybrid template run，确认浏览器只连 `opsc serve`，不保存 VPS token/cookie/secret。
- 仍需在真实长期 workspace 中确认 artifact 预览、刷新后 run status 回显、失败错误展示和断开本地工作区后的提示。
- 真实模型调用已证明可完成一条最小链路，但耗时接近 30 分钟，后续产品化需要补 watch/worker 体验和更细粒度远端事件同步。

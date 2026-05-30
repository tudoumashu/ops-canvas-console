# Phase 11 Manual Test Report

## Scope

本报告记录 hybrid ecommerce local workspace -> VPS PDD API backend 的目标验收状态。

已实现的本地链路：

- `opsc ecommerce import-template`：通过 workspace profile/channel `secretRef` 导入已确认远端 PDD template，重建为本地 canonical template。
- `opsc ecommerce create-run`：基于已导入 hybrid template 创建 pending local run、template snapshot、pending node state 和 `run.waiting_for_executor` event。
- `opsc executor --run <run_id>`：对 `metadata.hybridEcommerce.backend=vps_pdd` 模板创建远端 run、轮询 overview/product-detail、下载 key artifact，并写回本地 canonical artifact/ref/events/node state。
- `tools/hybrid_ecommerce_vps_smoke.py`：只编排上述 `opsc` 命令，不直接读写 workspace 文件，不直接调用 VPS API，不打印 secret。

非目标保持不变：

- 不迁移现有 PDD/VPS run。
- 不把 VPS run dir 当 local workspace canonical source。
- 不扩大 MCP 写面。
- 不新增危险 ops、stop/restart 或 host allowlist 能力。
- 不做 generic sync platform。

## Current Result

状态：`BLOCKED`

原因：当前环境缺少真实 admin credential 和远端 template id，且目标 VPS API 从本机不可稳定访问；因此无法证明真实 VPS API end-to-end run 已成功。

已覆盖的要求：

- 本地 canonical template import / run draft / artifact ref 规则已由 Go 自动化测试覆盖。
- 浏览器不保存或发送 VPS admin credential 的边界保持不变；Phase 11 headless smoke 通过 `opsc` 和本机 `secretRef` 执行。
- smoke helper 已补齐，后续拿到可达 VPS API、credential 和 template id 后可一条命令复测。

未完成的要求：

- 尚未完成真实 VPS API smoke，即真实导入远端 template、创建 local run、executor 触发 VPS run、同步 artifacts 并确认本地 run 进入终态。

## Evidence

### Environment

当前本机未设置以下必要变量：

```text
OPSC_VPS_ADMIN_TOKEN
OPSC_HYBRID_VPS_TOKEN
PDD_ADMIN_TOKEN
OPSC_HYBRID_REMOTE_TEMPLATE_ID
OPSC_VPS_REMOTE_TEMPLATE_ID
OPSC_HYBRID_VPS_URL
OPSC_VPS_URL
```

### Network Probe

目标 VPS：`92.9.225.98`

| Probe | Result |
| --- | --- |
| TCP `22` | open |
| TCP `443` | open |
| TCP `80` | open |
| TCP `18080` | open |
| TCP `13000` | open |
| SSH `-p 443` | banner exchange timeout |
| SSH `-p 22` | key exchange closed by remote host |
| `http://92.9.225.98:18080/api/health` | empty reply or timeout |
| `http://92.9.225.98:13000/workflows/ecommerce` | empty reply |
| `http://92.9.225.98/api/health` | timeout |
| `https://92.9.225.98/api/health` | SSL connection timeout |

### Smoke Helper Prerequisite Check

Command shape:

```bash
tmp_input=$(mktemp)
tmp_evidence=$(mktemp)
printf '{"inputs":[{"productTitle":"smoke"}]}' > "$tmp_input"

tools/hybrid_ecommerce_vps_smoke.py \
  --workspace /tmp/opsc-phase11-real-smoke \
  --remote-url http://92.9.225.98:18080 \
  --remote-template remote_tpl_placeholder \
  --input-file "$tmp_input" \
  --evidence "$tmp_evidence"
```

Observed result:

```text
missing required smoke prerequisite: env OPSC_HYBRID_VPS_TOKEN
exit=2
```

Redacted evidence:

```json
{
  "workspace": "<redacted>",
  "remoteUrl": "http://92.9.225.98:18080",
  "remoteTemplateId": "remote_tpl_placeholder",
  "profile": "default",
  "channel": "vps",
  "secretEnv": "OPSC_HYBRID_VPS_TOKEN",
  "steps": [],
  "ok": false,
  "missing": [
    "env OPSC_HYBRID_VPS_TOKEN"
  ]
}
```

## Future Retest Command

拿到可达 VPS API、真实 admin credential 和已确认 remote template id 后，执行：

```bash
export OPSC_HYBRID_VPS_URL=http://92.9.225.98:18080
export OPSC_HYBRID_REMOTE_TEMPLATE_ID=<confirmed_template_id>
export OPSC_HYBRID_VPS_TOKEN=<admin_token>

tools/hybrid_ecommerce_vps_smoke.py \
  --workspace ~/OpsCanvas \
  --input-file /path/to/hybrid-input.json \
  --evidence /tmp/opsc-phase11-vps-smoke.json
```

也可以显式传参：

```bash
tools/hybrid_ecommerce_vps_smoke.py \
  --workspace ~/OpsCanvas \
  --remote-url http://92.9.225.98:18080 \
  --remote-template <confirmed_template_id> \
  --secret-env OPSC_HYBRID_VPS_TOKEN \
  --input-file /path/to/hybrid-input.json \
  --evidence /tmp/opsc-phase11-vps-smoke.json
```

验收标准：

- helper 输出 `ok: true`。
- evidence 中 `runStatus` 为 `success`。
- `artifactCount` 大于 `0`。
- workspace canonical `runs/<run_id>/` 只保存 run、node state、events 和 artifact refs。
- canonical artifact metadata 位于 `artifacts/<art_id>/artifact.json`。
- evidence、CLI 输出和 workspace JSON 不包含 token、workspace 绝对路径、远端 `runDir` 或 secret 文件路径。

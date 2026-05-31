# Phase 14 Manual Test Report

## Scope

Phase 14 只收口已确认电商模板的 local-first 执行黄金路径：

- `opsc ecommerce import-template --local-executable` 从已确认远端模板重建本地可执行 template。
- 本地 run 继续以 `runs/<run_id>/`、node state、events、artifact ref 为 canonical。
- executor 支持该模板需要的 `material_lookup` 自动本地素材匹配、`image_edit`、内置 mockup 底版 fallback、项目相对 `package` 和 `sync_local` marker。
- hybrid VPS-backed fallback 保留，不迁移历史 PDD/VPS run，不扩大 MCP 写面。

## Automated Result

Status: PASS

Commands:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine /usr/local/go/bin/gofmt -w internal/localworkspace/executor.go internal/localworkspace/local_ecommerce.go internal/localworkspace/hybrid_ecommerce.go internal/localworkspace/executor_test.go cmd/opsc/main.go cmd/opsc/main_test.go
docker run --rm -v "$PWD":/src -w /src -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine /usr/local/go/bin/go test ./internal/localworkspace ./cmd/opsc
cd web && npx tsc --noEmit
```

Coverage:

- CLI local-executable template import and local ecommerce run creation.
- Confirmed-template executor happy path with fake image-edit provider.
- Auto material lookup from a configured local `anime_ip` style library fixture; fixture tests now cover the `OPSC_LOCAL_ECOMMERCE_MATERIAL_LIBRARY` fallback instead of a hardcoded machine path.
- Built-in mockup base fallback when the browser did not pre-materialize the server asset.
- Canonical artifact writes, run artifact refs, node states, event order, project output files and idempotent rerun.
- Local-first event assertion that this path does not emit VPS/hybrid run orchestration events.
- Secret/root path redaction checks in the new local ecommerce executor path.

## Manual Status

Status: PASS

Real smoke completed:

- Web UI started a local-first ecommerce run from the imported template.
- `opsc executor --watch` claimed the run and executed it to `success`.
- The run used the configured local `anime_ip` material library and matched `药屋少女的呢喃 / 猫猫`.
- Real image-edit requests went through the workspace profile/channel `secretRef` path.
- The run wrote 7 canonical artifacts: image artifacts plus text package/sync marker artifacts.
- Project output mapping wrote 5 files under the project-relative ecommerce output directory.
- The browser run page opened an artifact preview modal successfully.
- Browser persistent storage check did not find model/VPS secret material, bearer token, launch secret, or secret file content.
- Run events stayed local-first: no `remote.run.dispatched` or hybrid remote event was emitted.

Smoke command shape:

```bash
python3 tools/local_ecommerce_browser_smoke.py \
  --workspace <isolated-workspace> \
  --web-url http://127.0.0.1:3000 \
  --serve-url http://127.0.0.1:<loopback-port> \
  --launch-secret <one-time-secret> \
  --opsc-bin ./.tmp/opsc.phase14 \
  --template-id <local-template-id> \
  --project-root <isolated-project-root> \
  --forbidden-secret-file <local-secret-file> \
  --input-json '{"productTitle":"猫猫抱枕","theme":"药屋少女的呢喃","work":"药屋少女的呢喃","animeIP":"药屋少女的呢喃","character":"猫猫"}' \
  --evidence <redacted-evidence-json>
```

Result summary:

```json
{
  "status": "success",
  "artifactCount": 7,
  "artifactTypes": ["image", "text"],
  "nodeStatuses": {
    "input": "success",
    "reference": "success",
    "mockup_base": "success",
    "source": "success",
    "mockup": "success",
    "main": "success",
    "package": "success",
    "sync_local": "success"
  },
  "projectOutputFiles": 5
}
```

Remaining follow-up is no longer a Phase 14 blocker: repeat the same flow in the user's real long-lived personal workspace and any future packaged/systemd startup environment.

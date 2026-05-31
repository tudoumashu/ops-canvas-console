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
- Auto material lookup from a local `anime_ip` style library fixture.
- Built-in mockup base fallback when the browser did not pre-materialize the server asset.
- Canonical artifact writes, run artifact refs, node states, event order, project output files and idempotent rerun.
- Secret/root path redaction checks in the new local ecommerce executor path.

## Manual Status

Status: PENDING

Not run in this pass:

- Real browser Web UI flow with a real long-lived workspace.
- Real local material library at the user machine default `anime_ip` path.
- Real OpenAI-compatible image-edit provider account.
- Real project output directory inspection and artifact preview after a Web-started run.

These remain Phase 14 manual checks and are listed in `docs/pending-test.md`.

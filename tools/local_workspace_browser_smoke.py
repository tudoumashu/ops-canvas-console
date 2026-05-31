#!/usr/bin/env python3
"""Minimal browser smoke for the Web UI local workspace adapter.

Prerequisites:
- `opsc serve --origin <web-url>` is already running.
- The Next.js Web UI is already running at `--web-url`.
- Python Playwright is installed and a Chromium/Chrome browser is available.

The smoke creates an isolated local template/run through browser fetch calls so
the same Origin, credentials, and HttpOnly session path are exercised. It then
opens the real run status page, waits for the pending -> success refresh, and
opens an image artifact preview.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

from playwright.sync_api import Error as PlaywrightError
from playwright.sync_api import sync_playwright


PNG_1X1_BASE64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/l6r3JwAAAABJRU5ErkJggg=="


def main() -> int:
    parser = argparse.ArgumentParser(description="Run a minimal local workspace browser smoke.")
    parser.add_argument("--web-url", default="http://127.0.0.1:3000")
    parser.add_argument("--serve-url", default="http://127.0.0.1:17680")
    parser.add_argument("--launch-secret", required=True)
    parser.add_argument("--browser-channel", default="chrome")
    parser.add_argument("--user-data-dir", default="", help="Optional persistent browser profile directory.")
    parser.add_argument("--evidence", default="", help="Optional path for JSON evidence output.")
    parser.add_argument("--headed", action="store_true")
    args = parser.parse_args()

    try:
        with sync_playwright() as playwright:
            browser = None
            context = None
            try:
                if args.user_data_dir:
                    context = playwright.chromium.launch_persistent_context(
                        user_data_dir=args.user_data_dir,
                        channel=args.browser_channel,
                        headless=not args.headed,
                    )
                    page = context.new_page()
                else:
                    browser = playwright.chromium.launch(channel=args.browser_channel, headless=not args.headed)
                    page = browser.new_page()
                page.goto(args.web_url.rstrip("/") + "/workflows/ecommerce", wait_until="domcontentloaded")
                result = page.evaluate(
                    """async ({ serveUrl, launchSecret, pngBase64 }) => {
                        const storeKey = "opsc:local_workspace_connection";
                        const api = async (path, init = {}) => {
                            const response = await fetch(`${serveUrl}${path}`, {
                                credentials: "include",
                                ...init,
                                headers: init.headers,
                            });
                            const payload = await response.json();
                            if (!response.ok || payload.code !== 0) {
                                throw new Error(payload.msg || `local api failed: ${path}`);
                            }
                            return payload.data;
                        };
                        const snapshotWorkspace = async () => {
                            const spec = [
                                ["templates", "/api/local/templates", "templates"],
                                ["runs", "/api/local/runs", "runs"],
                                ["artifacts", "/api/local/artifacts", "artifacts"],
                                ["profiles", "/api/local/profiles", "profiles"],
                            ];
                            const snapshot = {};
                            for (const [name, path, key] of spec) {
                                const data = await api(path);
                                const items = Array.isArray(data?.[key]) ? data[key] : [];
                                snapshot[name] = {
                                    count: items.length,
                                    sampleIds: items.map((item) => item.id).filter(Boolean).slice(0, 5),
                                };
                            }
                            return snapshot;
                        };
                        await fetch(`${serveUrl}/api/local/bootstrap/session`, {
                            method: "POST",
                            credentials: "include",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({ launchSecret }),
                        }).then(async (response) => {
                            const payload = await response.json();
                            if (!response.ok || payload.code !== 0) {
                                throw new Error(payload.msg || "bootstrap failed");
                            }
                        });
                        localStorage.setItem(storeKey, JSON.stringify({ state: { baseUrl: serveUrl }, version: 1 }));
                        const historyBefore = await snapshotWorkspace();

                        const template = await api("/api/local/templates", {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                data: {
                                    title: "Browser smoke local template",
                                    workflowType: "pdd",
                                    version: 1,
                                    nodes: [
                                        { id: "input", type: "input", operation: "input", title: "Input" },
                                        { id: "preview", type: "image", operation: "text_static", title: "Preview", prompt: "browser-smoke" },
                                    ],
                                    edges: [{ id: "input-preview", from: "input", to: "preview" }],
                                    settings: { productConcurrency: 1, maxRetries: 0 },
                                    metadata: { source: "browser-smoke" },
                                },
                            }),
                        });
                        const run = await api("/api/local/runs", {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                data: {
                                    templateId: template.id,
                                    status: "pending",
                                    input: { inputs: [{ productTitle: "browser smoke" }] },
                                    metadata: { source: "browser-smoke", executor: "test-harness" },
                                },
                            }),
                        });
                        await api(`/api/local/runs/${run.id}/nodes/input`, {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({ data: { nodeId: "input", status: "success", output: { input: { productTitle: "browser smoke" } } } }),
                        });
                        await api(`/api/local/runs/${run.id}/nodes/preview`, {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({ data: { nodeId: "preview", status: "pending" } }),
                        });
                        await api(`/api/local/runs/${run.id}/events`, {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                event: {
                                    type: "run.waiting_for_executor",
                                    level: "info",
                                    actor: { type: "web", id: "browser-smoke" },
                                    message: "browser smoke run created",
                                    data: { mode: "local" },
                                },
                            }),
                        });
                        return { runId: run.id, templateId: template.id, historyBefore };
                    }""",
                    {"serveUrl": args.serve_url.rstrip("/"), "launchSecret": args.launch_secret, "pngBase64": PNG_1X1_BASE64},
                )

                page.goto(args.web_url.rstrip("/") + f"/workflows/ecommerce/{result['runId']}", wait_until="domcontentloaded")
                page.get_by_text(result["runId"]).wait_for(timeout=15000)
                page.get_by_text("pending").first.wait_for(timeout=15000)

                artifact_result = page.evaluate(
                    """async ({ serveUrl, runId, templateId, pngBase64, historyBefore }) => {
                        const api = async (path, init = {}) => {
                            const response = await fetch(`${serveUrl}${path}`, {
                                credentials: "include",
                                ...init,
                                headers: init.headers,
                            });
                            const payload = await response.json();
                            if (!response.ok || payload.code !== 0) {
                                throw new Error(payload.msg || `local api failed: ${path}`);
                            }
                            return payload.data;
                        };
                        const snapshotWorkspace = async () => {
                            const spec = [
                                ["templates", "/api/local/templates", "templates", "/api/local/templates/"],
                                ["runs", "/api/local/runs", "runs", "/api/local/runs/"],
                                ["artifacts", "/api/local/artifacts", "artifacts", "/api/local/artifacts/"],
                                ["profiles", "/api/local/profiles", "profiles", "/api/local/profiles/"],
                            ];
                            const snapshot = {};
                            for (const [name, path, key] of spec) {
                                const data = await api(path);
                                const items = Array.isArray(data?.[key]) ? data[key] : [];
                                snapshot[name] = {
                                    count: items.length,
                                    sampleIds: items.map((item) => item.id).filter(Boolean).slice(0, 5),
                                };
                            }
                            return snapshot;
                        };
                        const assertHistoryPreserved = async (before, after) => {
                            const spec = {
                                templates: "/api/local/templates/",
                                runs: "/api/local/runs/",
                                artifacts: "/api/local/artifacts/",
                                profiles: "/api/local/profiles/",
                            };
                            for (const [name, pathPrefix] of Object.entries(spec)) {
                                const beforeEntry = before?.[name] || { count: 0, sampleIds: [] };
                                const afterEntry = after?.[name] || { count: 0, sampleIds: [] };
                                if (afterEntry.count < beforeEntry.count) {
                                    throw new Error(`${name} count regressed from ${beforeEntry.count} to ${afterEntry.count}`);
                                }
                                for (const id of beforeEntry.sampleIds || []) {
                                    await api(`${pathPrefix}${encodeURIComponent(id)}`);
                                }
                            }
                        };
                        const bytes = Uint8Array.from(atob(pngBase64), (char) => char.charCodeAt(0));
                        const file = new Blob([bytes], { type: "image/png" });
                        const form = new FormData();
                        form.set("data", JSON.stringify({
                            type: "image",
                            mime: "image/png",
                            title: "Browser smoke artifact",
                            privacy: "private",
                            source: { type: "browser_smoke", templateId, nodeId: "preview" },
                            metadata: { source: "browser-smoke" },
                        }));
                        form.set("fileKey", "original");
                        form.set("file", file, "browser-smoke.png");
                        const artifact = await api("/api/local/artifacts/import", { method: "POST", body: form });
                        await api(`/api/local/runs/${runId}/artifacts`, {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                data: {
                                    artifactId: artifact.id,
                                    role: "primary_output",
                                    nodeId: "preview",
                                    slot: "image",
                                    order: 0,
                                    metadata: { source: "browser-smoke" },
                                },
                            }),
                        });
                        await api(`/api/local/runs/${runId}/nodes/preview`, {
                            method: "PUT",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                revision: 1,
                                data: {
                                    nodeId: "preview",
                                    status: "success",
                                    output: { artifactIds: [artifact.id], artifactId: artifact.id },
                                    metadata: { source: "browser-smoke" },
                                },
                            }),
                        });
                        const current = await api(`/api/local/runs/${runId}`);
                        await api(`/api/local/runs/${runId}`, {
                            method: "PUT",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                revision: current.revision,
                                data: {
                                    ...current.data,
                                    status: "success",
                                    output: { completedNodes: 2 },
                                },
                            }),
                        });
                        await api(`/api/local/runs/${runId}/events`, {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                event: {
                                    type: "run.succeeded",
                                    level: "info",
                                    actor: { type: "system", id: "browser-smoke" },
                                    message: "browser smoke run succeeded",
                                    data: { artifactId: artifact.id },
                                },
                            }),
                        });
                        const historyAfter = await snapshotWorkspace();
                        await assertHistoryPreserved(historyBefore, historyAfter);
                        return { artifactId: artifact.id, historyAfter };
                    }""",
                    {
                        "serveUrl": args.serve_url.rstrip("/"),
                        "runId": result["runId"],
                        "templateId": result["templateId"],
                        "pngBase64": PNG_1X1_BASE64,
                        "historyBefore": result["historyBefore"],
                    },
                )

                page.get_by_text("success").first.wait_for(timeout=20000)
                page.get_by_role("button", name="预览").first.click()
                page.locator(".ant-modal img").first.wait_for(timeout=15000)
                storage = page.evaluate("() => JSON.stringify(window.localStorage)")
                for forbidden in [args.launch_secret, "Bearer ", "bearer.token", "launch.secret", "tokenFile", "launchSecretFile"]:
                    if forbidden and forbidden in storage:
                        raise RuntimeError("browser localStorage contains local runtime or credential material")
                payload = {"ok": True, **result, **artifact_result, "persistentProfile": bool(args.user_data_dir)}
                write_evidence(args.evidence, payload)
                print(json.dumps(payload, ensure_ascii=False))
            finally:
                if context:
                    context.close()
                if browser:
                    browser.close()
    except (PlaywrightError, Exception) as exc:
        payload = {"ok": False, "error": str(exc), "persistentProfile": bool(args.user_data_dir)}
        write_evidence(args.evidence, payload)
        print(json.dumps(payload, ensure_ascii=False), file=sys.stderr)
        return 1
    return 0


def write_evidence(path: str, payload: dict[str, Any]) -> None:
    if not path:
        return
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    raise SystemExit(main())

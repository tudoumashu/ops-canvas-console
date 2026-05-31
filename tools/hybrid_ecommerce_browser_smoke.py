#!/usr/bin/env python3
"""Browser smoke for the hybrid ecommerce local workspace path.

Prerequisites:
- `opsc serve --origin <web-url>` is already running for `--workspace`.
- The Next.js Web UI is already running at `--web-url`.
- Python Playwright is installed and a Chromium/Chrome browser is available.

The smoke uses a local fake VPS API. Browser setup creates a workspace profile
and template through `opsc serve`; the run itself is started from the real Web
template editor UI, then a real `opsc executor --watch` process dispatches and
syncs the fake remote run. No browser credential is used.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import Error as PlaywrightError
from playwright.sync_api import sync_playwright


PNG_BYTES = b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\xdac\xfc\xcf\xc0P\x0f\x00\x05\x83\x02\x7f\x97\xaa\xf7'\x00\x00\x00\x00IEND\xaeB`\x82"
SECRET_ENV_NAME = "OPSC_BROWSER_HYBRID_TOKEN"
SECRET_VALUE = "browser-hybrid-secret"


class FakeHybridState:
    def __init__(self) -> None:
        self.remote_run_id = ""
        self.overview_calls = 0


def main() -> int:
    parser = argparse.ArgumentParser(description="Run a hybrid ecommerce browser smoke.")
    parser.add_argument("--workspace", required=True)
    parser.add_argument("--web-url", default="http://127.0.0.1:3000")
    parser.add_argument("--serve-url", default="http://127.0.0.1:17680")
    parser.add_argument("--launch-secret", required=True)
    parser.add_argument("--opsc-bin", default="")
    parser.add_argument("--repo-root", default=str(Path(__file__).resolve().parents[1]))
    parser.add_argument("--browser-channel", default="chrome")
    parser.add_argument("--user-data-dir", default="", help="Optional persistent browser profile directory.")
    parser.add_argument("--success-timeout-ms", type=int, default=30000)
    parser.add_argument("--evidence", default="", help="Optional path for JSON evidence output.")
    parser.add_argument("--headed", action="store_true")
    args = parser.parse_args()

    state = FakeHybridState()
    server = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(state))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    remote_url = f"http://127.0.0.1:{server.server_port}"
    executor: subprocess.Popen[str] | None = None

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
                else:
                    browser = playwright.chromium.launch(channel=args.browser_channel, headless=not args.headed)
                    context = browser.new_context()
                page = context.new_page()
                page.goto(args.web_url.rstrip("/") + "/workflows/ecommerce", wait_until="domcontentloaded")
                setup = page.evaluate(
                    """async ({ serveUrl, launchSecret, remoteUrl }) => {
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

                        const profile = await api("/api/local/profiles", {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                data: {
                                    name: "Browser Hybrid VPS",
                                    mode: "hybrid",
                                    channels: [{
                                        id: "vps",
                                        name: "Fake VPS",
                                        protocol: "ops-canvas-vps",
                                        baseUrl: remoteUrl,
                                        enabled: true,
                                        secretRef: { type: "env", name: "OPSC_BROWSER_HYBRID_TOKEN" },
                                    }],
                                },
                            }),
                        });
                        const template = await api("/api/local/templates", {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({
                                data: {
                                    title: "Browser Hybrid Ecommerce",
                                    workflowType: "pdd",
                                    version: 1,
                                    nodes: [{ id: "stage_generate", type: "image", operation: "image_generation", title: "Generate" }],
                                    edges: [],
                                    settings: { productConcurrency: 1, maxRetries: 0, defaultProfileId: profile.id },
                                    metadata: {
                                        source: "browser-hybrid-smoke",
                                        hybridEcommerce: {
                                            version: 1,
                                            backend: "vps_pdd",
                                            remoteTemplateId: "remote_tpl",
                                            remoteTitle: "Browser Hybrid Ecommerce",
                                            profileId: profile.id,
                                            channelId: "vps",
                                            credentialMode: "profileChannel",
                                        },
                                    },
                                },
                            }),
                        });
                        return { templateId: template.id, profileId: profile.id };
                    }""",
                    {"serveUrl": args.serve_url.rstrip("/"), "launchSecret": args.launch_secret, "remoteUrl": remote_url},
                )

                env = os.environ.copy()
                env[SECRET_ENV_NAME] = SECRET_VALUE
                command = ([args.opsc_bin] if args.opsc_bin else ["go", "run", "./cmd/opsc"]) + [
                    "executor",
                    "--watch",
                    "--poll-interval=200ms",
                    "--workspace",
                    args.workspace,
                ]
                executor = subprocess.Popen(command, cwd=args.repo_root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

                page.goto(args.web_url.rstrip("/") + f"/workflows/ecommerce/templates/{setup['templateId']}", wait_until="domcontentloaded")
                run_button = page.get_by_role("button", name="运行模板")
                run_button.wait_for(timeout=15000)
                run_button.click()
                page.locator("textarea").fill('{"productTitle":"browser hybrid smoke"}')
                page.get_by_role("button", name="启动").click()
                page.wait_for_url("**/workflows/ecommerce/run_*", wait_until="domcontentloaded", timeout=60000)
                run_id = page.url.rstrip("/").split("/")[-1]
                page.get_by_role("heading", name=run_id).wait_for(timeout=15000)
                page.get_by_text("success").first.wait_for(timeout=args.success_timeout_ms)
                page.get_by_role("button", name="预览").first.click()
                page.locator(".ant-modal img").first.wait_for(timeout=15000)
                storage = page.evaluate("() => JSON.stringify(window.localStorage)")
                for forbidden in [
                    SECRET_VALUE,
                    SECRET_ENV_NAME,
                    args.launch_secret,
                    "Bearer ",
                    "bearer.token",
                    "launch.secret",
                    "tokenFile",
                    "launchSecretFile",
                ]:
                    if forbidden in storage:
                        raise RuntimeError("browser localStorage contains credential material")
                payload = {
                    "ok": True,
                    **setup,
                    "runId": run_id,
                    "overviewCalls": state.overview_calls,
                    "persistentProfile": bool(args.user_data_dir),
                }
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
    finally:
        if executor and executor.poll() is None:
            executor.terminate()
            try:
                executor.wait(timeout=5)
            except subprocess.TimeoutExpired:
                executor.kill()
        server.shutdown()
    return 0


def write_evidence(path: str, payload: dict[str, Any]) -> None:
    if not path:
        return
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def make_handler(state: FakeHybridState) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:
            if not self._authorized():
                return
            if self.path == "/api/admin/workflows/pdd/templates/remote_tpl/runs":
                length = int(self.headers.get("Content-Length", "0") or "0")
                payload = json.loads(self.rfile.read(length) or b"{}")
                state.remote_run_id = str(payload.get("runId") or "hybrid_browser_smoke")
                self._json({"runId": state.remote_run_id})
                return
            self.send_error(404)

        def do_GET(self) -> None:
            if not self._authorized():
                return
            parsed = urlparse(self.path)
            if parsed.path.endswith("/overview"):
                state.overview_calls += 1
                if state.overview_calls == 1:
                    self._json({
                        "run": {"runId": state.remote_run_id, "status": "running", "completed": False, "productTotal": 1, "runningProducts": 1},
                        "stages": [{"id": "stage_generate", "title": "Generate", "status": "running", "total": 1, "running": 1}],
                        "products": [{"key": "prod_1", "product": "browser hybrid smoke", "status": "running"}],
                    })
                    return
                self._json({
                    "run": {"runId": state.remote_run_id, "status": "success", "completed": True, "productTotal": 1, "completedProducts": 1},
                    "stages": [{"id": "stage_generate", "title": "Generate", "status": "success", "total": 1, "success": 1}],
                    "products": [{"key": "prod_1", "product": "browser hybrid smoke", "status": "success"}],
                })
                return
            if parsed.path.endswith("/product-detail"):
                query = parse_qs(parsed.query)
                if query.get("key", [""])[0] != "prod_1":
                    self.send_error(400)
                    return
                self._json({
                    "runId": state.remote_run_id,
                    "product": {"key": "prod_1", "product": "browser hybrid smoke", "status": "success"},
                    "nodes": [{
                        "id": "stage_generate",
                        "type": "image_generation",
                        "title": "Generate",
                        "status": "success",
                        "artifacts": [{"id": "a1", "title": "Preview", "path": "logs/custom_workflow/products/prod_1/nodes/stage_generate/output.png", "kind": "image", "mimeType": "image/png"}],
                    }],
                })
                return
            if parsed.path.endswith("/file"):
                self.send_response(200)
                self.send_header("Content-Type", "image/png")
                self.end_headers()
                self.wfile.write(PNG_BYTES)
                return
            self.send_error(404)

        def log_message(self, format: str, *args: Any) -> None:
            return

        def _authorized(self) -> bool:
            if self.headers.get("Authorization") != f"Bearer {SECRET_VALUE}":
                self.send_response(401)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"code":1,"data":null,"msg":"unauthorized"}')
                return False
            return True

        def _json(self, data: Any) -> None:
            body = json.dumps({"code": 0, "data": data, "msg": "ok"}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return Handler


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Browser smoke for the local-first ecommerce workflow path.

Prerequisites:
- `opsc serve --origin <web-url>` is already running for `--workspace`.
- The Next.js Web UI is already running at `--web-url`.
- `--template-id` points at an imported local executable ecommerce template.
- The template already carries default profile/channel/project metadata.

The script starts the real Web UI run flow and a real `opsc executor --watch`.
Only model/image API calls should leave the machine via the workspace profile
secretRef; browser storage is checked for credential material.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any

from playwright.sync_api import Error as PlaywrightError
from playwright.sync_api import sync_playwright


def main() -> int:
    parser = argparse.ArgumentParser(description="Run a local-first ecommerce browser smoke.")
    parser.add_argument("--workspace", required=True)
    parser.add_argument("--template-id", required=True)
    parser.add_argument("--web-url", default="http://127.0.0.1:3000")
    parser.add_argument("--serve-url", default="http://127.0.0.1:17680")
    parser.add_argument("--launch-secret", required=True)
    parser.add_argument("--opsc-bin", default="")
    parser.add_argument("--repo-root", default=str(Path(__file__).resolve().parents[1]))
    parser.add_argument("--browser-channel", default="chrome")
    parser.add_argument("--user-data-dir", default="", help="Optional persistent browser profile directory.")
    parser.add_argument("--success-timeout-ms", type=int, default=180000)
    parser.add_argument("--input-json", default='{"productTitle":"重云抱枕","theme":"原神","work":"原神","animeIP":"原神","character":"重云"}')
    parser.add_argument("--project-root", default="", help="Optional project root used for output file checks.")
    parser.add_argument("--forbidden-secret-file", default="", help="Optional file containing a secret value to check against browser storage.")
    parser.add_argument("--evidence", default="", help="Optional path for JSON evidence output.")
    parser.add_argument("--headed", action="store_true")
    args = parser.parse_args()

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
                serve_url = args.serve_url.rstrip("/")
                web_url = args.web_url.rstrip("/")

                page.goto(f"{web_url}/workflows/ecommerce", wait_until="domcontentloaded")
                setup = page.evaluate(
                    """async ({ serveUrl, launchSecret, templateId }) => {
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
                        const template = await api(`/api/local/templates/${encodeURIComponent(templateId)}`);
                        if (template?.data?.metadata?.localEcommerce?.backend !== "local_first") {
                            throw new Error("template is not local_first localEcommerce");
                        }
                        return {
                            templateId,
                            title: template.data.title,
                            defaultProfileId: template.data.settings?.defaultProfileId || "",
                            defaultProjectId: template.data.settings?.defaultProjectId || "",
                            channelId: template.data.metadata?.localEcommerce?.channelId || "",
                        };
                    }""",
                    {"serveUrl": serve_url, "launchSecret": args.launch_secret, "templateId": args.template_id},
                )
                if not setup.get("defaultProfileId") or not setup.get("defaultProjectId"):
                    raise RuntimeError("local executable template requires defaultProfileId and defaultProjectId for Web run")

                command = ([args.opsc_bin] if args.opsc_bin else ["go", "run", "./cmd/opsc"]) + [
                    "executor",
                    "--watch",
                    "--poll-interval=500ms",
                    "--workspace",
                    args.workspace,
                ]
                executor = subprocess.Popen(command, cwd=args.repo_root, env=os.environ.copy(), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

                page.goto(f"{web_url}/workflows/ecommerce/templates/{args.template_id}", wait_until="domcontentloaded")
                run_button = page.get_by_role("button", name="运行模板")
                run_button.wait_for(timeout=15000)
                run_button.click()
                page.locator("textarea").fill(args.input_json)
                page.get_by_role("button", name="启动").click()
                page.wait_for_url("**/workflows/ecommerce/run_*", wait_until="domcontentloaded", timeout=60000)
                run_id = page.url.rstrip("/").split("/")[-1]
                page.get_by_role("heading", name=run_id).wait_for(timeout=15000)

                status = page.evaluate(
                    """async ({ serveUrl, runId, timeoutMs }) => {
                        const deadline = Date.now() + timeoutMs;
                        const api = async (path) => {
                            const response = await fetch(`${serveUrl}${path}`, { credentials: "include" });
                            const payload = await response.json();
                            if (!response.ok || payload.code !== 0) {
                                throw new Error(payload.msg || `local api failed: ${path}`);
                            }
                            return payload.data;
                        };
                        while (Date.now() < deadline) {
                            const snapshot = await api(`/api/local/runs/${encodeURIComponent(runId)}/status`);
                            const status = snapshot?.run?.status;
                            if (status === "success" || status === "error" || status === "canceled") {
                                return snapshot;
                            }
                            await new Promise((resolve) => setTimeout(resolve, 750));
                        }
                        throw new Error("run did not reach terminal status before timeout");
                    }""",
                    {"serveUrl": serve_url, "runId": run_id, "timeoutMs": args.success_timeout_ms},
                )
                if status.get("run", {}).get("status") != "success":
                    raise RuntimeError(f"run ended as {status.get('run', {}).get('status')}: {status.get('run', {}).get('error') or ''}")

                page.get_by_text("success").first.wait_for(timeout=15000)
                page.get_by_role("button", name="预览").first.click()
                page.locator(".ant-modal").first.wait_for(timeout=15000)

                evidence = page.evaluate(
                    """async ({ serveUrl, runId, launchSecret }) => {
                        const api = async (path) => {
                            const response = await fetch(`${serveUrl}${path}`, { credentials: "include" });
                            const payload = await response.json();
                            if (!response.ok || payload.code !== 0) {
                                throw new Error(payload.msg || `local api failed: ${path}`);
                            }
                            return payload.data;
                        };
                        const events = await api(`/api/local/runs/${encodeURIComponent(runId)}/events`);
                        const artifacts = await api(`/api/local/runs/${encodeURIComponent(runId)}/artifacts`);
                        const snapshot = await api(`/api/local/runs/${encodeURIComponent(runId)}/status`);
                        const storage = JSON.stringify(window.localStorage);
                        return {
                            status: snapshot.run.status,
                            nodeStatuses: (snapshot.nodes || []).map((node) => ({ nodeId: node.nodeId, status: node.status })),
                            eventTypes: (events.events || []).map((event) => event.type),
                            artifactCount: (artifacts.artifacts || []).length,
                            artifactTypes: (artifacts.artifacts || []).map((item) => item.artifact?.type).filter(Boolean),
                            localStorageContainsLaunchSecret: storage.includes(launchSecret),
                            localStorageText: storage,
                        };
                    }""",
                    {"serveUrl": serve_url, "runId": run_id, "launchSecret": args.launch_secret},
                )
                storage_text = evidence.pop("localStorageText", "")
                forbidden_values = [
                    args.launch_secret,
                    "Bearer ",
                    "bearer.token",
                    "launch.secret",
                    "tokenFile",
                    "launchSecretFile",
                    "OPSC_HYBRID",
                    "OPSC_BROWSER",
                    "cookie",
                ]
                if args.forbidden_secret_file:
                    secret = Path(args.forbidden_secret_file).read_text(encoding="utf-8").strip()
                    if secret:
                        forbidden_values.append(secret)
                for forbidden in forbidden_values:
                    if forbidden and forbidden in storage_text:
                        raise RuntimeError("browser localStorage contains credential material")
                if evidence["artifactCount"] <= 0:
                    raise RuntimeError("run produced no local artifacts")
                if "remote.run.dispatched" in evidence["eventTypes"] or "hybrid.remote_run.started" in evidence["eventTypes"]:
                    raise RuntimeError("local-first run emitted hybrid/remote dispatch events")

                project_outputs = check_project_outputs(args.project_root, run_id) if args.project_root else {}
                payload = {
                    "ok": True,
                    "runId": run_id,
                    "template": setup,
                    "persistentProfile": bool(args.user_data_dir),
                    "status": evidence["status"],
                    "nodeStatuses": evidence["nodeStatuses"],
                    "eventTypes": sorted(set(evidence["eventTypes"])),
                    "artifactCount": evidence["artifactCount"],
                    "artifactTypes": sorted(set(evidence["artifactTypes"])),
                    "projectOutputs": project_outputs,
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
    return 0


def check_project_outputs(project_root: str, run_id: str) -> dict[str, Any]:
    root = Path(project_root)
    base = root / "outputs" / "ecommerce" / run_id
    if not base.exists():
        raise RuntimeError("project output root for run was not created")
    relative_files = sorted(str(path.relative_to(root)) for path in base.rglob("*") if path.is_file())
    required_suffixes = ["package.json", "sync-local.json"]
    for suffix in required_suffixes:
        if not any(item.endswith(suffix) for item in relative_files):
            raise RuntimeError(f"project output missing {suffix}")
    if not any(item.endswith(".png") for item in relative_files):
        raise RuntimeError("project output missing generated png files")
    return {"checked": True, "fileCount": len(relative_files), "sampleFiles": relative_files[:10]}


def write_evidence(path: str, payload: dict[str, Any]) -> None:
    if not path:
        return
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    raise SystemExit(main())

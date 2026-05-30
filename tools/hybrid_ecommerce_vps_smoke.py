#!/usr/bin/env python3
"""Run a narrow Phase 11 hybrid ecommerce VPS smoke.

This script only orchestrates existing `opsc` commands. It does not read
workspace files directly, does not call the VPS API itself, and does not print
secret values. The local workspace remains the canonical source.
"""

from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit, urlunsplit


def main() -> int:
    parser = argparse.ArgumentParser(description="Run Phase 11 hybrid ecommerce VPS smoke through opsc.")
    parser.add_argument("--workspace", required=True, help="Local workspace path.")
    parser.add_argument("--remote-url", default=os.environ.get("OPSC_HYBRID_VPS_URL") or os.environ.get("OPSC_VPS_URL"))
    parser.add_argument(
        "--remote-template",
        default=os.environ.get("OPSC_HYBRID_REMOTE_TEMPLATE_ID") or os.environ.get("OPSC_VPS_REMOTE_TEMPLATE_ID"),
    )
    parser.add_argument("--profile", default=os.environ.get("OPSC_HYBRID_PROFILE_ID", "default"))
    parser.add_argument("--channel", default=os.environ.get("OPSC_HYBRID_CHANNEL_ID", "vps"))
    parser.add_argument("--secret-env", default=os.environ.get("OPSC_HYBRID_SECRET_ENV", "OPSC_HYBRID_VPS_TOKEN"))
    parser.add_argument("--input-file", required=True, help="JSON object or bare inputs array for the local run.")
    parser.add_argument("--opsc", default=os.environ.get("OPSC_BIN", "go run ./cmd/opsc"))
    parser.add_argument("--timeout", type=int, default=1800)
    parser.add_argument("--evidence", help="Optional redacted evidence JSON output path.")
    args = parser.parse_args()

    evidence: dict[str, Any] = {
        "workspace": "<redacted>",
        "remoteUrl": safe_url(args.remote_url or ""),
        "remoteTemplateId": args.remote_template or "",
        "profile": args.profile,
        "channel": args.channel,
        "secretEnv": args.secret_env,
        "steps": [],
    }

    missing = []
    for name, value in {
        "--remote-url or OPSC_HYBRID_VPS_URL/OPSC_VPS_URL": args.remote_url,
        "--remote-template or OPSC_HYBRID_REMOTE_TEMPLATE_ID/OPSC_VPS_REMOTE_TEMPLATE_ID": args.remote_template,
        "--input-file": args.input_file,
    }.items():
        if not str(value or "").strip():
            missing.append(name)
    if args.secret_env and not os.environ.get(args.secret_env):
        missing.append(f"env {args.secret_env}")
    if args.input_file and not Path(args.input_file).is_file():
        missing.append(f"input file {args.input_file}")
    if missing:
        evidence["ok"] = False
        evidence["missing"] = missing
        write_evidence(args.evidence, evidence)
        for item in missing:
            print(f"missing required smoke prerequisite: {item}", file=sys.stderr)
        return 2

    try:
        import_result = run_json(
            args.opsc,
            [
                "ecommerce",
                "import-template",
                "--workspace",
                args.workspace,
                "--remote-url",
                args.remote_url,
                "--remote-template",
                args.remote_template,
                "--profile",
                args.profile,
                "--channel",
                args.channel,
                "--secret-env",
                args.secret_env,
                "--json",
            ],
            args.timeout,
            "import-template",
            evidence,
        )
        template = import_result["data"]["template"]
        template_id = template["id"]

        run_result = run_json(
            args.opsc,
            [
                "ecommerce",
                "create-run",
                template_id,
                "--workspace",
                args.workspace,
                "--input-file",
                args.input_file,
                "--profile",
                args.profile,
                "--channel",
                args.channel,
                "--json",
            ],
            args.timeout,
            "create-run",
            evidence,
        )
        run_id = run_result["data"]["run"]["id"]

        executor_result = run_json(
            args.opsc,
            ["executor", "--workspace", args.workspace, "--run", run_id, "--json"],
            args.timeout,
            "executor",
            evidence,
        )
        status_result = run_json(
            args.opsc,
            ["run", "status", run_id, "--workspace", args.workspace, "--json"],
            args.timeout,
            "run-status",
            evidence,
        )
        artifacts_result = run_json(
            args.opsc,
            ["artifact", "list", "--run", run_id, "--workspace", args.workspace, "--json"],
            args.timeout,
            "artifact-list",
            evidence,
        )
    except SmokeError as exc:
        evidence["ok"] = False
        evidence["error"] = exc.message
        write_evidence(args.evidence, evidence)
        print(exc.message, file=sys.stderr)
        return exc.exit_code

    run_status = status_result["data"]["run"]["status"]
    artifact_count = len(artifacts_result["data"])
    evidence["ok"] = run_status == "success"
    evidence["templateId"] = template_id
    evidence["runId"] = run_id
    evidence["runStatus"] = run_status
    evidence["artifactCount"] = artifact_count
    evidence["executorProcessed"] = executor_result["data"].get("processed")
    write_evidence(args.evidence, evidence)

    print(json.dumps({k: v for k, v in evidence.items() if k != "steps"}, ensure_ascii=False, indent=2))
    return 0 if evidence["ok"] else 1


class SmokeError(Exception):
    def __init__(self, message: str, exit_code: int = 1) -> None:
        super().__init__(message)
        self.message = message
        self.exit_code = exit_code


def run_json(opsc: str, args: list[str], timeout: int, step: str, evidence: dict[str, Any]) -> dict[str, Any]:
    repo_root = Path(__file__).resolve().parents[1]
    command = [*shlex.split(opsc), *args]
    completed = subprocess.run(command, cwd=repo_root, text=True, capture_output=True, timeout=timeout, check=False)
    evidence["steps"].append(
        {
            "name": step,
            "exitCode": completed.returncode,
            "stderr": redact_text(completed.stderr),
        }
    )
    if completed.returncode != 0:
        raise SmokeError(f"{step} failed with exit code {completed.returncode}", completed.returncode)
    try:
        payload = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise SmokeError(f"{step} did not return JSON: {exc}") from exc
    if not payload.get("ok"):
        raise SmokeError(f"{step} returned ok=false")
    return payload


def redact_text(value: str) -> str:
    if not value:
        return ""
    redacted = value
    for key in ("OPSC_HYBRID_VPS_TOKEN", "OPSC_VPS_ADMIN_TOKEN", "PDD_ADMIN_TOKEN"):
        secret = os.environ.get(key)
        if secret:
            redacted = redacted.replace(secret, "<redacted>")
    home = str(Path.home())
    redacted = redacted.replace(home, "~")
    return redacted.strip()


def safe_url(value: str) -> str:
    parsed = urlsplit(value)
    host = parsed.hostname or ""
    if parsed.port:
        host = f"{host}:{parsed.port}"
    return urlunsplit((parsed.scheme, host, parsed.path, "", ""))


def write_evidence(path: str | None, evidence: dict[str, Any]) -> None:
    if not path:
        return
    output_path = Path(path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    raise SystemExit(main())

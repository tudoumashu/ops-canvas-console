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
    parser.add_argument("--profile", default=os.environ.get("OPSC_HYBRID_PROFILE_ID", ""))
    parser.add_argument("--channel", default=os.environ.get("OPSC_HYBRID_CHANNEL_ID", ""))
    parser.add_argument("--secret-env", default=default_secret_env())
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
        "credentialSource": credential_source(args),
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
    if not args.secret_env and not args.profile and not args.channel:
        missing.append("--secret-env/OPSC_HYBRID_SECRET_ENV or OPSC_HYBRID_PROFILE_ID/OPSC_HYBRID_CHANNEL_ID")
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
            with_optional_pairs(
                [
                    "ecommerce",
                    "import-template",
                    "--workspace",
                    args.workspace,
                    "--remote-url",
                    args.remote_url,
                    "--remote-template",
                    args.remote_template,
                    "--json",
                ],
                {"--profile": args.profile, "--channel": args.channel, "--secret-env": args.secret_env},
            ),
            args.timeout,
            "import-template",
            evidence,
            args.secret_env,
        )
        template = import_result["data"]["template"]
        template_id = template["id"]

        run_result = run_json(
            args.opsc,
            with_optional_pairs(
                [
                    "ecommerce",
                    "create-run",
                    template_id,
                    "--workspace",
                    args.workspace,
                    "--input-file",
                    args.input_file,
                    "--json",
                ],
                {"--profile": args.profile, "--channel": args.channel},
            ),
            args.timeout,
            "create-run",
            evidence,
            args.secret_env,
        )
        run_id = run_result["data"]["run"]["id"]

        executor_result = run_json(
            args.opsc,
            ["executor", "--workspace", args.workspace, "--run", run_id, "--json"],
            args.timeout,
            "executor",
            evidence,
            args.secret_env,
        )
        status_result = run_json(
            args.opsc,
            ["run", "status", run_id, "--workspace", args.workspace, "--json"],
            args.timeout,
            "run-status",
            evidence,
            args.secret_env,
        )
        artifacts_result = run_json(
            args.opsc,
            ["artifact", "list", "--run", run_id, "--workspace", args.workspace, "--json"],
            args.timeout,
            "artifact-list",
            evidence,
            args.secret_env,
        )
    except SmokeError as exc:
        evidence["ok"] = False
        evidence["error"] = exc.message
        write_evidence(args.evidence, evidence)
        print(exc.message, file=sys.stderr)
        return exc.exit_code

    run_status = status_result["data"]["run"]["status"]
    artifact_count = len(artifacts_result["data"].get("artifacts", []))
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


def default_secret_env() -> str:
    configured = os.environ.get("OPSC_HYBRID_SECRET_ENV", "").strip()
    if configured:
        return configured
    if os.environ.get("OPSC_HYBRID_VPS_TOKEN"):
        return "OPSC_HYBRID_VPS_TOKEN"
    return ""


def credential_source(args: argparse.Namespace) -> str:
    if args.profile or args.channel:
        return "profileChannel"
    if args.secret_env:
        return "envSecretRef"
    return "missing"


def with_optional_pairs(args: list[str], pairs: dict[str, str | None]) -> list[str]:
    result = list(args)
    for key, value in pairs.items():
        value = str(value or "").strip()
        if value:
            result.extend([key, value])
    return result


def run_json(
    opsc: str,
    args: list[str],
    timeout: int,
    step: str,
    evidence: dict[str, Any],
    secret_env: str | None,
) -> dict[str, Any]:
    repo_root = Path(__file__).resolve().parents[1]
    command = [*shlex.split(opsc), *args]
    try:
        completed = subprocess.run(command, cwd=repo_root, text=True, capture_output=True, timeout=timeout, check=False)
    except subprocess.TimeoutExpired as exc:
        evidence["steps"].append(
            {
                "name": step,
                "exitCode": "timeout",
                "stderr": redact_text(exc.stderr or "", secret_env),
            }
        )
        raise SmokeError(f"{step} timed out after {timeout}s") from exc
    evidence["steps"].append(
        {
            "name": step,
            "exitCode": completed.returncode,
            "stderr": redact_text(completed.stderr, secret_env),
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


def redact_text(value: str, secret_env: str | None) -> str:
    if not value:
        return ""
    redacted = value
    secret_keys = {"OPSC_HYBRID_VPS_TOKEN", "OPSC_VPS_ADMIN_TOKEN", "PDD_ADMIN_TOKEN"}
    if secret_env:
        secret_keys.add(secret_env)
    for key in secret_keys:
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

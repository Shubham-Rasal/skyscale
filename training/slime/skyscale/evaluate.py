"""Run a frozen coding suite against SGLang and report promotion metrics."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import urllib.request


def request(url: str, payload: dict | None = None) -> dict:
    headers: dict[str, str] = {}
    body = None
    if payload is not None:
        body = json.dumps(payload).encode()
        headers["Content-Type"] = "application/json"
    if token := os.environ.get("SKYSCALE_RUNTIME_TOKEN"):
        headers["Authorization"] = f"Bearer {token}"
    with urllib.request.urlopen(
        urllib.request.Request(url, data=body, headers=headers), timeout=300
    ) as response:
        return json.loads(response.read())


def load_suite(uri: str, expected_hash: str) -> list[dict]:
    if uri.startswith(("http://", "https://")):
        with urllib.request.urlopen(uri, timeout=60) as response:
            raw = response.read()
    elif uri.startswith("file://"):
        raw = Path(uri.removeprefix("file://")).read_bytes()
    else:
        raise ValueError("suite_uri must be a signed HTTP URL or file:// URI")
    actual = hashlib.sha256(raw).hexdigest()
    if expected_hash and actual != expected_hash:
        raise ValueError(f"frozen suite hash mismatch: {actual}")
    suite = json.loads(raw)
    if not isinstance(suite, list) or not suite:
        raise ValueError("evaluation suite must be a non-empty JSON list")
    return suite


def generated_text(response: dict) -> str:
    value = response.get("text", response.get("generated_text", ""))
    if isinstance(value, list):
        value = value[0] if value else ""
    if not isinstance(value, str) or not value:
        raise ValueError("SGLang response did not contain generated text")
    return value


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--policy-version", required=True)
    parser.add_argument("--suite-uri", required=True)
    parser.add_argument("--suite-hash", default="")
    parser.add_argument("--engine-url", required=True)
    parser.add_argument("--control-plane-url", required=True)
    parser.add_argument("--phase", choices=("evaluation", "canary"), default="evaluation")
    args = parser.parse_args()
    suite = load_suite(args.suite_uri, args.suite_hash)
    rewards: list[float] = []
    pass_rates: list[float] = []
    for case in suite:
        generation = request(
            args.engine_url.rstrip("/") + "/generate",
            {
                "text": case["prompt"],
                "sampling_params": case.get(
                    "sampling_params", {"temperature": 0, "max_new_tokens": 256}
                ),
            },
        )
        result = request(
            args.control_plane_url.rstrip("/") + "/api/rl/env/evaluate",
            {
                "run_id": args.run_id,
                "task_id": case["task_id"],
                "code": generated_text(generation),
            },
        )
        rewards.append(float(result["reward"]))
        total = max(1, int(result["total_tests"]))
        pass_rates.append(int(result["passed_tests"]) / total)
    metrics = {
        "reward": sum(rewards) / len(rewards),
        "pass_rate": sum(pass_rates) / len(pass_rates),
    }
    request(
        args.control_plane_url.rstrip("/")
        + f"/api/rl/v1/runs/{args.run_id}/weights/{args.policy_version}/evaluate",
        {"phase": args.phase, "metrics": metrics},
    )
    print(json.dumps(metrics, sort_keys=True))


if __name__ == "__main__":
    main()

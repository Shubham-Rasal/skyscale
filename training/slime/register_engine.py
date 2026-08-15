#!/usr/bin/env python3
"""Register an independently managed SGLang engine after it is ready."""

from __future__ import annotations

import json
import hashlib
import os
from pathlib import Path
import tempfile
import time
import urllib.request


def request_json(url: str, payload: dict | None = None) -> dict:
    headers = {}
    data = None
    if payload is not None:
        data = json.dumps(payload).encode()
        headers["Content-Type"] = "application/json"
    if token := os.environ.get("SKYSCALE_RUNTIME_TOKEN"):
        headers["Authorization"] = f"Bearer {token}"
    with urllib.request.urlopen(urllib.request.Request(url, data=data, headers=headers), timeout=10) as response:
        raw = response.read()
        return json.loads(raw) if raw else {}


def download_verified(url: str, expected_sha256: str, destination: Path) -> None:
    headers = {}
    if token := os.environ.get("SKYSCALE_RUNTIME_TOKEN"):
        headers["Authorization"] = f"Bearer {token}"
    with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=300) as response:
        payload = response.read()
    actual = hashlib.sha256(payload).hexdigest()
    if actual != expected_sha256:
        raise RuntimeError(f"downloaded weight checksum mismatch: {actual}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=destination.parent, delete=False) as handle:
        handle.write(payload)
        temporary = Path(handle.name)
    temporary.replace(destination)


def main() -> None:
    control_plane = os.environ["SKYSCALE_CONTROL_PLANE_URL"].rstrip("/")
    run_id = os.environ["SKYSCALE_RUN_ID"]
    engine_id = os.environ["SKYSCALE_ENGINE_ID"]
    current_version = os.environ.get("SKYSCALE_POLICY_VERSION", "")
    while True:
        try:
            request_json("http://127.0.0.1:8000/server_info")
            instruction = request_json(
                control_plane + "/api/rl/v1/rollout-engines/register",
                {
                    "run_id": run_id,
                    "engine_id": engine_id,
                    "current_version": current_version,
                },
            )
            desired = instruction.get("desired_version", "")
            if desired and desired != current_version:
                destination = Path("/var/lib/skyscale/weights") / run_id / desired / "model.safetensors"
                download_verified(
                    f"{control_plane}/api/rl/v1/weights/{desired}/artifact?run_id={run_id}",
                    instruction["sha256"],
                    destination,
                )
                request_json(
                    "http://127.0.0.1:8000" + os.environ.get("SGLANG_UPDATE_PATH", "/update_weights_from_disk"),
                    {"model_path": str(destination)},
                )
                request_json(
                    f"{control_plane}/api/rl/v1/weights/{desired}/ack",
                    {"run_id": run_id, "engine_id": engine_id, "checksum": instruction["sha256"]},
                )
                current_version = desired
            time.sleep(15)
        except Exception as error:
            print(f"engine registration retry: {error}", flush=True)
            time.sleep(5)


if __name__ == "__main__":
    main()

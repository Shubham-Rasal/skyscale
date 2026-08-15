#!/usr/bin/env python3
"""Report slime checkpoint progress to the SkyScale control plane."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import time

from skyscale.adapters import SkyScaleClient


def checkpoint_manifest(checkpoint_root: Path) -> tuple[dict[str, object], str]:
    files: list[dict[str, object]] = []
    manifest_path = checkpoint_root / "skyscale-checkpoint-manifest.json"
    for path in sorted(p for p in checkpoint_root.rglob("*") if p.is_file() and p != manifest_path):
        digest = hashlib.sha256()
        with path.open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
        files.append(
            {
                "path": str(path.relative_to(checkpoint_root)),
                "size": path.stat().st_size,
                "sha256": digest.hexdigest(),
            }
        )
    manifest: dict[str, object] = {"version": 1, "files": files}
    encoded = json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode()
    return manifest, hashlib.sha256(encoded).hexdigest()


def latest_step(checkpoint_root: Path) -> int | None:
    marker = checkpoint_root / "latest_checkpointed_iteration.txt"
    if not marker.is_file():
        return None
    value = marker.read_text().strip()
    return int(value) + 1


def report_checkpoint(
    client: SkyScaleClient,
    attempt_id: str,
    checkpoint_root: Path,
    optimizer_step: int,
) -> None:
    manifest, manifest_sha256 = checkpoint_manifest(checkpoint_root)
    (checkpoint_root / "skyscale-checkpoint-manifest.json").write_text(
        json.dumps(manifest, sort_keys=True, separators=(",", ":"))
    )
    client.post(
        f"/api/rl/v1/runs/{client.run_id}/trainer-progress",
        {"attempt_id": attempt_id, "optimizer_step": optimizer_step},
    )
    client.post(
        f"/api/rl/v1/runs/{client.run_id}/checkpoints",
        {
            "attempt_id": attempt_id,
            "optimizer_step": optimizer_step,
            "resume_uri": str(checkpoint_root),
            "serving_uri": "",
            "policy_version": f"{client.run_id}-step-{optimizer_step}",
            "manifest_sha256": manifest_sha256,
        },
    )


def main() -> None:
    client = SkyScaleClient.from_env()
    attempt_id = os.environ["SKYSCALE_ATTEMPT_ID"]
    checkpoint_root = Path(os.environ.get("SKYSCALE_CHECKPOINT_DIR", "/models/resume"))
    interval = float(os.environ.get("SKYSCALE_REPORT_INTERVAL_SECONDS", "10"))
    reported_step = -1
    while True:
        try:
            step = latest_step(checkpoint_root)
            if step is not None and step > reported_step:
                report_checkpoint(client, attempt_id, checkpoint_root, step)
                reported_step = step
                print(f"reported checkpoint at optimizer step {step}", flush=True)
        except Exception as error:
            print(f"checkpoint reporter retry: {error}", flush=True)
        time.sleep(interval)


if __name__ == "__main__":
    main()

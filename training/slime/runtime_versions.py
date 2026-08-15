#!/usr/bin/env python3
"""Fail closed unless every numerical runtime dependency matches its pin."""

from __future__ import annotations

import importlib.metadata
import json
import os
import subprocess
from pathlib import Path


def git_revision(path: str) -> str:
    return subprocess.check_output(
        ["git", "-C", path, "rev-parse", "HEAD"], text=True
    ).strip()


def distribution_version(*names: str) -> str:
    for name in names:
        try:
            return importlib.metadata.version(name)
        except importlib.metadata.PackageNotFoundError:
            continue
    raise RuntimeError(f"none of the distributions are installed: {names}")


def validate(environ: dict[str, str] | None = None) -> dict[str, str]:
    env = environ or os.environ
    expected = {
        "slime_commit": env["SLIME_COMMIT"],
        "megatron_commit": env["MEGATRON_COMMIT"],
        "sglang_version": env["SGLANG_VERSION"],
        "transformer_engine_version": env["TRANSFORMER_ENGINE_VERSION"],
        "cuda_version": env["CUDA_VERSION"],
    }
    import torch

    actual = {
        "slime_commit": git_revision("/root/slime"),
        "megatron_commit": git_revision("/root/Megatron-LM"),
        "sglang_version": distribution_version("sglang"),
        "transformer_engine_version": distribution_version(
            "transformer-engine", "transformer_engine"
        ),
        "cuda_version": torch.version.cuda or "",
    }
    mismatches = {
        key: {"expected": expected[key], "actual": value}
        for key, value in actual.items()
        if (not value.startswith(expected[key]) if key == "cuda_version" else value != expected[key])
    }
    if mismatches:
        raise RuntimeError(f"runtime pin mismatch: {json.dumps(mismatches, sort_keys=True)}")
    manifest = {**actual, "base_image_digest": "sha256:a97ec147e37bef050337a9b229036eda00b4aa9c4d02b31a0109dc850f8ca342"}
    output = Path("/opt/skyscale/slime/runtime-versions.json")
    output.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    print(json.dumps(manifest, sort_keys=True))
    return manifest


if __name__ == "__main__":
    validate()

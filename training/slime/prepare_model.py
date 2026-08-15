#!/usr/bin/env python3
"""Idempotently prepare an HF model and Megatron torch_dist artifact."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path


QWEN3_06B_ARGS = [
    "--swiglu",
    "--num-layers", "28",
    "--hidden-size", "1024",
    "--ffn-hidden-size", "3072",
    "--num-attention-heads", "16",
    "--group-query-attention",
    "--num-query-groups", "8",
    "--use-rotary-position-embeddings",
    "--disable-bias-linear",
    "--normalization", "RMSNorm",
    "--norm-epsilon", "1e-6",
    "--rotary-base", "1000000",
    # HF embedding rows are padded to 151936; Megatron defaults to tokenizer size (~151680).
    "--vocab-size", "151936",
    "--kv-channels", "128",
    "--qk-layernorm",
    "--seq-length", "4096",
    "--max-position-embeddings", "40960",
    "--untie-embeddings-and-output-weights",
]


def tree_hash(path: Path) -> str:
    digest = hashlib.sha256()
    for item in sorted(p for p in path.rglob("*") if p.is_file()):
        digest.update(str(item.relative_to(path)).encode())
        with item.open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", default="Qwen/Qwen3-0.6B")
    parser.add_argument("--revision", required=True)
    parser.add_argument("--output-root", default="/models")
    args = parser.parse_args()

    root = Path(args.output_root)
    hf_dir = root / "hf"
    megatron_dir = root / "torch_dist"
    manifest_path = root / "model-artifact.json"
    root.mkdir(parents=True, exist_ok=True)

    expected = {"model": args.model, "revision": args.revision, "format": "torch_dist"}
    if manifest_path.exists():
        manifest = json.loads(manifest_path.read_text())
        if all(manifest.get(key) == value for key, value in expected.items()) and megatron_dir.exists():
            print(json.dumps(manifest, sort_keys=True))
            return 0

    from huggingface_hub import snapshot_download

    snapshot_download(
        repo_id=args.model,
        revision=args.revision,
        local_dir=hf_dir,
        token=os.environ.get("HF_TOKEN"),
    )
    command = [
        sys.executable,
        "/root/slime/tools/convert_hf_to_torch_dist.py",
        *QWEN3_06B_ARGS,
        "--hf-checkpoint", str(hf_dir),
        "--save", str(megatron_dir),
    ]
    env = dict(os.environ)
    env["PYTHONPATH"] = "/root/Megatron-LM:" + env.get("PYTHONPATH", "")
    subprocess.run(command, env=env, check=True)

    manifest = {
        **expected,
        "hf_path": str(hf_dir),
        "torch_dist_path": str(megatron_dir),
        "hf_sha256": tree_hash(hf_dir),
        "torch_dist_sha256": tree_hash(megatron_dir),
        "slime_commit": "aaf5c2092b01219fa0d5c2d323741d409086ca32",
        "megatron_commit": "1dcf0dafa884ad52ffb243625717a3471643e087",
    }
    temporary = manifest_path.with_suffix(".tmp")
    temporary.write_text(json.dumps(manifest, indent=2, sort_keys=True))
    temporary.replace(manifest_path)
    print(json.dumps(manifest, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Submit the MNIST trainer through the Hugging Face GPU provider.

Run the control plane with:

    GPU_PROVIDER_ORDER=huggingface,akash \
    HF_TOKEN=hf_... \
    HF_NAMESPACE=<user-or-org> \
    SKYSCALE_PUBLIC_BASE=https://your-control-plane.example.com \
    ./skyscale-cp

Then submit:

    python examples/huggingface_gpu_mnist.py

Use --dry-run to print the payload without submitting.
"""

import argparse
import json
import os
import time
import urllib.error
import urllib.request
import urllib.parse
from typing import Optional


def is_local_url(value: Optional[str]) -> bool:
    if not value:
        return True
    host = (urllib.parse.urlparse(value).hostname or "").lower()
    if host in {"", "localhost", "0.0.0.0", "::1"} or host.startswith("127."):
        return True
    if host.startswith("10.") or host.startswith("192.168."):
        return True
    parts = host.split(".")
    return len(parts) == 4 and parts[0] == "172" and parts[1].isdigit() and 16 <= int(parts[1]) <= 31


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--api-url", default=os.getenv("SKYSCALE_API_URL", "http://localhost:8080"))
    parser.add_argument("--public-base", default=os.getenv("SKYSCALE_PUBLIC_BASE"))
    parser.add_argument("--job-id", default=f"hf-mnist-{int(time.time())}")
    parser.add_argument("--image", default=os.getenv("MNIST_IMAGE", "ghcr.io/shubham-rasal/skyscale-mnist:v1"))
    parser.add_argument("--gpu-model", default=os.getenv("GPU_MODEL", "a10g"))
    parser.add_argument("--epochs", default=os.getenv("EPOCHS", "1"))
    parser.add_argument("--batch-size", default=os.getenv("BATCH_SIZE", "512"))
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    public_base = args.public_base or os.getenv("HF_CALLBACK_BASE")
    if not args.dry_run and is_local_url(public_base):
        raise SystemExit(
            "Hugging Face jobs need a public callback URL. "
            "Set SKYSCALE_PUBLIC_BASE or pass --public-base https://your-control-plane.example.com"
        )

    payload = {
        "job_id": args.job_id,
        "provider": "huggingface",
        "docker_image": args.image,
        "gpu_model": args.gpu_model,
        "env_vars": {
            "SKYSCALE_JOB_ID": args.job_id,
            "EPOCHS": str(args.epochs),
            "BATCH_SIZE": str(args.batch_size),
            "LR": os.getenv("LR", "1e-3"),
        },
    }
    if public_base:
        payload["control_plane_url"] = public_base

    print(json.dumps(payload, indent=2))
    if args.dry_run:
        return

    req = urllib.request.Request(
        args.api_url.rstrip("/") + "/api/training/jobs",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            print(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        print(exc.read().decode("utf-8"))
        raise


if __name__ == "__main__":
    main()

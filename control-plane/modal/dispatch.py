#!/usr/bin/env python3
"""
Modal dispatcher sidecar.

Called by the Go control plane to submit a GPU job to Modal.

Usage:
    python3 dispatch.py '{"job_id":"...", "execution_id":"...", "docker_image":"...",
                          "gpu_model":"a100", "env_vars":{...}, "timeout":86400}'

Prints a JSON result to stdout:
    {"sandbox_id": "sb_xxx", "status": "started"}

The sandbox runs the docker_image as its base, injects env_vars, and runs the
container's default CMD. When the container exits it calls back to
CONTROL_PLANE_URL/api/executions/{EXECUTION_ID}/complete.

Requires MODAL_TOKEN_ID and MODAL_TOKEN_SECRET in environment (or ~/.modal.toml).
"""

import json
import os
import sys

import modal

# GPU model → Modal GPU string mapping
GPU_MAP = {
    "a100":    "a100",
    "a100-40": "a100-40gb",
    "a100-80": "a100-80gb",
    "h100":    "h100",
    "a10g":    "a10g",
    "t4":      "t4",
    "l4":      "l4",
    "any":     "any",
    "":        "a100",
}


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "usage: dispatch.py <json-params>"}))
        sys.exit(1)

    try:
        params = json.loads(sys.argv[1])
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"invalid JSON: {e}"}))
        sys.exit(1)

    job_id       = params.get("job_id", "job")
    execution_id = params.get("execution_id", job_id)
    docker_image = params.get("docker_image", "")
    gpu_model    = params.get("gpu_model", "a100")
    env_vars     = params.get("env_vars", {})
    timeout      = int(params.get("timeout", 86400))  # default 24h

    if not docker_image:
        print(json.dumps({"error": "docker_image is required"}))
        sys.exit(1)

    modal_gpu = GPU_MAP.get(gpu_model.lower(), gpu_model)
    cmd        = params.get("cmd", [])  # explicit command; if empty, image entrypoint runs

    image = modal.Image.from_registry(docker_image)
    app   = modal.App(f"skyscale-{job_id}")

    # Sandbox.create(*args) sets the command. If no cmd given, run a no-op shell
    # that lets the image's own ENTRYPOINT+CMD drive via env vars (most training
    # containers use entrypoint scripts that read env vars).
    if cmd:
        sb = modal.Sandbox.create(
            *cmd,
            app=app,
            image=image,
            gpu=modal_gpu,
            timeout=timeout,
            env=env_vars,
        )
    else:
        # No explicit cmd — launch via the image entrypoint by running
        # the standard training script path from env or fallback.
        entrypoint = env_vars.get("ENTRYPOINT", "/app/entrypoint.sh")
        sb = modal.Sandbox.create(
            "sh", "-c", f"exec {entrypoint}",
            app=app,
            image=image,
            gpu=modal_gpu,
            timeout=timeout,
            env=env_vars,
        )

    result = {
        "sandbox_id": sb.object_id,
        "status":     "started",
        "job_id":     job_id,
        "execution_id": execution_id,
        "gpu":        modal_gpu,
    }
    print(json.dumps(result))


if __name__ == "__main__":
    main()

"""
Runtime logic for `skyscale run` — discovers @method functions, submits to the control plane.
"""

import importlib.util
import json
import os
import sys
import time

import requests


def _load_module(path: str):
    spec = importlib.util.spec_from_file_location("_skyscale_user_module", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def run(
    file: str,
    *,
    function: str | None = None,
    gpu: str = "a100",
    image: str,
    env: dict | None = None,
    control_plane_url: str | None = None,
    epochs: int | None = None,
    batch_size: int | None = None,
    poll_interval: int = 5,
):
    """Submit a training job and stream progress until completion."""
    base_url = control_plane_url or os.environ.get("SKYSCALE_API_URL", "http://localhost:8080")

    # Build env vars from explicit kwargs and passthrough.
    env_vars: dict[str, str] = {}
    if env:
        env_vars.update({k: str(v) for k, v in env.items()})
    if epochs is not None:
        env_vars["EPOCHS"] = str(epochs)
    if batch_size is not None:
        env_vars["BATCH_SIZE"] = str(batch_size)

    job_id = f"cli-{os.path.splitext(os.path.basename(file))[0]}-{int(time.time())}"

    payload = {
        "job_id": job_id,
        "docker_image": image,
        "gpu_model": gpu,
        "env_vars": env_vars,
    }

    print(f"[skyscale] submitting job {job_id} (gpu={gpu}, image={image})")
    resp = requests.post(f"{base_url}/api/training/jobs", json=payload, timeout=30)
    resp.raise_for_status()
    data = resp.json()
    execution_id = data["execution_id"]
    print(f"[skyscale] execution_id={execution_id}  status={data.get('status', '?')}")

    # Poll until terminal state.
    terminal = {"completed", "failed", "error"}
    last_step = -1
    while True:
        time.sleep(poll_interval)
        try:
            r = requests.get(f"{base_url}/api/executions/{execution_id}", timeout=10)
            r.raise_for_status()
            exec_data = r.json()
        except Exception as e:
            print(f"[skyscale] poll error: {e}")
            continue

        status = exec_data.get("Status", "")

        # Fetch and print new metrics.
        try:
            mr = requests.get(f"{base_url}/api/training/metrics/{job_id}", timeout=10)
            if mr.ok:
                metrics = mr.json() or []
                for m in metrics:
                    step = m.get("step", 0)
                    if step > last_step:
                        last_step = step
                        loss = m.get("loss", 0)
                        reward = m.get("episode_reward", 0)
                        gpu_util = m.get("gpu_util", 0)
                        print(
                            f"[skyscale] step={step:>6}  loss={loss:.4f}  "
                            f"reward={reward:.2f}  gpu={gpu_util}%"
                        )
        except Exception:
            pass

        if status in terminal:
            print(f"[skyscale] job finished: {status}")
            if exec_data.get("Error"):
                print(f"[skyscale] error: {exec_data['Error']}")
            break

    return execution_id

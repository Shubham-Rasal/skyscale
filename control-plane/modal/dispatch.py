#!/usr/bin/env python3
"""
Modal dispatcher sidecar.

Called by the Go control plane to submit a GPU/CPU job to Modal.
"""

import json
import sys
import time
from pathlib import Path

import modal
import requests

REPO_ROOT = Path(__file__).resolve().parent.parent.parent

GPU_MAP = {
    "a100": "a100",
    "a100-40": "a100-40gb",
    "a100-80": "a100-80gb",
    "h100": "h100",
    "a10g": "a10g",
    "t4": "t4",
    "l4": "l4",
    "any": "any",
    "": "a10g",
}


def is_policy_server(params: dict) -> bool:
    image = params.get("docker_image", "")
    env = params.get("env_vars", {})
    return "policy-server" in image or env.get("SKYSCALE_ROLE") == "policy_server"


def is_cpu_job(gpu_model: str) -> bool:
    return gpu_model.lower() in ("cpu", "none", "0")


def log(msg: str) -> None:
    print(f"[dispatch] {msg}", file=sys.stderr, flush=True)


def wait_http(url: str, timeout: int = 180, interval: int = 3) -> bool:
    deadline = time.time() + timeout
    attempt = 0
    while time.time() < deadline:
        attempt += 1
        try:
            resp = requests.get(url, timeout=5)
            if resp.status_code < 500:
                log(f"health OK {url} status={resp.status_code} after {attempt} attempts")
                return True
            log(f"health wait attempt={attempt} status={resp.status_code} url={url}")
        except Exception as e:
            if attempt == 1 or attempt % 10 == 0:
                log(f"health wait attempt={attempt} url={url} err={e}")
        time.sleep(interval)
    log(f"health TIMEOUT after {timeout}s url={url}")
    return False


def wait_policy_ready(policy_url: str, timeout: int = 900, interval: int = 5, native: bool = False) -> bool:
    """Block until policy server is ready (/health for native vLLM, /ready for serve.py)."""
    deadline = time.time() + timeout
    attempt = 0
    while time.time() < deadline:
        attempt += 1
        try:
            resp = requests.get(f"{policy_url.rstrip('/')}/health", timeout=15)
            if resp.status_code == 200:
                if native:
                    log(f"policy /health ready after {attempt} attempts")
                    return True
                data = resp.json()
                if data.get("ready"):
                    log(f"policy /health ready after {attempt} attempts")
                    return True
            if not native:
                resp = requests.get(f"{policy_url.rstrip('/')}/ready", timeout=15)
                if resp.status_code == 200:
                    log(f"policy /ready after {attempt} attempts")
                    return True
        except Exception as e:
            if attempt == 1 or attempt % 12 == 0:
                log(f"policy ready wait attempt={attempt} err={e}")
        time.sleep(interval)
    log(f"policy ready TIMEOUT after {timeout}s url={policy_url}")
    return False


def verify_generate(policy_url: str, model_name: str, timeout: int = 120) -> bool:
    """Confirm vLLM chat/completions returns a real completion."""
    try:
        resp = requests.post(
            f"{policy_url.rstrip('/')}/v1/chat/completions",
            json={
                "model": model_name,
                "messages": [{"role": "user", "content": "Write: def add(a,b): return a+b"}],
                "max_tokens": 32,
                "temperature": 0.8,
            },
            timeout=timeout,
        )
        resp.raise_for_status()
        content = resp.json()["choices"][0]["message"]["content"]
        ok = bool(content and content.strip())
        if ok:
            log(f"policy chat/completions verified chars={len(content)}")
        return ok
    except Exception as e:
        log(f"policy chat/completions verify failed: {e}")
        return False


def build_image(docker_image: str):
    if "rl-worker" in docker_image:
        worker_dir = REPO_ROOT / "training" / "rl-worker"
        if worker_dir.exists():
            return (
                modal.Image.debian_slim()
                .pip_install("requests")
                .add_local_dir(str(worker_dir), "/app", copy=True)
            )
    if "rl-trainer" in docker_image:
        trainer_dir = REPO_ROOT / "training" / "rl-trainer"
        if trainer_dir.exists():
            return (
                modal.Image.debian_slim()
                .pip_install("requests", "torch", "transformers", "accelerate")
                .add_local_dir(str(trainer_dir), "/app", copy=True)
            )
    if "policy-server" in docker_image:
        policy_dir = REPO_ROOT / "training" / "policy-server"
        if policy_dir.exists():
            return (
                modal.Image.from_registry("vllm/vllm-openai:latest", add_python="3.11")
                .pip_install("fastapi", "uvicorn", "requests", "transformers", "accelerate")
                .add_local_dir(str(policy_dir), "/app", copy=True)
            )
    return modal.Image.from_registry(docker_image)


def resolve_entrypoint(docker_image: str, env_vars: dict, cmd: list) -> str:
    if cmd:
        return ""
    entrypoint = env_vars.get("ENTRYPOINT", "")
    if entrypoint:
        return entrypoint
    if "rl-worker" in docker_image:
        return "python /app/worker.py"
    if "policy-server" in docker_image:
        return "python /app/serve.py"
    if "rl-trainer" in docker_image:
        return "python /app/trainer.py"
    return "/app/entrypoint.sh"


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "usage: dispatch.py <json-params>"}))
        sys.exit(1)

    try:
        params = json.loads(sys.argv[1])
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"invalid JSON: {e}"}))
        sys.exit(1)

    job_id = params.get("job_id", "job")
    execution_id = params.get("execution_id", job_id)
    docker_image = params.get("docker_image", "")
    gpu_model = params.get("gpu_model", "a10g")
    env_vars = params.get("env_vars", {})
    timeout = int(params.get("timeout", 86400))
    control_plane_url = params.get("control_plane_url") or env_vars.get("CONTROL_PLANE_URL", "")

    if not docker_image:
        print(json.dumps({"error": "docker_image is required"}))
        sys.exit(1)

    log(f"start job={job_id} exec={execution_id} gpu={gpu_model} image={docker_image}")
    if is_policy_server(params):
        log("policy server job — building vLLM image and waiting for /health (may take several minutes)")

    cmd = params.get("cmd", [])
    entrypoint = resolve_entrypoint(docker_image, env_vars, cmd)
    log(f"entrypoint={entrypoint or cmd}")
    image = build_image(docker_image)
    app = modal.App.lookup(f"skyscale-{job_id}", create_if_missing=True)

    create_kwargs = {
        "app": app,
        "image": image,
        "timeout": timeout,
        "env": env_vars,
    }

    if is_cpu_job(gpu_model):
        create_kwargs["cpu"] = 1.0
        create_kwargs["memory"] = 512
    else:
        create_kwargs["gpu"] = GPU_MAP.get(gpu_model.lower(), gpu_model)

    if is_policy_server(params):
        create_kwargs["encrypted_ports"] = [8000]
        model = env_vars.get("MODEL_NAME", "Qwen/Qwen3-0.6B")
        checkpoint_url = env_vars.get("CHECKPOINT_URL", "")
        if checkpoint_url:
            create_kwargs["memory"] = 24576
            log(f"policy: launching serve.py with checkpoint (closed-loop redeploy)")
            sb = modal.Sandbox.create("sh", "-c", "exec python /app/serve.py", **create_kwargs)
        else:
            create_kwargs["memory"] = 16384
            policy_image = modal.Image.from_registry("vllm/vllm-openai:latest", add_python="3.11")
            create_kwargs["image"] = policy_image
            log(f"policy: launching native vLLM serve for {model}")
            sb = modal.Sandbox.create(
                model,
                "--dtype", "float16",
                "--max-model-len", "2048",
                "--gpu-memory-utilization", "0.85",
                "--port", "8000",
                "--host", "0.0.0.0",
                **create_kwargs,
            )
    elif cmd:
        sb = modal.Sandbox.create(*cmd, **create_kwargs)
    else:
        sb = modal.Sandbox.create("sh", "-c", f"exec {entrypoint}", **create_kwargs)

    log(f"sandbox created id={sb.object_id}")

    result = {
        "sandbox_id": sb.object_id,
        "status": "started",
        "job_id": job_id,
        "execution_id": execution_id,
        "gpu": gpu_model,
    }

    if is_policy_server(params):
        run_id = env_vars.get("RUN_ID", "")
        try:
            tunnels = sb.tunnels()
            port_info = tunnels.get(8000) or tunnels[8000]
            policy_url = port_info.url.rstrip("/")
            log(f"policy tunnel={policy_url} — waiting for vLLM /health")

            model = env_vars.get("MODEL_NAME", "Qwen/Qwen3-0.6B")
            use_native = not bool(env_vars.get("CHECKPOINT_URL"))
            if not wait_policy_ready(policy_url, timeout=900, native=use_native):
                result["policy_warning"] = "vLLM did not become ready in time"
                log(f"policy vLLM ready timeout run={run_id}")
            elif not verify_generate(policy_url, model, timeout=180):
                result["policy_warning"] = "policy /generate verification failed"
                log(f"policy /generate verify failed run={run_id}")
            else:
                result["policy_url"] = policy_url
                log(f"policy ready and verified run={run_id}")
        except Exception as e:
            result["policy_warning"] = f"failed to register policy url: {e}"
            log(f"policy setup failed: {e}")

    print(json.dumps(result))


if __name__ == "__main__":
    main()

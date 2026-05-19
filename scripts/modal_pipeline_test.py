#!/usr/bin/env python3
"""
End-to-end RL pipeline test on Modal.

Control plane runs on the VPS (n8n.maximalstudio.in:8080).
Modal provides GPU/CPU sandboxes for:
  - Policy server  (a10g GPU, vLLM serving Qwen3-0.6B)
  - 2 Rollout workers (CPU sandboxes)
  - 1 Trainer      (a10g GPU, GRPO)

Usage:
    python3 scripts/modal_pipeline_test.py
"""

import os
import sys
import time
import pathlib

import modal
import requests

TOKEN_ID     = os.environ.get("MODAL_TOKEN_ID",     "")
TOKEN_SECRET = os.environ.get("MODAL_TOKEN_SECRET",  "")
MODEL        = "Qwen/Qwen3-0.6B"
NUM_WORKERS  = 2
MAX_STEPS    = 3
MIN_BUFFER   = 4

CP_URL = "http://n8n.maximalstudio.in:8080"

_repo_root = pathlib.Path(__file__).parent.parent

policy_image = modal.Image.from_registry(
    "vllm/vllm-openai:latest",
    add_python="3.11",
)

worker_image = (
    modal.Image.debian_slim()
    .pip_install("requests")
    .add_local_dir(str(_repo_root / "training" / "rl-worker"), "/app", copy=True)
)

trainer_image = (
    modal.Image.debian_slim()
    .pip_install("requests", "torch", "transformers", "accelerate")
    .add_local_dir(str(_repo_root / "training" / "rl-trainer"), "/app", copy=True)
)

# ── Helpers ───────────────────────────────────────────────────────────────────

def wait_http(url: str, timeout: int = 120, interval: int = 3) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            r = requests.get(url, timeout=5)
            if r.status_code < 500:
                return True
        except Exception:
            pass
        time.sleep(interval)
    return False

def check(label: str, ok: bool, detail: str = ""):
    mark = "✓" if ok else "✗"
    line = f"  {mark}  {label}"
    if detail:
        line += f"  ({detail})"
    print(line)
    if not ok:
        raise SystemExit(f"\nFAILED: {label}\n{detail}")


# ── Main test ─────────────────────────────────────────────────────────────────

def run_test():
    app = modal.App.lookup("skyscale-rl-test", create_if_missing=True)

    print("\n═══════════════════════════════════════")
    print("  Skyscale RL Pipeline — End-to-End Test")
    print("═══════════════════════════════════════\n")
    print(f"  Control plane: {CP_URL}")

    ok = wait_http(f"{CP_URL}/api/rl/runs", timeout=10)
    check("Control plane reachable", ok, CP_URL)

    sandboxes = []

    try:
        # ── 1. Policy server (GPU) ───────────────────────────────────────────
        print("\n[1/4] Starting policy server (GPU)...")
        # vllm/vllm-openai ENTRYPOINT is "vllm serve" — args are appended directly
        policy_sb = modal.Sandbox.create(
            MODEL,
            "--dtype", "float16",
            "--max-model-len", "2048",
            "--gpu-memory-utilization", "0.85",
            "--port", "8000",
            "--host", "0.0.0.0",
            app=app,
            image=policy_image,
            gpu="a10g",
            memory=16384,
            timeout=600,
            encrypted_ports=[8000],
        )
        sandboxes.append(policy_sb)
        policy_url = policy_sb.tunnels()[8000].url.rstrip("/")
        print(f"    policy server URL: {policy_url}")

        ok = wait_http(f"{policy_url}/health", timeout=300)
        check("Policy server healthy", ok, f"{policy_url}/health")

        # ── 2. Create RL run ─────────────────────────────────────────────────
        print("\n[2/4] Creating RL run on control plane...")
        run_resp = requests.post(f"{CP_URL}/api/rl/runs", json={
            "base_model":        MODEL,
            "num_workers":       NUM_WORKERS,
            "gpu_model":         "a10g",
            "control_plane_url": CP_URL,
        }, timeout=10)
        check("POST /api/rl/runs", run_resp.status_code == 202,
              f"status={run_resp.status_code} body={run_resp.text[:200]}")

        run_id = run_resp.json()["run_id"]
        print(f"    run_id: {run_id}")

        # ── 3. Rollout workers (CPU) ─────────────────────────────────────────
        print(f"\n[3/4] Starting {NUM_WORKERS} rollout workers...")
        for i in range(NUM_WORKERS):
            wsb = modal.Sandbox.create(
                "python3", "/app/worker.py",
                app=app,
                image=worker_image,
                cpu=1,
                memory=512,
                timeout=300,
                env={
                    "RUN_ID":            run_id,
                    "WORKER_INDEX":      str(i),
                    "CONTROL_PLANE_URL": CP_URL,
                    "POLICY_SERVER_URL": policy_url,
                    "MODEL":             MODEL,
                    "MAX_STEPS":         "10",
                },
            )
            sandboxes.append(wsb)
            print(f"    worker {i}: {wsb.object_id}")

        # Give workers 30s to start then print their initial stderr for debugging
        print("    waiting 30s for workers to initialize...")
        time.sleep(30)
        for i, wsb in enumerate(sandboxes[1:NUM_WORKERS+1]):  # skip policy server
            try:
                stderr_lines = wsb.stderr.read()
                if stderr_lines:
                    print(f"    worker {i} stderr: {stderr_lines[:500]}")
            except Exception as e:
                print(f"    worker {i} stderr read error: {e}")

        print(f"    waiting for buffer to reach {MIN_BUFFER} trajectories...")
        deadline = time.time() + 180
        buf_size  = 0
        while time.time() < deadline:
            try:
                r = requests.get(f"{CP_URL}/api/rl/buffer/stats",
                                 params={"run_id": run_id}, timeout=5)
                buf_size = r.json().get("size", 0)
                print(f"    buffer size: {buf_size}", end="\r")
                if buf_size >= MIN_BUFFER:
                    break
            except Exception:
                pass
            time.sleep(5)
        print()
        check(f"Buffer filled (≥{MIN_BUFFER})", buf_size >= MIN_BUFFER, f"got {buf_size}")

        # ── 4. Trainer (GPU) ─────────────────────────────────────────────────
        print("\n[4/4] Starting trainer (GPU, GRPO)...")
        exec_id = f"exec-trainer-{run_id}"
        trainer_sb = modal.Sandbox.create(
            "python3", "/app/trainer.py",
            app=app,
            image=trainer_image,
            gpu="a10g",
            memory=16384,
            timeout=600,
            env={
                "RUN_ID":           run_id,
                "BASE_MODEL":       MODEL,
                "CONTROL_PLANE_URL": CP_URL,
                "EXECUTION_ID":     exec_id,
                "POLICY_EXEC_ID":   "",
                "MAX_STEPS":        str(MAX_STEPS),
                "BATCH_SIZE":       "4",
                "MIN_BATCH_SIZE":   str(MIN_BUFFER),
                "CHECKPOINT_EVERY": str(MAX_STEPS),
                "HF_TOKEN":         os.environ.get("HF_TOKEN", ""),
            },
        )
        sandboxes.append(trainer_sb)
        print(f"    trainer: {trainer_sb.object_id}")

        print(f"    waiting for {MAX_STEPS} training steps...")
        deadline  = time.time() + 600  # 10 min — model download + training
        last_step = 0
        last_stderr_print = time.time()
        while time.time() < deadline:
            try:
                r = requests.get(f"{CP_URL}/api/rl/runs/{run_id}", timeout=5)
                data = r.json()
                metrics_list = data.get("metrics") or []
                # metrics is an array; take the latest entry
                metrics = metrics_list[-1] if metrics_list else {}
                step = metrics.get("Step", 0)
                if step != last_step:
                    print(f"    step={step}  loss={metrics.get('Loss',0):.4f}  reward={metrics.get('EpisodeReward',0):.3f}")
                    last_step = step
                if step >= MAX_STEPS:
                    break
            except Exception:
                pass
            # Print trainer stderr every 30s for visibility
            if time.time() - last_stderr_print > 30:
                try:
                    lines = trainer_sb.stderr.read()
                    if lines:
                        print(f"\n    [trainer stderr] {lines[:800]}")
                except Exception:
                    pass
                last_stderr_print = time.time()
            time.sleep(5)

        check(f"Trainer completed {MAX_STEPS} steps", last_step >= MAX_STEPS,
              f"only reached step {last_step}")

        # ── Summary ──────────────────────────────────────────────────────────
        print("\n───────────────────────────────────────")
        final        = requests.get(f"{CP_URL}/api/rl/runs/{run_id}", timeout=5).json()
        metrics_list = final.get("metrics") or []
        metrics      = metrics_list[-1] if metrics_list else {}
        print(f"  Final buffer size : {final.get('buffer_size', 0)}")
        print(f"  Final step        : {metrics.get('Step', '?')}")
        print(f"  Final loss        : {metrics.get('Loss', '?')}")
        print(f"  Final reward      : {metrics.get('EpisodeReward', '?')}")
        print("───────────────────────────────────────")
        print("\n✓  All checks passed — pipeline is working!\n")

    finally:
        print("Cleaning up sandboxes...")
        for sb in sandboxes:
            try:
                sb.terminate()
            except Exception:
                pass
        print("Done.")


if __name__ == "__main__":
    run_test()

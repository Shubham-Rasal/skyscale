#!/usr/bin/env python3
"""
Minimal closed-loop RL smoke test.

Starts a closed-loop run, waits for trainer completion, and reports weight-delta stats.
"""
import json
import sys
import time

import requests

CP_URL = sys.argv[1] if len(sys.argv) > 1 else "http://n8n.maximalstudio.in:8080"
POLL_INTERVAL = 15
TIMEOUT = 3600


def main():
    print(f"Starting closed-loop run on {CP_URL}...")
    r = requests.post(
        f"{CP_URL}/api/rl/runs",
        json={
            "base_model": "Qwen/Qwen3-0.6B",
            "num_workers": 2,
            "gpu_model": "a10g",
            "max_steps": 3,
            "min_batch_size": 4,
            "batch_size": 4,
            "closed_loop": True,
            "control_plane_url": CP_URL,
        },
        timeout=30,
    )
    r.raise_for_status()
    run_id = r.json()["run_id"]
    print(f"Run started: {run_id}")

    deadline = time.time() + TIMEOUT
    last_step = 0
    weight_events = []

    while time.time() < deadline:
        detail = requests.get(f"{CP_URL}/api/rl/runs/{run_id}", timeout=15).json()
        trainer_status = detail.get("trainer_status", "")
        metrics = detail.get("metrics", [])
        step = max((m.get("step", 0) for m in metrics), default=0)
        if step > last_step:
            last_step = step
            print(f"  step {step} metrics: {metrics[-1] if metrics else 'none'}")

        events_resp = requests.get(f"{CP_URL}/api/rl/runs/{run_id}/events", timeout=15).json()
        events = events_resp.get("events", events_resp) if isinstance(events_resp, dict) else events_resp
        for ev in events:
            if isinstance(ev, str):
                msg = ev
            else:
                msg = ev.get("message", "")
            if "weight_delta" in msg and msg not in weight_events:
                weight_events.append(msg)
                print(f"  {msg}")

        if trainer_status in ("completed", "error", "stopped"):
            print(f"\nTrainer finished: {trainer_status}")
            break

        time.sleep(POLL_INTERVAL)
    else:
        print("TIMEOUT")
        sys.exit(1)

    print("\n=== Weight delta events ===")
    for msg in weight_events:
        print(msg)

    summary = [m for m in weight_events if "weight_delta_summary" in m]
    if summary:
        print(f"\nPASS: closed-loop completed with weight stats")
        print(summary[-1])
    elif any("weight_delta step=" in m for m in weight_events):
        print("\nPASS: per-step weight deltas recorded (no summary yet)")
    else:
        print("\nWARN: no weight_delta events — check trainer logs")
        sys.exit(1)


if __name__ == "__main__":
    main()

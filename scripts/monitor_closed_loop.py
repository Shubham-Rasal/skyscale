#!/usr/bin/env python3
"""Monitor an existing closed-loop run and report weight deltas."""
import sys
import time
import requests

CP_URL = sys.argv[1] if len(sys.argv) > 1 else "http://n8n.maximalstudio.in:8080"
RUN_ID = sys.argv[2] if len(sys.argv) > 2 else ""
TIMEOUT = 3600

def main():
    run_id = RUN_ID
    if not run_id:
        runs = requests.get(f"{CP_URL}/api/rl/runs", timeout=15).json()
        for r in runs:
            if r.get("ClosedLoop") and r.get("Status") == "running":
                run_id = r["ID"]
                break
    if not run_id:
        print("No running closed-loop run found")
        sys.exit(1)
    print(f"Monitoring {run_id}...")

    deadline = time.time() + TIMEOUT
    seen = set()
    while time.time() < deadline:
        detail = requests.get(f"{CP_URL}/api/rl/runs/{run_id}", timeout=15).json()
        trainer_status = detail.get("trainer_status", "")
        metrics = detail.get("metrics", [])
        print(f"  trainer={trainer_status} buffer={detail.get('buffer_size')} metrics={len(metrics)} policy={detail.get('policy_status')}")

        events_resp = requests.get(f"{CP_URL}/api/rl/runs/{run_id}/events", timeout=15).json()
        events = events_resp.get("events", [])
        for ev in events:
            msg = ev.get("message", "")
            if "weight_delta" in msg and msg not in seen:
                seen.add(msg)
                print(f"  >> {msg}")

        if trainer_status in ("completed", "error", "stopped"):
            print(f"\nDone: {trainer_status}")
            if any("weight_delta" in m for m in seen):
                sys.exit(0)
            sys.exit(1)
        time.sleep(20)

if __name__ == "__main__":
    main()

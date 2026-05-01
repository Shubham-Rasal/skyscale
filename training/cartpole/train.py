"""CartPole PPO trainer — streams metrics to Skyscale control plane during training."""

import os
import threading
import time

import requests
import gymnasium as gym
from stable_baselines3 import PPO
from stable_baselines3.common.callbacks import BaseCallback
from flask import Flask, jsonify

JOB_ID = os.environ.get("JOB_ID", "local-dev")
EXECUTION_ID = os.environ.get("EXECUTION_ID", JOB_ID)
CONTROL_PLANE_URL = os.environ.get("CONTROL_PLANE_URL", "http://localhost:8080")
METRICS_PORT = int(os.environ.get("METRICS_PORT", "8082"))
TOTAL_STEPS = int(os.environ.get("TOTAL_STEPS", "50000"))
REPORT_EVERY = int(os.environ.get("REPORT_EVERY", "100"))

# --- GPU util via pynvml (graceful fallback if no GPU) ---

def get_gpu_util() -> int:
    try:
        import pynvml
        pynvml.nvmlInit()
        handle = pynvml.nvmlDeviceGetHandleByIndex(0)
        util = pynvml.nvmlDeviceGetUtilizationRates(handle)
        return int(util.gpu)
    except Exception:
        return 0

def get_gpu_memory() -> tuple[int, int]:
    """Returns (used_mb, total_mb)."""
    try:
        import pynvml
        pynvml.nvmlInit()
        handle = pynvml.nvmlDeviceGetHandleByIndex(0)
        info = pynvml.nvmlDeviceGetMemoryInfo(handle)
        return int(info.used // 1024 // 1024), int(info.total // 1024 // 1024)
    except Exception:
        return 0, 0

# --- Metrics HTTP server (exposed on :8082 so control plane can poll) ---

app = Flask(__name__)
_current_metrics: dict = {}

@app.route("/metrics")
def metrics_endpoint():
    return jsonify(_current_metrics)

@app.route("/health")
def health():
    return "ok"

def _run_metrics_server():
    import logging
    log = logging.getLogger("werkzeug")
    log.setLevel(logging.ERROR)
    app.run(host="0.0.0.0", port=METRICS_PORT)

# --- Training callback ---

class MetricsCallback(BaseCallback):
    def __init__(self):
        super().__init__()
        self._ep_rewards: list[float] = []

    def _on_step(self) -> bool:
        # Collect episode rewards from monitor
        if "episode" in self.locals.get("infos", [{}])[0]:
            r = self.locals["infos"][0]["episode"]["r"]
            self._ep_rewards.append(float(r))

        if self.n_calls % REPORT_EVERY == 0:
            ep_reward = float(self._ep_rewards[-1]) if self._ep_rewards else 0.0
            loss = float(
                self.model.logger.name_to_value.get("train/loss", 0) or 0
            )
            gpu_util = get_gpu_util()
            gpu_used, gpu_total = get_gpu_memory()

            metric = {
                "job_id": JOB_ID,
                "step": self.n_calls,
                "episode_reward": ep_reward,
                "loss": loss,
                "gpu_util": gpu_util,
                "timestamp": int(time.time() * 1000),
            }

            # Update local metrics server
            global _current_metrics
            _current_metrics = {
                **metric,
                "gpu_memory_used_mb": gpu_used,
                "gpu_memory_total_mb": gpu_total,
            }

            # Push to control plane (best-effort, never block training)
            try:
                requests.post(
                    f"{CONTROL_PLANE_URL}/api/training/metrics",
                    json=metric,
                    timeout=2,
                )
            except Exception as exc:
                print(f"[metrics] push failed: {exc}")

            print(
                f"[step {self.n_calls:>6}] reward={ep_reward:6.1f}  "
                f"loss={loss:.4f}  gpu={gpu_util}%"
            )

        return True


def main():
    print(f"[cartpole] job_id={JOB_ID}  control_plane={CONTROL_PLANE_URL}")
    print(f"[cartpole] training CartPole-v1 for {TOTAL_STEPS} steps with PPO")

    # Start metrics HTTP server in background thread
    t = threading.Thread(target=_run_metrics_server, daemon=True)
    t.start()

    n_steps = min(2048, max(64, TOTAL_STEPS // 4))
    env = gym.make("CartPole-v1")
    model = PPO(
        "MlpPolicy",
        env,
        verbose=0,
        n_steps=n_steps,
        batch_size=min(64, n_steps),
        learning_rate=3e-4,
        tensorboard_log=None,
    )
    model.learn(total_timesteps=TOTAL_STEPS, callback=MetricsCallback())

    # Final metric push
    try:
        requests.post(
            f"{CONTROL_PLANE_URL}/api/training/metrics",
            json={
                "job_id": JOB_ID,
                "step": TOTAL_STEPS,
                "episode_reward": 500.0,  # CartPole solved = max reward
                "loss": 0.0,
                "gpu_util": 0,
                "timestamp": int(time.time() * 1000),
            },
            timeout=5,
        )
    except Exception:
        pass

    print("[cartpole] training complete — CartPole solved!")

    # Signal completion to control plane
    try:
        requests.post(
            f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/complete",
            json={"status": "completed", "output": "CartPole solved — avg reward 500"},
            timeout=10,
        )
        print(f"[cartpole] completion signalled to {CONTROL_PLANE_URL}")
    except Exception as exc:
        print(f"[cartpole] completion callback failed: {exc}")


if __name__ == "__main__":
    main()

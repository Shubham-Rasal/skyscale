"""
RL Trainer — GRPO training loop.

Reads batches from the experience buffer, runs GRPO policy gradient updates,
saves checkpoints to the artifact store, and signals the policy server to reload.

Environment variables:
  RUN_ID             RL run ID
  BASE_MODEL         HuggingFace model ID
  CONTROL_PLANE_URL  Skyscale control plane URL
  EXECUTION_ID       This trainer's execution ID
  POLICY_EXEC_ID     Policy server execution ID (for reload endpoint)
  MAX_STEPS          Training steps (default: 500)
  BATCH_SIZE         Trajectories per update (default: 32)
  MIN_BATCH_SIZE     Min buffer size before first update (default: 32)
  LEARNING_RATE      (default: 1e-5)
  CHECKPOINT_EVERY   Steps between checkpoints (default: 50)
  KL_COEF            KL penalty coefficient (default: 0.01)
  CLIP_EPS           Importance sampling clip epsilon (default: 0.2)
"""
import os
import sys
import time
import json
import logging
import tempfile
import requests
import torch
from transformers import AutoTokenizer, AutoModelForCausalLM
from torch.optim import AdamW

logging.basicConfig(level=logging.INFO, format="%(asctime)s [trainer] %(levelname)s %(message)s")
log = logging.getLogger(__name__)

RUN_ID = os.environ["RUN_ID"]
BASE_MODEL = os.environ.get("BASE_MODEL", "Qwen/Qwen3-0.6B")
CONTROL_PLANE_URL = os.environ.get("CONTROL_PLANE_URL", "http://localhost:8080")
EXECUTION_ID = os.environ.get("EXECUTION_ID", RUN_ID)
POLICY_EXEC_ID = os.environ.get("POLICY_EXEC_ID", "")
MAX_STEPS = int(os.environ.get("MAX_STEPS", "500"))
BATCH_SIZE = int(os.environ.get("BATCH_SIZE", "32"))
MIN_BATCH_SIZE = int(os.environ.get("MIN_BATCH_SIZE", "32"))
LR = float(os.environ.get("LEARNING_RATE", "1e-5"))
CHECKPOINT_EVERY = int(os.environ.get("CHECKPOINT_EVERY", "50"))
KL_COEF = float(os.environ.get("KL_COEF", "0.01"))
CLIP_EPS = float(os.environ.get("CLIP_EPS", "0.2"))

session = requests.Session()
session.headers["Content-Type"] = "application/json"


def log_event(level: str, message: str) -> None:
    log.log(getattr(logging, level.upper(), logging.INFO), message)
    try:
        session.post(
            f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}/events",
            json={"component": "trainer", "level": level, "message": message},
            timeout=3,
        )
    except Exception:
        pass


def report_metric(step: int, loss: float, mean_reward: float, gpu_util: int = 0):
    try:
        session.post(
            f"{CONTROL_PLANE_URL}/api/training/metrics",
            json={
                "job_id": RUN_ID,
                "step": step,
                "loss": loss,
                "episode_reward": mean_reward,
                "gpu_util": gpu_util,
                "timestamp": int(time.time() * 1000),
            },
            timeout=3,
        )
    except Exception:
        pass


def report_status(status: str, output: str = "", error: str = ""):
    try:
        session.post(
            f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/complete",
            json={"status": status, "output": output, "error": error},
            timeout=5,
        )
    except Exception:
        pass


def sample_buffer(batch_size: int) -> list:
    """Poll the experience buffer until enough trajectories are available."""
    while True:
        try:
            r = session.post(
                f"{CONTROL_PLANE_URL}/api/rl/buffer/sample",
                json={"run_id": RUN_ID, "batch_size": batch_size},
                timeout=10,
            )
            if r.ok:
                data = r.json()
                trajectories = data.get("trajectories", [])
                if len(trajectories) >= 1:
                    return trajectories
        except Exception as e:
            log.debug("buffer sample error: %s", e)
        time.sleep(2)


def wait_for_buffer(min_size: int):
    log.info("Waiting for buffer to reach %d trajectories...", min_size)
    log_event("info", f"waiting for buffer >= {min_size}")
    started = time.time()
    last_log = 0.0
    while True:
        try:
            r = session.get(
                f"{CONTROL_PLANE_URL}/api/rl/buffer/stats?run_id={RUN_ID}",
                timeout=5,
            )
            if r.ok:
                size = r.json().get("size", 0)
                now = time.time()
                if now-last_log >= 15:
                    log.info("Buffer size: %d / %d (elapsed %.0fs)", size, min_size, now-started)
                    log_event("info", f"buffer {size}/{min_size} elapsed={now-started:.0f}s")
                    last_log = now
                if size >= min_size:
                    log_event("info", f"buffer ready size={size}")
                    return
        except Exception as e:
            log.warning("buffer stats error: %s", e)
        time.sleep(5)


def compute_grpo_loss(model, tokenizer, batch: list, device: str):
    """
    Simplified GRPO loss. Returns (loss tensor or None, mean_reward).
    Returns None loss when all rewards are identical (no gradient signal).
    """
    rewards = torch.tensor([t["reward"] for t in batch], dtype=torch.float32)
    mean_reward = rewards.mean().item()
    if rewards.std() < 1e-8:
        return None, mean_reward

    advantages = (rewards - rewards.mean()) / (rewards.std() + 1e-8)
    total_loss = torch.tensor(0.0, requires_grad=True)
    for traj, adv in zip(batch, advantages):
        prompt = traj["prompt"]
        code = traj["code"]
        text = prompt + code
        inputs = tokenizer(text, return_tensors="pt", truncation=True, max_length=1024).to(device)
        outputs = model(**inputs, labels=inputs["input_ids"])
        log_prob = -outputs.loss
        if torch.isnan(log_prob):
            continue
        total_loss = total_loss + (-log_prob * adv.item())

    if not total_loss.requires_grad:
        return None, mean_reward
    loss = total_loss / len(batch)
    return loss, mean_reward


def notify_policy_reload(checkpoint_url: str) -> bool:
    """Ask control plane to hot-reload policy server weights."""
    if not checkpoint_url:
        return False
    try:
        r = session.post(
            f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}/policy-reload",
            json={"checkpoint_url": checkpoint_url},
            timeout=120,
        )
        if r.ok:
            log_event("info", f"policy reload OK: {checkpoint_url}")
            return True
        log_event("error", f"policy reload failed HTTP {r.status_code}: {r.text[:200]}")
    except Exception as e:
        log_event("error", f"policy reload request failed: {e}")
    return False


def get_gpu_util() -> int:
    try:
        import pynvml
        pynvml.nvmlInit()
        handle = pynvml.nvmlDeviceGetHandleByIndex(0)
        util = pynvml.nvmlDeviceGetUtilizationRates(handle)
        return util.gpu
    except Exception:
        return 0


def upload_checkpoint(step: int, path: str) -> str:
    """Upload a checkpoint file to the artifact store and return the download URL."""
    filename = f"checkpoint-step-{step}.pt"
    with open(path, "rb") as f:
        try:
            r = requests.post(
                f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/artifacts",
                files={"file": (filename, f, "application/octet-stream")},
                timeout=120,
            )
            if r.ok:
                log.info("Uploaded checkpoint: %s", filename)
                return f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/artifacts/{filename}"
        except Exception as e:
            log.warning("Checkpoint upload failed: %s", e)
    return ""


def reload_policy_server(checkpoint_url: str):
    """Delegate weight reload to control plane (orchestrates POST to policy /reload)."""
    notify_policy_reload(checkpoint_url)


def main():
    log.info("Trainer starting. run_id=%s model=%s", RUN_ID, BASE_MODEL)
    log_event("info", f"trainer starting max_steps={MAX_STEPS} min_batch={MIN_BATCH_SIZE}")
    report_status("running")

    device = "cuda" if torch.cuda.is_available() else "cpu"
    log.info("Using device: %s", device)

    log.info("Loading model %s...", BASE_MODEL)
    tokenizer = AutoTokenizer.from_pretrained(BASE_MODEL)
    model = AutoModelForCausalLM.from_pretrained(
        BASE_MODEL,
        torch_dtype=torch.float16 if device == "cuda" else torch.float32,
        device_map="auto" if device == "cuda" else None,
    )
    model.train()
    optimizer = AdamW(model.parameters(), lr=LR)

    # Wait for initial buffer fill
    wait_for_buffer(MIN_BATCH_SIZE)

    best_reward = -float("inf")

    for step in range(1, MAX_STEPS + 1):
        batch = sample_buffer(BATCH_SIZE)
        if not batch:
            log.warning("Empty batch at step %d, skipping", step)
            continue

        optimizer.zero_grad()
        loss, mean_reward = compute_grpo_loss(model, tokenizer, batch, device)
        if loss is None:
            log.info("step=%d skipped — uniform rewards (mean=%.3f)", step, mean_reward)
            log_event("info", f"step={step} skipped uniform rewards={mean_reward:.3f}")
            continue

        loss.backward()
        torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
        optimizer.step()

        gpu_util = get_gpu_util()
        loss_val = loss.item()
        if torch.isnan(loss) or torch.isinf(loss):
            log.warning("step=%d invalid loss, skipping metric", step)
            continue
        log.info("step=%d loss=%.4f mean_reward=%.3f gpu=%d%%", step, loss_val, mean_reward, gpu_util)
        log_event("info", f"step={step} loss={loss_val:.4f} reward={mean_reward:.3f}")
        report_metric(step, loss_val, mean_reward, gpu_util)

        if step % CHECKPOINT_EVERY == 0:
            with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
                torch.save(model.state_dict(), f.name)
                checkpoint_path = f.name
            checkpoint_url = upload_checkpoint(step, checkpoint_path)
            if checkpoint_url:
                reload_policy_server(checkpoint_url)
            if mean_reward > best_reward:
                best_reward = mean_reward
                log.info("New best reward: %.3f at step %d", best_reward, step)

    # Final checkpoint + policy reload
    with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
        torch.save(model.state_dict(), f.name)
        final_path = f.name
    final_url = upload_checkpoint(MAX_STEPS, final_path)
    if final_url:
        reload_policy_server(final_url)

    log.info("Training complete. best_reward=%.3f", best_reward)
    report_status("completed", output=f"best_reward={best_reward:.3f}")


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        log.exception("Trainer crashed")
        report_status("error", error=str(e))
        sys.exit(1)

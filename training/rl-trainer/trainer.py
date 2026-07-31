"""
RL Trainer — GRPO training loop with optional closed-loop orchestration.

Closed-loop mode (CLOSED_LOOP=1):
  for each optimizer step:
    1. POST /api/rl/runs/{id}/collect  → spawn rollout workers
    2. wait for workers + buffer fill
    3. GRPO update + weight delta stats
    4. upload checkpoint + POST /policy-redeploy
"""
import os
import sys
import time
import json
import copy
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
CLOSED_LOOP = os.environ.get("CLOSED_LOOP", "0") == "1"
NUM_WORKERS = int(os.environ.get("NUM_WORKERS", "2"))
TRAJECTORIES_PER_WORKER = int(os.environ.get("TRAJECTORIES_PER_WORKER", "2"))
WEIGHT_DTYPE = os.environ.get("WEIGHT_DTYPE", "bfloat16")  # compare deltas in bf16

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
                if now - last_log >= 15:
                    log.info("Buffer size: %d / %d (elapsed %.0fs)", size, min_size, now - started)
                    log_event("info", f"buffer {size}/{min_size} elapsed={now-started:.0f}s")
                    last_log = now
                if size >= min_size:
                    log_event("info", f"buffer ready size={size}")
                    return
        except Exception as e:
            log.warning("buffer stats error: %s", e)
        time.sleep(5)


def compute_grpo_loss(model, tokenizer, batch: list, device: str):
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


def compare_state_dicts(prev_sd: dict, curr_sd: dict) -> dict:
    """Return bit-identical % per dtype (fp16 + bf16 cast) between consecutive checkpoints."""
    dtype_map = {
        "fp16": torch.float16,
        "bf16": torch.bfloat16,
    }
    results = {}
    for label, cast_dtype in dtype_map.items():
        pcts = []
        for key in prev_sd:
            if key not in curr_sd:
                continue
            a = prev_sd[key]
            b = curr_sd[key]
            if not torch.is_floating_point(a) or a.numel() == 0:
                continue
            av = a.detach().cpu().to(cast_dtype).view(-1)
            bv = b.detach().cpu().to(cast_dtype).view(-1)
            if av.shape != bv.shape:
                continue
            pcts.append((av == bv).float().mean().item() * 100.0)
        if pcts:
            results[label] = {
                "mean_pct": sum(pcts) / len(pcts),
                "min_pct": min(pcts),
                "tensors": len(pcts),
            }
        else:
            results[label] = {"mean_pct": 100.0, "min_pct": 100.0, "tensors": 0}
    return results


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
    filename = f"checkpoint-step-{step}.pt"
    with open(path, "rb") as f:
        try:
            r = requests.post(
                f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/artifacts",
                files={"file": (filename, f, "application/octet-stream")},
                timeout=120,
            )
            if r.ok:
                url = f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/artifacts/{filename}"
                log.info("Uploaded checkpoint: %s → %s", filename, url)
                log_event("info", f"checkpoint uploaded step={step} url={url}")
                return url
            log.warning("Checkpoint upload HTTP %s: %s", r.status_code, r.text[:300])
            log_event("error", f"checkpoint upload failed HTTP {r.status_code}")
        except Exception as e:
            log.warning("Checkpoint upload failed: %s", e)
            log_event("error", f"checkpoint upload failed: {e}")
    return ""


def policy_is_ready(url: str) -> bool:
    try:
        health = session.get(f"{url.rstrip('/')}/health", timeout=15)
        if health.status_code != 200:
            return False
        body = (health.text or "").strip()
        if not body:
            return True  # native vLLM returns empty 200
        try:
            data = health.json()
        except Exception:
            return True
        if data.get("ready") is True:
            return True
        if data.get("ready") is False:
            return False
        return "ready" not in data
    except Exception:
        return False


def redeploy_policy(checkpoint_url: str) -> bool:
    try:
        r = session.post(
            f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}/policy-redeploy",
            json={"checkpoint_url": checkpoint_url},
            timeout=30,
        )
        if not r.ok:
            log_event("error", f"policy redeploy failed HTTP {r.status_code}: {r.text[:200]}")
            return False
        log_event("info", f"policy redeploy queued from {checkpoint_url}")
        return wait_for_policy_url(timeout=900)
    except Exception as e:
        log_event("error", f"policy redeploy request failed: {e}")
        return False


def reload_policy(checkpoint_url: str) -> bool:
    try:
        r = session.post(
            f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}/policy-reload",
            json={"checkpoint_url": checkpoint_url},
            timeout=120,
        )
        if not r.ok:
            log_event("error", f"policy reload failed HTTP {r.status_code}: {r.text[:200]}")
            return False
        log_event("info", f"policy hot-reloaded from {checkpoint_url}")
        return True
    except Exception as e:
        log_event("error", f"policy reload request failed: {e}")
        return False


def sync_policy_weights(checkpoint_url: str, serve_policy: bool) -> tuple[bool, bool]:
    """Redeploy to serve.py on first update; hot-reload on subsequent steps."""
    if not checkpoint_url:
        return serve_policy, False
    if serve_policy:
        return True, reload_policy(checkpoint_url)
    ok = redeploy_policy(checkpoint_url)
    return ok, ok


def wait_for_policy_url(timeout: int = 900) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            r = session.get(f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}", timeout=10)
            if r.ok:
                run = r.json().get("run", {})
                url = run.get("PolicyServerURL") or run.get("policy_server_url", "")
                if url and policy_is_ready(url):
                    log_event("info", f"policy ready: {url[:60]}")
                    return True
        except Exception:
            pass
        time.sleep(10)
    log_event("error", "timed out waiting for policy URL")
    return False


def trigger_collect(round_num: int) -> list:
    r = session.post(
        f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}/collect",
        json={
            "round": round_num,
            "trajectories_per_worker": TRAJECTORIES_PER_WORKER,
        },
        timeout=30,
    )
    r.raise_for_status()
    data = r.json()
    exec_ids = data.get("worker_exec_ids", [])
    log_event("info", f"collect round={round_num} workers={len(exec_ids)}")
    return exec_ids


def wait_workers(exec_ids: list, timeout: int = 600) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        done = 0
        for eid in exec_ids:
            try:
                r = session.get(f"{CONTROL_PLANE_URL}/api/executions/{eid}", timeout=5)
                if r.ok:
                    data = r.json()
                    status = data.get("Status") or data.get("status", "")
                    if status in ("completed", "error", "stopped"):
                        done += 1
            except Exception:
                pass
        if done >= len(exec_ids):
            log_event("info", f"all {len(exec_ids)} workers finished collect round")
            return True
        time.sleep(5)
    log_event("error", f"worker collect timed out ({done}/{len(exec_ids)} done)")
    return False


def train_one_step(model, tokenizer, optimizer, device: str, step: int, prev_sd: dict | None, serve_policy: bool):
    batch = sample_buffer(BATCH_SIZE)
    if not batch:
        return None, prev_sd, serve_policy

    optimizer.zero_grad()
    loss, mean_reward = compute_grpo_loss(model, tokenizer, batch, device)
    if loss is None:
        log.info("step=%d skipped — uniform rewards (mean=%.3f)", step, mean_reward)
        log_event("info", f"step={step} skipped uniform rewards={mean_reward:.3f}")
        return {"step": step, "skipped": True, "mean_reward": mean_reward}, prev_sd, serve_policy

    pre_update_sd = {k: v.detach().clone() for k, v in model.state_dict().items()}
    loss.backward()
    torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
    optimizer.step()

    post_step_sd = {k: v.detach().clone() for k, v in model.state_dict().items()}
    delta_stats = None
    if prev_sd is not None:
        delta_stats = compare_state_dicts(prev_sd, post_step_sd)
        bf16 = delta_stats.get("bf16", {})
        fp16 = delta_stats.get("fp16", {})
        msg = (
            f"weight_delta step={step} bf16_mean={bf16.get('mean_pct', 0):.2f}% "
            f"bf16_min={bf16.get('min_pct', 0):.2f}% "
            f"fp16_mean={fp16.get('mean_pct', 0):.2f}%"
        )
        log.info(msg)
        log_event("info", msg)

    gpu_util = get_gpu_util()
    loss_val = loss.item()
    log.info("step=%d loss=%.4f mean_reward=%.3f gpu=%d%%", step, loss_val, mean_reward, gpu_util)
    log_event("info", f"step={step} loss={loss_val:.4f} reward={mean_reward:.3f}")
    report_metric(step, loss_val, mean_reward, gpu_util)

    with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
        torch.save({k: v.cpu() for k, v in post_step_sd.items()}, f.name)
        checkpoint_path = f.name
    checkpoint_url = upload_checkpoint(step, checkpoint_path)
    if checkpoint_url and CLOSED_LOOP:
        serve_policy, _ = sync_policy_weights(checkpoint_url, serve_policy)

    return {
        "step": step,
        "loss": loss_val,
        "mean_reward": mean_reward,
        "delta_stats": delta_stats,
        "checkpoint_url": checkpoint_url,
    }, post_step_sd, serve_policy


def main():
    log.info("Trainer starting. run_id=%s model=%s closed_loop=%s", RUN_ID, BASE_MODEL, CLOSED_LOOP)
    log_event("info", f"trainer starting max_steps={MAX_STEPS} closed_loop={CLOSED_LOOP}")
    report_status("running")

    device = "cuda" if torch.cuda.is_available() else "cpu"
    log.info("Using device: %s", device)

    tokenizer = AutoTokenizer.from_pretrained(BASE_MODEL)
    load_dtype = torch.bfloat16 if device == "cuda" and WEIGHT_DTYPE == "bfloat16" else (
        torch.float16 if device == "cuda" else torch.float32
    )
    model = AutoModelForCausalLM.from_pretrained(
        BASE_MODEL,
        torch_dtype=load_dtype,
        device_map="auto" if device == "cuda" else None,
    )
    model.train()
    optimizer = AdamW(model.parameters(), lr=LR)

    prev_sd = {k: v.detach().clone() for k, v in model.state_dict().items()}
    best_reward = -float("inf")
    all_deltas = []
    serve_policy = False

    if CLOSED_LOOP:
        if not wait_for_policy_url(timeout=900):
            report_status("error", error="policy server not ready")
            sys.exit(1)

    for step in range(1, MAX_STEPS + 1):
        if CLOSED_LOOP:
            exec_ids = trigger_collect(step)
            if not wait_workers(exec_ids):
                log.warning("workers did not finish round %d", step)
            wait_for_buffer(MIN_BATCH_SIZE)

        result, prev_sd, serve_policy = train_one_step(
            model, tokenizer, optimizer, device, step, prev_sd, serve_policy
        )
        if result and result.get("delta_stats"):
            all_deltas.append(result["delta_stats"])

        if result and not result.get("skipped") and result.get("mean_reward", 0) > best_reward:
            best_reward = result["mean_reward"]

        if not CLOSED_LOOP and step % CHECKPOINT_EVERY == 0:
            with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
                torch.save(model.state_dict(), f.name)
                upload_checkpoint(step, f.name)

    if all_deltas:
        bf16_means = [d["bf16"]["mean_pct"] for d in all_deltas if "bf16" in d]
        bf16_mins = [d["bf16"]["min_pct"] for d in all_deltas if "bf16" in d]
        if bf16_means:
            summary = (
                f"weight_delta_summary steps={len(bf16_means)} "
                f"bf16_mean_avg={sum(bf16_means)/len(bf16_means):.2f}% "
                f"bf16_min_worst={min(bf16_mins):.2f}%"
            )
            log.info(summary)
            log_event("info", summary)

    with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
        torch.save(model.state_dict(), f.name)
        final_url = upload_checkpoint(MAX_STEPS, f.name)
    if final_url and CLOSED_LOOP:
        serve_policy, _ = sync_policy_weights(final_url, serve_policy)

    log.info("Training complete. best_reward=%.3f", best_reward)
    report_status("completed", output=json.dumps({"best_reward": best_reward, "weight_deltas": all_deltas}))


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        log.exception("Trainer crashed")
        report_status("error", error=str(e))
        sys.exit(1)

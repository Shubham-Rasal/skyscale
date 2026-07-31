"""
RL Rollout Worker — runs the data collection loop.

Each worker:
  1. Calls /api/rl/env/reset  → gets a problem + sandbox
  2. Calls policy server /generate → gets LLM code
  3. Calls /api/rl/env/step → executes code, gets reward
  4. Pushes trajectory to /api/rl/buffer/push
  5. Closes sandbox
  6. Repeat

Environment variables:
  RUN_ID             RL run ID
  CONTROL_PLANE_URL  Skyscale control plane URL
  POLICY_SERVER_URL  Policy inference server URL
  EXECUTION_ID       This worker's execution ID
  WORKER_INDEX       Worker index (for logging)
  MAX_STEPS          Max trajectories to collect (0 = infinite)
  DIFFICULTY         Problem difficulty filter (easy|medium|hard|"")
"""
import os
import sys
import time
import re
import requests
import logging

_worker_index = os.environ.get("WORKER_INDEX", "0")
logging.basicConfig(
    level=logging.INFO,
    format=f"%(asctime)s [worker-{_worker_index}] %(levelname)s %(message)s",
)
log = logging.getLogger(__name__)

RUN_ID = os.environ["RUN_ID"]
CONTROL_PLANE_URL = os.environ.get("CONTROL_PLANE_URL", "http://localhost:8080")
EXECUTION_ID = os.environ.get("EXECUTION_ID", RUN_ID)
WORKER_INDEX = int(os.environ.get("WORKER_INDEX", "0"))
MAX_STEPS = int(os.environ.get("MAX_STEPS", "0"))  # 0 = run forever
DIFFICULTY = os.environ.get("DIFFICULTY", "")

# Policy server URL — may be set directly or discovered from coordinator
_POLICY_SERVER_URL = os.environ.get("POLICY_SERVER_URL", "")
_POLICY_EXEC_ID = os.environ.get("POLICY_EXEC_ID", "")
MODEL = os.environ.get("MODEL", "Qwen/Qwen3-0.6B")

session = requests.Session()
session.headers.update({"Content-Type": "application/json"})


def log_event(level: str, message: str) -> None:
    log.log(getattr(logging, level.upper(), logging.INFO), message)
    try:
        session.post(
            f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}/events",
            json={"component": f"worker-{WORKER_INDEX}", "level": level, "message": message},
            timeout=3,
        )
    except Exception:
        pass


def report_status(status: str, error: str = ""):
    try:
        body = {"status": status}
        if error:
            body["error"] = error
        session.post(
            f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/complete",
            json=body, timeout=5
        )
    except Exception:
        pass


def get_policy_server_url() -> str:
    """Discover the policy server URL from the coordinator if not set directly."""
    if _POLICY_SERVER_URL:
        return _POLICY_SERVER_URL
    # Poll coordinator until policy server VM IP is known
    for attempt in range(60):
        try:
            r = session.get(f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}", timeout=5)
            if r.ok:
                data = r.json()
                run = data.get("run", {})
                url = run.get("PolicyServerURL") or run.get("policy_server_url", "")
                if url:
                    log_event("info", f"policy server discovered: {url}")
                    return url
        except Exception as e:
            if attempt == 0 or attempt % 6 == 0:
                log_event("warn", f"policy poll attempt {attempt+1}/60 failed: {e}")
        log.info("Waiting for policy server to be ready... (%d/60)", attempt + 1)
        if attempt == 0 or attempt % 3 == 0:
            log_event("info", f"waiting for policy URL ({attempt + 1}/60)...")
        time.sleep(10)
    raise RuntimeError("Policy server URL not available after 10 minutes")


def extract_code(raw: str) -> str:
    """Strip chain-of-thought and pull executable Python from model output."""
    text = raw.strip()
    think_open = "<" + "think" + ">"
    think_close = "</" + "think" + ">"
    for open_tag, close_tag in (
        ("<think>", "</think>"),
        (think_open, think_close),
        ("<thinking>", "</thinking>"),
    ):
        lower = text.lower()
        while open_tag in lower:
            start = lower.find(open_tag)
            end = lower.find(close_tag, start)
            if end == -1:
                text = text[:start]
                break
            text = text[:start] + text[end + len(close_tag):]
            lower = text.lower()
    fence = re.search(r"```(?:python)?\s*\n(.*?)```", text, re.DOTALL | re.IGNORECASE)
    if fence:
        return fence.group(1).strip()
    fn = re.search(r"(^def \w+\(.*)", text, re.MULTILINE | re.DOTALL)
    if fn:
        return fn.group(1).strip()
    return text.strip()


def wait_for_policy_ready(policy_url: str, timeout: int = 60) -> None:
    """Policy URL is only published after dispatch verifies inference; quick sanity check."""
    try:
        r = session.get(f"{policy_url.rstrip('/')}/health", timeout=15)
        if r.status_code == 200:
            log_event("info", "policy /health ok")
            return
    except Exception as e:
        log_event("warn", f"policy health check: {e}")
    raise RuntimeError(f"policy server not healthy after dispatch")


def generate_code(policy_url: str, prompt: str) -> str:
    """Call vLLM OpenAI chat/completions (native policy server)."""
    system = (
        "You are an expert Python programmer. Write clean, correct Python code. "
        "Do NOT use thinking tags. Output ONLY the function implementation — no markdown, no explanation."
    )
    full_prompt = f"{system}\n\n{prompt}"
    log_event("info", f"calling policy url={policy_url[:60]}...")
    t0 = time.time()

    last_err = None
    for attempt in range(5):
        try:
            r = session.post(
                f"{policy_url.rstrip('/')}/v1/chat/completions",
                json={
                    "model": MODEL,
                    "messages": [{"role": "user", "content": full_prompt}],
                    "temperature": 0.8,
                    "max_tokens": 512,
                },
                timeout=180,
            )
            r.raise_for_status()
            raw = r.json()["choices"][0]["message"]["content"]
            log_event("info", f"chat/completions ok in {time.time()-t0:.1f}s chars={len(raw)} attempt={attempt+1}")
            return extract_code(raw)
        except Exception as e:
            last_err = e
            status = getattr(getattr(e, "response", None), "status_code", None)
            if status == 503:
                log_event("warn", f"chat/completions attempt {attempt+1}/5 model loading (503)")
                time.sleep(10)
                continue
            log_event("warn", f"chat/completions attempt {attempt+1}/5 failed: {e}")
            time.sleep(15 * (attempt + 1))
    raise RuntimeError(f"policy inference failed after retries: {last_err}")


def env_reset() -> dict:
    body = {"run_id": RUN_ID}
    if DIFFICULTY:
        body["difficulty"] = DIFFICULTY
    r = session.post(f"{CONTROL_PLANE_URL}/api/rl/env/reset", json=body, timeout=30)
    r.raise_for_status()
    return r.json()


def env_step(sandbox_id: str, code: str, test_cases: list) -> dict:
    r = session.post(
        f"{CONTROL_PLANE_URL}/api/rl/env/step",
        json={"sandbox_id": sandbox_id, "code": code, "test_cases": test_cases},
        timeout=60,
    )
    r.raise_for_status()
    return r.json()


def env_close(sandbox_id: str):
    try:
        session.post(
            f"{CONTROL_PLANE_URL}/api/rl/env/close",
            json={"sandbox_id": sandbox_id}, timeout=10
        )
    except Exception:
        pass


def buffer_push(problem_id: str, prompt: str, code: str, reward: float, done: bool, step_n: int):
    session.post(
        f"{CONTROL_PLANE_URL}/api/rl/buffer/push",
        json={
            "run_id": RUN_ID,
            "problem_id": problem_id,
            "prompt": prompt,
            "code": code,
            "reward": reward,
            "done": done,
            "step_n": step_n,
        },
        timeout=10,
    )


def main():
    log.info("Worker %d starting. run_id=%s cp=%s", WORKER_INDEX, RUN_ID, CONTROL_PLANE_URL)
    log_event("info", f"worker {WORKER_INDEX} starting max_steps={MAX_STEPS}")
    report_status("running")

    policy_url = get_policy_server_url()
    log.info("Policy server: %s", policy_url)
    wait_for_policy_ready(policy_url)

    step = 0
    errors = 0
    refresh_policy = os.environ.get("REFRESH_POLICY_URL", "0") == "1"

    while MAX_STEPS == 0 or step < MAX_STEPS:
        sandbox_id = None
        try:
            if refresh_policy:
                policy_url = get_policy_server_url()
            # 1. Get a problem
            episode = env_reset()
            sandbox_id = episode["sandbox_id"]
            problem_id = episode["problem_id"]
            prompt = episode["prompt"]
            test_cases = episode.get("test_cases", [])
            log_event("info", f"step={step} env_reset problem={problem_id} sandbox={sandbox_id}")

            # 2. Generate code from policy
            code = generate_code(policy_url, prompt)

            # 3. Execute in sandbox, get reward
            result = env_step(sandbox_id, code, test_cases)
            reward = result.get("reward", 0.0)
            passed = result.get("passed_tests", 0)
            total = result.get("total_tests", 0)

            # 4. Push trajectory to buffer
            buffer_push(problem_id, prompt, code, reward, result.get("done", True), step)

            log.info("step=%d problem=%s reward=%.3f passed=%d/%d",
                     step, problem_id, reward, passed, total)
            log_event("info", f"step={step} pushed reward={reward:.3f} passed={passed}/{total}")
            step += 1
            errors = 0

        except Exception as e:
            log.warning("step=%d error: %s", step, e)
            log_event("error", f"step={step} failed: {e}")
            errors += 1
            if errors > 10:
                log.error("Too many consecutive errors, stopping worker")
                report_status("error", str(e))
                sys.exit(1)
            time.sleep(5)
        finally:
            if sandbox_id:
                env_close(sandbox_id)

    log.info("Worker %d finished after %d steps", WORKER_INDEX, step)
    report_status("completed")


if __name__ == "__main__":
    main()

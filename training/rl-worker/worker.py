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

session = requests.Session()
session.headers.update({"Content-Type": "application/json"})


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
                url = data.get("run", {}).get("policy_server_url", "")
                if url:
                    return url
        except Exception:
            pass
        log.info("Waiting for policy server to be ready... (%d/60)", attempt + 1)
        time.sleep(10)
    raise RuntimeError("Policy server URL not available after 10 minutes")


def generate_code(policy_url: str, prompt: str) -> str:
    """Call the vLLM OpenAI-compatible policy server to generate code."""
    system = (
        "You are an expert Python programmer. Write clean, correct Python code. "
        "Output ONLY the function implementation, no explanations, no markdown."
    )
    r = session.post(
        f"{policy_url}/v1/chat/completions",
        json={
            "model": os.environ.get("MODEL", "Qwen/Qwen3-0.6B"),
            "messages": [
                {"role": "system", "content": system},
                {"role": "user", "content": prompt},
            ],
            "temperature": 0.8,
            "max_tokens": 512,
        },
        timeout=60,
    )
    r.raise_for_status()
    choices = r.json().get("choices", [])
    return choices[0]["message"]["content"] if choices else ""


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
    log.info("Worker %d starting. run_id=%s", WORKER_INDEX, RUN_ID)
    report_status("running")

    policy_url = get_policy_server_url()
    log.info("Policy server: %s", policy_url)

    step = 0
    errors = 0

    while MAX_STEPS == 0 or step < MAX_STEPS:
        sandbox_id = None
        try:
            # 1. Get a problem
            episode = env_reset()
            sandbox_id = episode["sandbox_id"]
            problem_id = episode["problem_id"]
            prompt = episode["prompt"]
            test_cases = episode.get("test_cases", [])

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
            step += 1
            errors = 0

        except Exception as e:
            log.warning("step=%d error: %s", step, e)
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

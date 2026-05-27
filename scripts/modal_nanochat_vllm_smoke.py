#!/usr/bin/env python3
"""
Smoke test: load a model on Modal with vLLM and print sample chat completions.

Usage:
    python3 scripts/modal_nanochat_vllm_smoke.py
    python3 scripts/modal_nanochat_vllm_smoke.py --model karpathy/nanochat-d32
"""

from __future__ import annotations

import argparse
import json
import sys
import time

import modal
import requests

DEFAULT_MODEL = "sdobson/nanochat"
FALLBACK_MODEL = "karpathy/nanochat-d32"

PROMPTS = [
    "What is 17 + 28? Show your work.",
    "Solve: If a train travels 60 mph for 2.5 hours, how far does it go?",
    "What is the derivative of x^2 + 3x?",
    "Explain why the sky is blue in one sentence.",
    "Write a Python function to check if a number is prime.",
]

policy_image = modal.Image.from_registry(
    "vllm/vllm-openai:latest",
    add_python="3.11",
)


def wait_http(url: str, timeout: int = 900, interval: int = 5) -> tuple[bool, str]:
    deadline = time.time() + timeout
    last_err = ""
    attempt = 0
    while time.time() < deadline:
        attempt += 1
        try:
            resp = requests.get(url, timeout=15)
            if resp.status_code == 200:
                return True, f"ready after {attempt} attempts"
            last_err = f"HTTP {resp.status_code}"
        except Exception as e:
            last_err = str(e)
        if attempt == 1 or attempt % 12 == 0:
            print(f"  waiting for {url} ... attempt {attempt} ({last_err})", flush=True)
        time.sleep(interval)
    return False, f"timeout after {timeout}s ({last_err})"


def chat_complete(base_url: str, model: str, prompt: str, timeout: int = 120) -> dict:
    resp = requests.post(
        f"{base_url.rstrip('/')}/v1/chat/completions",
        json={
            "model": model,
            "messages": [{"role": "user", "content": prompt}],
            "temperature": 0.7,
            "max_tokens": 256,
        },
        timeout=timeout,
    )
    resp.raise_for_status()
    data = resp.json()
    content = data["choices"][0]["message"]["content"]
    usage = data.get("usage", {})
    return {
        "prompt": prompt,
        "response": content.strip(),
        "completion_tokens": usage.get("completion_tokens"),
        "prompt_tokens": usage.get("prompt_tokens"),
    }


def read_sandbox_logs(sb: modal.Sandbox, label: str, max_chars: int = 4000) -> str:
    chunks = []
    for stream, name in ((sb.stdout, "stdout"), (sb.stderr, "stderr")):
        try:
            text = stream.read()
            if text:
                chunks.append(f"--- {label} {name} ---\n{text[-max_chars:]}")
        except Exception as e:
            chunks.append(f"--- {label} {name} (read failed: {e}) ---")
    return "\n".join(chunks)


def run_vllm_smoke(model: str, gpu: str = "a10g") -> dict:
    app = modal.App.lookup("skyscale-nanochat-vllm-smoke", create_if_missing=True)
    print(f"\nLaunching vLLM on Modal (gpu={gpu}, model={model}) ...", flush=True)

    sb = modal.Sandbox.create(
        model,
        "--dtype",
        "float16",
        "--max-model-len",
        "2048",
        "--gpu-memory-utilization",
        "0.85",
        "--port",
        "8000",
        "--host",
        "0.0.0.0",
        "--trust-remote-code",
        app=app,
        image=policy_image,
        gpu=gpu,
        memory=24576,
        timeout=1200,
        encrypted_ports=[8000],
    )

    result = {
        "model": model,
        "gpu": gpu,
        "sandbox_id": sb.object_id,
        "load_ok": False,
        "samples": [],
        "error": "",
        "logs": "",
    }

    try:
        policy_url = sb.tunnels()[8000].url.rstrip("/")
        print(f"  tunnel: {policy_url}", flush=True)

        ok, detail = wait_http(f"{policy_url}/health", timeout=900)
        result["health_detail"] = detail
        if not ok:
            result["error"] = f"vLLM /health failed: {detail}"
            result["logs"] = read_sandbox_logs(sb, "vllm")
            return result

        result["load_ok"] = True
        print("  vLLM healthy — generating samples ...", flush=True)

        for i, prompt in enumerate(PROMPTS, 1):
            print(f"  [{i}/{len(PROMPTS)}] {prompt[:60]}...", flush=True)
            try:
                sample = chat_complete(policy_url, model, prompt)
                result["samples"].append(sample)
            except Exception as e:
                result["samples"].append(
                    {"prompt": prompt, "response": f"ERROR: {e}", "completion_tokens": None, "prompt_tokens": None}
                )
    except Exception as e:
        result["error"] = str(e)
        result["logs"] = read_sandbox_logs(sb, "vllm")
    finally:
        try:
            sb.terminate()
        except Exception:
            pass

    return result


def print_markdown_table(result: dict) -> None:
    print("\n## vLLM smoke test results\n")
    print(f"- **Model:** `{result['model']}`")
    print(f"- **GPU:** `{result.get('gpu', '?')}`")
    print(f"- **Load OK:** {result['load_ok']}")
    if result.get("error"):
        print(f"- **Error:** {result['error']}")
    print()

    if result["samples"]:
        print("| # | Prompt | Response | Tokens |")
        print("|---|--------|----------|--------|")
        for i, s in enumerate(result["samples"], 1):
            prompt = s["prompt"].replace("|", "\\|").replace("\n", " ")
            response = s["response"].replace("|", "\\|").replace("\n", " ")
            if len(response) > 120:
                response = response[:117] + "..."
            tokens = s.get("completion_tokens")
            tok = str(tokens) if tokens is not None else "—"
            print(f"| {i} | {prompt} | {response} | {tok} |")
    elif result.get("logs"):
        print("### Sandbox logs (tail)\n")
        print("```")
        print(result["logs"][-3000:])
        print("```")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--gpu", default="a10g")
    parser.add_argument("--fallback", action="store_true", help="Try HF nanochat if primary fails")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()

    result = run_vllm_smoke(args.model, gpu=args.gpu)

    if args.fallback and not result["load_ok"] and args.model == DEFAULT_MODEL:
        print(f"\nPrimary model failed — retrying with {FALLBACK_MODEL} ...", flush=True)
        fallback = run_vllm_smoke(FALLBACK_MODEL, gpu=args.gpu)
        fallback["primary_attempt"] = result
        result = fallback

    if args.json:
        print(json.dumps(result, indent=2))
    else:
        print_markdown_table(result)

    return 0 if result["load_ok"] and result["samples"] else 1


if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3
"""
Deploy Qwen3-0.6B on vLLM via Anyscale Services, run sample prompts, time them, then tear down.

Usage:
    export ANYSCALE_CLI_TOKEN=...
    python3 scripts/anyscale_bench.py
"""

import os, sys, time, requests

TOKEN    = os.environ["ANYSCALE_CLI_TOKEN"]
MODEL    = "Qwen/Qwen3-0.6B"
SVC_NAME = "vllm-bench-qwen3"

import anyscale
from anyscale.service.models import ServiceConfig, ServiceState
from anyscale.compute_config.models import ComputeConfig, HeadNodeConfig

# ── Config ────────────────────────────────────────────────────────────────────

# Ray Serve app that wraps vLLM's OpenAI server
SERVE_APP_SCRIPT = """\
import os
from ray import serve
from vllm.entrypoints.openai.api_server import build_app
from vllm.entrypoints.openai.cli_args import make_arg_parser

MODEL = os.environ.get("MODEL_NAME", "Qwen/Qwen3-0.6B")

parser = make_arg_parser()
args = parser.parse_args([
    "--model", MODEL,
    "--dtype", "float16",
    "--max-model-len", "4096",
    "--gpu-memory-utilization", "0.85",
])

router = build_app(args)

@serve.deployment(
    ray_actor_options={"num_gpus": 1},
    autoscaling_config={"min_replicas": 1, "max_replicas": 1},
)
@serve.ingress(router)
class VLLMApp:
    pass

app = VLLMApp.bind()
"""

# ── Benchmark prompts ─────────────────────────────────────────────────────────

PROMPTS = [
    {
        "name": "simple add",
        "messages": [
            {"role": "system", "content": "You are a Python expert. Output only code, no explanation."},
            {"role": "user",   "content": "Write a Python function `add(a: int, b: int) -> int` that returns the sum."},
        ]
    },
    {
        "name": "fizzbuzz",
        "messages": [
            {"role": "system", "content": "You are a Python expert. Output only code, no explanation."},
            {"role": "user",   "content": "Write a Python function `fizzbuzz(n: int) -> list` that returns FizzBuzz strings for 1..n."},
        ]
    },
    {
        "name": "binary search",
        "messages": [
            {"role": "system", "content": "You are a Python expert. Output only code, no explanation."},
            {"role": "user",   "content": "Write a Python function `binary_search(arr: list, target: int) -> int` returning index or -1."},
        ]
    },
    {
        "name": "merge sort",
        "messages": [
            {"role": "system", "content": "You are a Python expert. Output only code, no explanation."},
            {"role": "user",   "content": "Write a Python function `merge_sort(arr: list) -> list` using merge sort algorithm."},
        ]
    },
    {
        "name": "thinking mode",
        "messages": [
            {"role": "system", "content": "You are a Python expert."},
            {"role": "user",   "content": "/think\nWrite `is_palindrome(s: str) -> bool`. Think through edge cases first."},
        ]
    },
]

# ── Deploy ────────────────────────────────────────────────────────────────────

def deploy_service(client):
    print(f"🚀  Deploying {MODEL} on Anyscale (g5.xlarge A10G 24GB)...")

    # Write serve script to a temp file so we can pass it as working_dir
    import tempfile, pathlib
    tmpdir = tempfile.mkdtemp(prefix="anyscale_bench_")
    (pathlib.Path(tmpdir) / "serve_app.py").write_text(SERVE_APP_SCRIPT)

    # Build vLLM into the image so there's no runtime pip virtualenv.
    # Ray's runtime_env pip creates a virtualenv that leaks the base conda's
    # old scipy (no `Inf` compat), crashing vLLM on import.
    # Baking into the image installs everything in one consistent env.
    containerfile_contents = (
        "FROM anyscale/ray:2.55.1-py312-cu124\n"
        "RUN pip install --no-cache-dir 'vllm>=0.7.3' 'scipy>=1.13.0'\n"
    )

    print("    Building image with vLLM pre-installed (first time ~10 min, cached after)...")
    image_uri = client.image.build(
        name="anyscale-vllm-bench",
        containerfile=containerfile_contents,
        ray_version="2.55.1",
    )
    print(f"    image_uri={image_uri}")

    cfg = ServiceConfig(
        name=SVC_NAME,
        image_uri=image_uri,
        env_vars={"MODEL_NAME": MODEL},
        working_dir=tmpdir,
        compute_config=ComputeConfig(
            head_node=HeadNodeConfig(instance_type="g5.xlarge"),
        ),
        applications=[
            {
                "name": "vllm",
                "import_path": "serve_app:app",
                "route_prefix": "/",
            }
        ],
        query_auth_token_enabled=False,
    )

    client.service.deploy(cfg, name=SVC_NAME)
    print(f"    service name: {SVC_NAME}")
    return SVC_NAME


def wait_for_service(client, svc_name, timeout=900):
    print("⏳  Waiting for RUNNING (model load ~3-5 min)...")
    deadline = time.time() + timeout
    last_state = ""
    while time.time() < deadline:
        try:
            status = client.service.status(svc_name)
            state  = str(status.state)
            url    = status.query_url or ""
            if state != last_state:
                print(f"    state={state}  url={url or '(pending)'}")
                last_state = state
            if status.state == ServiceState.RUNNING and url:
                print(f"\n    ✓ Service ready: {url}")
                return url.rstrip("/")
            if status.state in (ServiceState.TERMINATED, ServiceState.UNHEALTHY):
                raise RuntimeError(f"Service entered state: {state}")
        except Exception as e:
            if "RUNNING" not in str(e) and "TERMINATED" not in str(e) and "UNHEALTHY" not in str(e):
                print(f"    poll error: {e}")
        time.sleep(10)
    raise RuntimeError("Service did not reach RUNNING within timeout")


def wait_for_health(base_url, timeout=120):
    print("⏳  Checking vLLM /health...")
    deadline = time.time() + timeout
    while time.time() < deadline:
        for path in ["/health", "/v1/models"]:
            try:
                r = requests.get(f"{base_url}{path}", timeout=5)
                if r.ok:
                    print(f"    ✓ {path} OK")
                    return
            except Exception:
                pass
        time.sleep(5)
    print("    ⚠ health check timed out, proceeding anyway")


# ── Benchmark ─────────────────────────────────────────────────────────────────

def run_benchmark(base_url):
    print(f"\n{'='*60}")
    print(f"BENCHMARK: {MODEL} on Anyscale g5.xlarge (A10G 24GB)")
    print(f"Endpoint:  {base_url}")
    print(f"{'='*60}\n")

    results = []
    session = requests.Session()

    for p in PROMPTS:
        print(f"→ [{p['name']}]")
        payload = {
            "model":       MODEL,
            "messages":    p["messages"],
            "max_tokens":  256,
            "temperature": 0.0,
        }
        t0 = time.time()
        try:
            r = session.post(f"{base_url}/v1/chat/completions", json=payload, timeout=60)
            r.raise_for_status()
            elapsed = time.time() - t0
            data    = r.json()
            choice  = data["choices"][0]
            text    = choice["message"]["content"].strip()
            usage   = data.get("usage", {})
            in_tok  = usage.get("prompt_tokens", 0)
            out_tok = usage.get("completion_tokens", 0)
            tps     = out_tok / elapsed if elapsed > 0 else 0
            results.append({"name": p["name"], "elapsed": elapsed,
                            "in_tokens": in_tok, "out_tokens": out_tok, "tps": tps})
            preview = text[:200].replace('\n', ' ')
            print(f"   time:   {elapsed:.2f}s")
            print(f"   tokens: {in_tok} in → {out_tok} out  ({tps:.1f} tok/s)")
            print(f"   output: {preview}{'...' if len(text)>200 else ''}")
        except Exception as e:
            print(f"   ERROR: {e}")
        print()

    print(f"{'='*60}")
    print(f"{'Prompt':<28} {'Time':>6} {'Out tok':>8} {'Tok/s':>8}")
    print(f"{'-'*60}")
    for r in results:
        print(f"{r['name']:<28} {r['elapsed']:>5.2f}s {r['out_tokens']:>8} {r['tps']:>7.1f}")
    if results:
        avg_tps = sum(r['tps'] for r in results) / len(results)
        avg_lat = sum(r['elapsed'] for r in results) / len(results)
        print(f"{'-'*60}")
        print(f"{'Average':<28} {avg_lat:>5.2f}s {'':>8} {avg_tps:>7.1f}")
    print(f"{'='*60}")
    return results


# ── Main ──────────────────────────────────────────────────────────────────────

def main():
    client   = anyscale.Anyscale(auth_token=TOKEN)
    svc_name = None
    start    = time.time()
    try:
        svc_name = deploy_service(client)
        url      = wait_for_service(client, svc_name)
        wait_for_health(url)

        print(f"\n⏱  Total deploy+load time: {time.time()-start:.0f}s")
        run_benchmark(url)

    except KeyboardInterrupt:
        print("\n⚠️  Interrupted")
        if svc_name:
            print(f"\n🧹  Terminating service '{svc_name}'...")
            try:
                client.service.terminate(name=svc_name)
                print("    Done.")
            except Exception as e:
                print(f"    Terminate error: {e}")
    except Exception as e:
        print(f"\n❌  Error: {e}")
        import traceback; traceback.print_exc()
        print(f"\n💡  Service '{svc_name}' left running. Re-run to resume benchmark.")


if __name__ == "__main__":
    main()

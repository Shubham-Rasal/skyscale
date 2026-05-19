#!/usr/bin/env python3
"""
Deploy Qwen3-0.6B on vLLM to an Akash GPU, run sample prompts, time them, then tear down.

Usage:
    export AKASH_API_KEY=...
    python3 scripts/vllm_bench.py
"""

import os, sys, time, json, requests

# ── Config ──────────────────────────────────────────────────────────────────

API_KEY    = os.environ["AKASH_API_KEY"]
BASE       = os.environ.get("AKASH_CONSOLE_URL", "https://console-api.akash.network")
MODEL      = "Qwen/Qwen3-0.6B"
GPU_MODEL  = "rtx4070"    # 12GB — cheapest that fits; fallback list below
DEPOSIT    = 5.0          # USD escrow

HEADERS = {"Content-Type": "application/json", "x-api-key": API_KEY}

# ── SDL ──────────────────────────────────────────────────────────────────────

SDL = f"""
version: "2.0"
services:
  vllm:
    image: vllm/vllm-openai:v0.6.6
    args:
      - "--model"
      - "{MODEL}"
      - "--port"
      - "8000"
      - "--dtype"
      - "float16"
      - "--max-model-len"
      - "4096"
      - "--gpu-memory-utilization"
      - "0.85"
    env:
      - HF_HUB_ENABLE_HF_TRANSFER=1
      - HUGGING_FACE_HUB_TOKEN=${{HF_TOKEN}}
    expose:
      - port: 8000
        as: 8000
        proto: tcp
        to:
          - global: true
profiles:
  compute:
    vllm:
      resources:
        gpu:
          units: 1
          attributes:
            vendor:
              nvidia:
                - model: a100
        cpu:
          units: 4
        memory:
          size: 16Gi
        storage:
          - size: 30Gi
  placement:
    akash:
      attributes:
        region: us-central
      pricing:
        vllm:
          denom: uakt
          amount: 10000000
deployment:
  vllm:
    akash:
      profile: vllm
      count: 1
"""

# ── Akash API helpers ─────────────────────────────────────────────────────────

def api_post(path, body):
    r = requests.post(BASE + path, headers=HEADERS, json=body, timeout=30)
    if not r.ok:
        print(f"    API error {r.status_code}: {r.text[:500]}")
    r.raise_for_status()
    return r.json()

def api_get(path):
    r = requests.get(BASE + path, headers=HEADERS, timeout=30)
    r.raise_for_status()
    return r.json()

def api_delete(path):
    r = requests.delete(BASE + path, headers=HEADERS, timeout=30)
    # 404 is fine — already gone
    return r.status_code

# ── Deploy ────────────────────────────────────────────────────────────────────

def create_deployment():
    print("📦  Creating Akash deployment...")
    resp = api_post("/v1/deployments", {"data": {"sdl": SDL, "deposit": DEPOSIT}})
    dseq     = resp["data"]["dseq"]
    manifest = resp["data"]["manifest"]
    print(f"    dseq={dseq}")
    return dseq, manifest

def wait_for_bid(dseq, timeout=300):
    print("⏳  Waiting for provider bids...")
    bad = {"akash1sevd2ymtty3dpq9ycxgkhuzzk4fe6mchqdwd4e",
           "akash18ga02jzaq8cw52anyhzkwta5wygufgu6zsz6xc"}
    deadline = time.time() + timeout
    best = None
    collect_until = time.time() + 90   # collect bids for 90s, then pick cheapest
    while time.time() < deadline:
        resp = api_get(f"/v1/bids?dseq={dseq}")
        for item in resp.get("data", []):
            b = item["bid"]
            if b["state"] not in ("open", "active"):
                continue
            if b["id"]["provider"] in bad:
                print(f"    skipping bad provider {b['id']['provider'][:20]}...")
                continue
            if best is None:
                best = b
                print(f"    bid from {b['id']['provider'][:24]}  price={b['price']['amount']} {b['price']['denom']}")
        if best and time.time() > collect_until:
            return best
        time.sleep(3)
    if best:
        return best
    raise RuntimeError(f"No acceptable bids within {timeout}s")

def create_lease(dseq, bid, manifest):
    print("🔗  Accepting bid and creating lease...")
    leases = [{"dseq": dseq, "gseq": bid["id"]["gseq"],
               "oseq": bid["id"]["oseq"], "provider": bid["id"]["provider"]}]
    resp = api_post("/v1/leases", {"manifest": manifest, "leases": leases})
    print("    Lease created — container starting")
    return bid["id"]["gseq"], bid["id"]["oseq"], bid["id"]["provider"]

def wait_for_endpoint(dseq, timeout=600):
    """Poll deployment status until vllm service has a forwarded port."""
    print("⏳  Waiting for vLLM container to start (this takes 3-5 min for image pull)...")
    deadline = time.time() + timeout
    last_msg = ""
    while time.time() < deadline:
        try:
            resp = api_get(f"/v1/deployments/{dseq}")
            data = resp.get("data", resp)
            leases = data.get("leases", [])
            if leases:
                svc_map = leases[0].get("status", {}).get("services", {})
                for svc_name, svc in svc_map.items():
                    fps = svc.get("forwardedPorts", [])
                    for fp in fps:
                        if fp["port"] == 8000:
                            host = fp["host"]
                            ext  = fp["externalPort"]
                            url  = f"http://{host}:{ext}"
                            print(f"\n    ✓ endpoint: {url}")
                            return url
                    # also try URIs
                    for uri in svc.get("uris", []):
                        url = uri if uri.startswith("http") else f"http://{uri}"
                        print(f"\n    ✓ URI endpoint: {url}")
                        return url
                msg = f"    services: {list(svc_map.keys()) or 'starting...'}"
            else:
                msg = "    waiting for lease to activate..."
            if msg != last_msg:
                print(msg)
                last_msg = msg
        except Exception as e:
            print(f"    poll error: {e}")
        time.sleep(10)
    raise RuntimeError("Container did not expose endpoint within timeout")

def wait_for_health(base_url, timeout=120):
    print("⏳  Waiting for vLLM /health...")
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            r = requests.get(f"{base_url}/health", timeout=5)
            if r.status_code == 200:
                print("    ✓ vLLM is healthy")
                return
        except Exception:
            pass
        time.sleep(5)
    raise RuntimeError("vLLM health check timed out")

# ── Benchmark ─────────────────────────────────────────────────────────────────

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
        "name": "thinking mode (palindrome)",
        "messages": [
            {"role": "system", "content": "You are a Python expert."},
            {"role": "user",   "content": "/think\nWrite a Python function `is_palindrome(s: str) -> bool`. Think through edge cases first."},
        ]
    },
]

def run_benchmark(base_url):
    print(f"\n{'='*60}")
    print(f"BENCHMARK: {MODEL} on Akash")
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
            "temperature": 0.0,   # greedy for reproducibility
        }

        t0 = time.time()
        try:
            r = session.post(
                f"{base_url}/v1/chat/completions",
                json=payload,
                timeout=60,
            )
            r.raise_for_status()
            elapsed = time.time() - t0
            data = r.json()

            choice    = data["choices"][0]
            text      = choice["message"]["content"].strip()
            usage     = data.get("usage", {})
            in_tok    = usage.get("prompt_tokens", 0)
            out_tok   = usage.get("completion_tokens", 0)
            tps       = out_tok / elapsed if elapsed > 0 else 0

            results.append({
                "name": p["name"], "elapsed": elapsed,
                "in_tokens": in_tok, "out_tokens": out_tok, "tps": tps,
            })

            # Print truncated output
            preview = text[:200].replace('\n', ' ')
            print(f"   time:    {elapsed:.2f}s")
            print(f"   tokens:  {in_tok} in → {out_tok} out  ({tps:.1f} tok/s)")
            print(f"   output:  {preview}{'...' if len(text)>200 else ''}")

        except Exception as e:
            print(f"   ERROR: {e}")
        print()

    # Summary table
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
    dseq = None
    start = time.time()
    try:
        dseq, manifest = create_deployment()
        bid = wait_for_bid(dseq)
        gseq, oseq, provider = create_lease(dseq, bid, manifest)

        endpoint = wait_for_endpoint(dseq)
        wait_for_health(endpoint)

        deploy_time = time.time() - start
        print(f"\n⏱  Total deploy time: {deploy_time:.0f}s")

        run_benchmark(endpoint)

    except KeyboardInterrupt:
        print("\n⚠️  Interrupted")
    except Exception as e:
        print(f"\n❌  Error: {e}")
        import traceback; traceback.print_exc()
    finally:
        if dseq:
            print(f"\n🧹  Closing deployment {dseq}...")
            api_delete(f"/v1/deployments/{dseq}")
            print("    Done.")

if __name__ == "__main__":
    main()

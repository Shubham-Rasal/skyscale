"""
Policy server: vLLM-backed inference endpoint for RL rollout workers.

Environment variables:
  MODEL_NAME         HuggingFace model ID (default: Qwen/Qwen2.5-Coder-1.5B-Instruct)
  RUN_ID             RL run ID
  CONTROL_PLANE_URL  Skyscale control plane URL
  EXECUTION_ID       Execution ID to report status
  PORT               HTTP port (default: 8000)
"""
import os
import sys
import time
import threading
import requests
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List, Optional

MODEL_NAME = os.environ.get("MODEL_NAME", "Qwen/Qwen3-0.6B")
RUN_ID = os.environ.get("RUN_ID", "local")
CONTROL_PLANE_URL = os.environ.get("CONTROL_PLANE_URL", "http://localhost:8080")
EXECUTION_ID = os.environ.get("EXECUTION_ID", RUN_ID)
PORT = int(os.environ.get("PORT", "8000"))

app = FastAPI(title="Skyscale Policy Server")

_llm = None
_llm_lock = threading.Lock()
_llm_ready = False
_llm_error: Optional[str] = None


def load_llm():
    """Load vLLM synchronously before serving traffic."""
    global _llm, _llm_ready, _llm_error
    try:
        print(f"[policy] loading model {MODEL_NAME}...", flush=True)
        from vllm import LLM
        _llm = LLM(model=MODEL_NAME, dtype="float16", max_model_len=2048)
        _llm_ready = True
        print("[policy] model loaded", flush=True)
    except Exception as e:
        _llm_error = str(e)
        print(f"[policy] model load failed: {_llm_error}", flush=True)


def get_llm():
    if _llm_error:
        raise RuntimeError(f"vLLM failed to load: {_llm_error}")
    if not _llm_ready:
        raise RuntimeError("vLLM is not ready")
    return _llm


class GenerateRequest(BaseModel):
    prompts: List[str]
    temperature: float = 0.8
    max_tokens: int = 512
    top_p: float = 0.95


class GenerateResponse(BaseModel):
    completions: List[str]
    model: str


class ReloadRequest(BaseModel):
    checkpoint_url: str


@app.post("/generate", response_model=GenerateResponse)
async def generate(req: GenerateRequest):
    if not _llm_ready:
        raise HTTPException(status_code=503, detail="model loading")
    from vllm import SamplingParams
    llm = get_llm()
    params = SamplingParams(
        temperature=req.temperature,
        max_tokens=req.max_tokens,
        top_p=req.top_p,
    )
    outputs = llm.generate(req.prompts, params)
    completions = [o.outputs[0].text for o in outputs]
    return GenerateResponse(completions=completions, model=MODEL_NAME)


@app.post("/reload")
async def reload_weights(req: ReloadRequest):
    """Hot-swap model weights from a checkpoint URL (artifact store)."""
    import tempfile
    import urllib.request

    try:
        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
            urllib.request.urlretrieve(req.checkpoint_url, f.name)
            checkpoint_path = f.name
        with _llm_lock:
            llm = get_llm()
            llm.load_weights(checkpoint_path)
        try:
            requests.post(
                f"{CONTROL_PLANE_URL}/api/rl/runs/{RUN_ID}/events",
                json={
                    "component": "policy",
                    "level": "info",
                    "message": f"weights loaded from {req.checkpoint_url}",
                },
                timeout=5,
            )
        except Exception:
            pass
        return {"ok": True, "checkpoint": req.checkpoint_url}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/health")
async def health():
    return {
        "ok": _llm_ready and _llm_error is None,
        "ready": _llm_ready,
        "model": MODEL_NAME,
        "run_id": RUN_ID,
        "error": _llm_error,
    }


@app.get("/ready")
async def ready():
    if not _llm_ready:
        raise HTTPException(status_code=503, detail=_llm_error or "model loading")
    return {"ok": True, "model": MODEL_NAME}


def _report_status(status: str, error: str = ""):
    try:
        body = {"status": status}
        if error:
            body["error"] = error
        requests.post(
            f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/complete",
            json=body,
            timeout=5,
        )
    except Exception:
        pass


if __name__ == "__main__":
    _report_status("running")
    # Bind HTTP immediately so Modal tunnel stays reachable while vLLM loads.
    server = threading.Thread(
        target=lambda: uvicorn.run(app, host="0.0.0.0", port=PORT, log_level="info"),
        daemon=True,
    )
    server.start()
    for _ in range(30):
        try:
            requests.get(f"http://127.0.0.1:{PORT}/health", timeout=1)
            break
        except Exception:
            time.sleep(0.5)
    load_llm()
    if _llm_error:
        _report_status("error", _llm_error)
        sys.exit(1)
    while server.is_alive():
        time.sleep(60)

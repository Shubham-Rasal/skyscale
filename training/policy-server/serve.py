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

# Lazy-load vLLM so the server starts fast and we can report "starting" status
_llm = None
_llm_lock = threading.Lock()


def get_llm():
    global _llm
    if _llm is None:
        with _llm_lock:
            if _llm is None:
                from vllm import LLM
                _llm = LLM(model=MODEL_NAME, dtype="float16", max_model_len=2048)
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
    global _llm
    import tempfile, urllib.request
    try:
        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
            urllib.request.urlretrieve(req.checkpoint_url, f.name)
            checkpoint_path = f.name
        with _llm_lock:
            llm = get_llm()
            llm.load_weights(checkpoint_path)
        return {"ok": True, "checkpoint": req.checkpoint_url}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/health")
async def health():
    return {"ok": True, "model": MODEL_NAME, "run_id": RUN_ID}


def _report_status(status: str, error: str = ""):
    """Report execution status back to control plane."""
    try:
        body = {"status": status}
        if error:
            body["error"] = error
        requests.post(
            f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/complete",
            json=body, timeout=5
        )
    except Exception:
        pass


if __name__ == "__main__":
    # Report that we're starting
    _report_status("running")
    try:
        uvicorn.run(app, host="0.0.0.0", port=PORT)
    except Exception as e:
        _report_status("error", str(e))
        raise

"""
Policy server: vLLM-backed inference with /reload and OpenAI-compatible chat API.

Supports CHECKPOINT_URL to load fine-tuned weights at startup (closed-loop redeploy).
"""
import os
import sys
import time
import threading
import tempfile
import urllib.request
import requests
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List, Optional

MODEL_NAME = os.environ.get("MODEL_NAME", "Qwen/Qwen3-0.6B")
CHECKPOINT_URL = os.environ.get("CHECKPOINT_URL", "")
RUN_ID = os.environ.get("RUN_ID", "local")
CONTROL_PLANE_URL = os.environ.get("CONTROL_PLANE_URL", "http://localhost:8080")
EXECUTION_ID = os.environ.get("EXECUTION_ID", RUN_ID)
PORT = int(os.environ.get("PORT", "8000"))

app = FastAPI(title="Skyscale Policy Server")

_llm = None
_llm_lock = threading.Lock()
_llm_ready = False
_llm_error: Optional[str] = None
_loaded_model = MODEL_NAME


def _download_checkpoint(url: str) -> str:
    with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
        urllib.request.urlretrieve(url, f.name)
        return f.name


def _apply_checkpoint_to_hf(model_name: str, checkpoint_path: str) -> str:
    """Merge HF state_dict checkpoint into a temp model dir for vLLM."""
    import torch
    from transformers import AutoModelForCausalLM, AutoTokenizer

    tmp = tempfile.mkdtemp(prefix="skyscale-policy-")
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    model = AutoModelForCausalLM.from_pretrained(model_name, torch_dtype="auto")
    state = torch.load(checkpoint_path, map_location="cpu")
    model.load_state_dict(state, strict=False)
    model.save_pretrained(tmp)
    tokenizer.save_pretrained(tmp)
    return tmp


def load_llm():
    """Load vLLM synchronously after HTTP server is up."""
    global _llm, _llm_ready, _llm_error, _loaded_model
    try:
        model_path = MODEL_NAME
        if CHECKPOINT_URL:
            print(f"[policy] downloading checkpoint {CHECKPOINT_URL}...", flush=True)
            ckpt = _download_checkpoint(CHECKPOINT_URL)
            print("[policy] merging checkpoint into base model...", flush=True)
            model_path = _apply_checkpoint_to_hf(MODEL_NAME, ckpt)
            _loaded_model = model_path
        print(f"[policy] loading vLLM model {model_path}...", flush=True)
        from vllm import LLM
        _llm = LLM(model=model_path, dtype="float16", max_model_len=2048)
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


class ChatMessage(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    model: str = MODEL_NAME
    messages: List[ChatMessage]
    temperature: float = 0.8
    max_tokens: int = 512


@app.post("/v1/chat/completions")
async def chat_completions(req: ChatRequest):
    if not _llm_ready:
        raise HTTPException(status_code=503, detail="model loading")
    from vllm import SamplingParams

    llm = get_llm()
    parts = []
    for m in req.messages:
        parts.append(m.content)
    prompt = "\n".join(parts)
    params = SamplingParams(
        temperature=req.temperature,
        max_tokens=req.max_tokens,
    )
    outputs = llm.generate([prompt], params)
    text = outputs[0].outputs[0].text
    return {
        "id": "skyscale-policy",
        "object": "chat.completion",
        "model": req.model or MODEL_NAME,
        "choices": [{"index": 0, "message": {"role": "assistant", "content": text}, "finish_reason": "stop"}],
    }


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
    global _loaded_model
    try:
        ckpt = _download_checkpoint(req.checkpoint_url)
        model_path = _apply_checkpoint_to_hf(MODEL_NAME, ckpt)
        with _llm_lock:
            from vllm import LLM
            llm = LLM(model=model_path, dtype="float16", max_model_len=2048)
            global _llm
            _llm = llm
            _loaded_model = model_path
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
        "model": _loaded_model,
        "run_id": RUN_ID,
        "error": _llm_error,
    }


@app.get("/ready")
async def ready():
    if not _llm_ready:
        raise HTTPException(status_code=503, detail=_llm_error or "model loading")
    return {"ok": True, "model": _loaded_model}


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
    server = threading.Thread(
        target=lambda: uvicorn.run(app, host="0.0.0.0", port=PORT, log_level="info"),
        daemon=True,
    )
    server.start()
    for _ in range(60):
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

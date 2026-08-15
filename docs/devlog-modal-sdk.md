# Building a Modal-style Serverless SDK from Scratch (Because I'm Applying to Modal)

*May 2026*

---

I'm applying for an engineering role at [Modal](https://modal.com). If you haven't used Modal before, the pitch is simple: you write Python, you stick a decorator on a function, and it runs in the cloud — on GPUs, at scale, with zero infrastructure config. No Dockerfiles, no Kubernetes, no YAML.

It's genuinely one of the most elegant developer experiences I've used. So when I started thinking about what to build for my application, the answer was obvious: build a version of it myself. Not to compete with Modal — that would be absurd — but to prove I understand how it works from the inside out.

I already had a project called **Skyscale**, a serverless platform I'd been building on top of Firecracker microVMs and Akash Network for GPU workloads. It had a working control plane written in Go, a scheduler, VM lifecycle management, and a sandbox API. What it didn't have was the thing that makes Modal *Modal*: the decorator-based Python SDK that makes remote execution feel like local function calls.

So that's what I built this week.

---

## What I Started With

Skyscale already had decent bones. The control plane could:

- Spin up Firecracker microVMs from a warm pool
- Route GPU jobs to Akash Network deployments
- Run one-shot Python functions inside VMs via a Go daemon
- Expose a REST Sandbox API for AI agent workloads

But the developer experience was Lambda-style: you deployed a directory with a `handler.py`, a `requirements.txt`, and a `skyscale.yaml`, and invoked it by name via curl or the CLI. Fine, but not *Modal*.

The other thing that bothered me: CPU (Firecracker) and GPU (Akash) were completely separate code paths. The scheduler had `if hwType == "gpu" { ... } else { ... }` branching everywhere. That had to go.

---

## The Goal

I wanted this to work:

```python
import skyscale

app = skyscale.App("text-analyzer")

@app.cls()
class ReadabilityScorer:

    @skyscale.enter()
    def setup(self):
        self.calls = 0
        import re
        self._word_re = re.compile(r"\b\w+\b")

    @skyscale.web_endpoint(method="POST", path="/readability")
    def score(self, text: str) -> dict:
        self.calls += 1
        words = self._word_re.findall(text.lower())
        # ... compute Flesch Reading Ease ...
        return {"score": fre, "container_call_count": self.calls}
```

And then:

```
$ skyscale deploy text_analysis.py
  ✓  ReadabilityScorer.score
     http://127.0.0.1:8080/proxy/text-analyzer--readabilityscorer-score/readability

✅ App deployed.
```

And that URL should just... work. Like Modal.

---

## Layer 1: The Python SDK

The SDK lives in `sdk/python/skyscale/`. It's four files.

**`app.py`** defines the `App` class with `@app.function()` and `@app.cls()` decorators. Both return handle objects (`FunctionHandle` and `ClsHandle`) that wrap the original function or class and carry metadata — image, GPU model, memory, scaledown window. The `App` also has a `web_endpoints()` iterator that walks all registered functions and classes and yields the ones decorated with `@skyscale.web_endpoint`.

**`image.py`** defines `Image`, a fluent builder:

```python
image = skyscale.Image.pip_install("pandas", "numpy").apt_install("libgomp1")
```

Internally it's just building a list of pip packages, apt packages, and shell commands. When the CLI serializes a deployment, `image.to_requirements_txt()` and `image.to_setup_script()` turn those lists into strings the daemon can consume.

**`decorators.py`** defines `@enter()`, `@web_endpoint()`, and `@method()`. They're just small functions that stamp metadata onto the decorated function as attributes:

```python
def web_endpoint(method="POST", path="/"):
    def decorator(fn):
        fn._web_endpoint = {"method": method.upper(), "path": path}
        return fn
    return decorator
```

Simple. The `App` looks for `_web_endpoint` attributes when building the deployment spec.

**`_deploy.py`** is the interesting one. It's the module the CLI calls to turn a user's `.py` file into a JSON `DeploymentSpec`. When you run `skyscale deploy app.py`, the CLI does roughly:

```
python3 -m skyscale._deploy app.py
```

Which imports the user's file, finds the `App` instance, iterates `web_endpoints()`, and prints a JSON list of specs — one per endpoint. Each spec contains the function's entry point, image requirements, hardware target (cpu/gpu), and web metadata (method, path).

The tricky part here was the *code*. I need to send the handler code to the daemon so it can write it to disk inside the VM and run it. My first instinct was `inspect.getsource()`. That works fine locally, but when you load a module via `importlib.util.module_from_spec()` and `exec_module()` (which is what you have to do to load an arbitrary file path), Python 3.12 raises `TypeError: is a built-in class` when you call `inspect.getsource()` on classes defined in that module. The module is "synthetic" from the interpreter's perspective.

The fix was simpler than I expected: just read the raw file source before loading the module, and send the whole thing. Every endpoint in the same file gets the same `code` payload — the entire file content, stripped of SDK-specific lines. This is actually *more correct* anyway, because functions often depend on module-level imports and helpers that `getsource()` on a single function wouldn't capture.

The stripping function removes:

- `import skyscale` / `from skyscale import ...`
- `@app.function()`, `@app.cls()`, `@skyscale.web_endpoint()`, `@skyscale.enter()`
- `app = skyscale.App(...)` and `image = skyscale.Image...` (unindented SDK construction lines)

What's left is pure Python that the daemon can run without the SDK installed.

---

## Layer 2: Unified Compute Routing

Before this, `GetVM()` gave you a Firecracker VM and `GetAkashVM()` gave you a GPU container on Akash, and those were two completely separate paths wired separately through the scheduler.

I collapsed them into one:

```go
func (m *VMManager) GetCompute(req ComputeRequest) (*state.VM, error) {
    if req.HardwareType == "gpu" {
        return m.GetAkashVM(req.JobID, req.ExecutionID,
                            req.GPUModel, req.DockerImage, req.ControlPlaneURL)
    }
    return m.GetVM()
}

func (m *VMManager) ReleaseCompute(vmID string) error {
    vm, _ := m.stateManager.GetVM(vmID)
    if vm.HardwareType == "gpu" {
        return m.ReturnAkashVM(vmID)
    }
    return m.ReturnVM(vmID)
}
```

Everything in the deployment layer now calls `GetCompute` and `ReleaseCompute`. The scheduler still uses the old paths for legacy one-shot FaaS invocations, but new deployments go through the unified interface. From a user's perspective, you just write `gpu="a100"` in your decorator and the platform routes accordingly.

I also added a test-mode fallback: if `GetVM()` fails (e.g., no Firecracker rootfs on the machine) and `DAEMON_PATH` is set (which indicates we're in local test mode), it falls back to the test host VM at `127.0.0.1`. This let me test the whole deploy flow on a plain Linux VPS without real Firecracker images.

---

## Layer 3: The Deployment Manager

This is the biggest new Go component. The existing system only had the concept of *Executions* — one-shot function runs that live for milliseconds. A web endpoint is fundamentally different: it's a **long-running process** that stays alive across many requests.

I added a `Deployment` model to the state layer:

```go
type Deployment struct {
    ID              string
    Slug            string  // "demo-web--process"
    AppName         string
    EndpointName    string
    VMID            string
    VMPort          int     // port uvicorn listens on inside the VM
    WebPath         string
    WebMethod       string
    URL             string
    Status          string
    HardwareType    string
    ScaledownWindow int
    LastRequestAt   time.Time
    CreatedAt       time.Time
}
```

The `DeploymentManager.Deploy()` method:

1. Calls `GetCompute` to acquire a VM
2. Sends a `ServeRequest` to the daemon's new `POST /serve` endpoint
3. Waits for `{"port": 9000, "status": "ready"}`
4. Persists the `Deployment` record with the VM IP and port
5. Returns the public URL

Then `ProxyRequest()` handles incoming traffic. When a request hits `/proxy/demo-web--process/process`, the manager looks up the deployment by slug, finds the VM, and reverse-proxies using Go's `httputil.ReverseProxy`.

One bug I hit: I initially hardcoded port 9000 for every deployment. The first deploy works, the second one's uvicorn silently fails to bind, and you end up with multiple deployments all pointing at port 9000 — which only has the first deployment's routes. I fixed this with an `atomic.Int32` counter on the manager, seeded from the highest `vm_port` already in the DB on startup (so a control plane restart doesn't re-use ports that running uvicorn processes still hold):

```go
func (m *Manager) allocatePort() int {
    return int(m.nextPort.Add(1))
}
```

---

## Layer 4: The Daemon's `/serve` Endpoint

The daemon is a Go binary that runs inside each VM (or on the host in test mode). It already handled one-shot function execution via `POST /execute`. I added `POST /serve` in a new `serve.go` file.

The flow:

1. Write `handler.py` (the stripped user code) and `requirements.txt` to `/tmp/faas/code/skyscale_serve/`
2. Create a Python venv if one doesn't exist, run `pip install`
3. Generate `server.py` — a FastAPI app that wraps the user's function or class
4. Start `uvicorn server:app --host 0.0.0.0 --port <N>`
5. Poll `127.0.0.1:<N>` with `net.DialTimeout` until it accepts connections (max 120s)
6. Respond `{"port": N, "status": "ready"}`

The `generateServerWrapper` function is where the two entry types diverge. For a plain `@app.function`:

```python
from fastapi import FastAPI, Body
from handler import process as _user_fn

app = FastAPI()

@app.post("/process")
def endpoint(payload: dict = Body(...)):
    return _user_fn(**payload)
```

For `@app.cls` with an `@enter()` hook:

```python
from fastapi import FastAPI, Body
from handler import ReadabilityScorer

app = FastAPI()

_instance = ReadabilityScorer()
_instance.setup()   # @enter() hook

@app.post("/readability")
def endpoint(payload: dict = Body(...)):
    return _instance.score(**payload)
```

The class instance is module-level in the generated `server.py`, so it lives for the entire uvicorn process lifetime. State set in `setup()` persists across every request that process handles. That's the `@enter()` semantics: runs once on container boot, not once per request.

---

## Testing It

I deployed to a Linux VPS (`n8n.maximalstudio.in` — a server I already had lying around). The first example was `skyscale_web_app.py`: a `process` function that wraps `pd.DataFrame(data).describe()` and returns the stats.

```
$ skyscale deploy skyscale_web_app.py
  ✓  process
     http://127.0.0.1:8080/proxy/demo-web--process/process

✅ App deployed.

$ curl -X POST .../proxy/demo-web--process/process \
    -d '{"data": {"col1": [1,2,3,4,5], "col2": [10,20,30,40,50]}}'

{
  "result": {
    "col1": {"count": 5.0, "mean": 3.0, "std": 1.58, ...},
    "col2": {"count": 5.0, "mean": 30.0, "std": 15.81, ...}
  }
}
```

Worked first try. Subsequent requests came back in ~16ms. No cold start after the first one.

Then I wrote `text_analysis.py` — a more interesting example with two endpoints, including the stateful `ReadabilityScorer` class. That one surfaced three bugs in a row:

**Bug 1:** `inspect.getsource()` raising `TypeError` on the importlib-loaded class. Fixed by reading the raw file source before loading the module.

**Bug 2:** `import skyscale` left in the stripped `handler.py`, causing `ModuleNotFoundError` in the daemon venv. Fixed by adding import stripping to `_strip_skyscale_decorators`.

**Bug 3:** `app = skyscale.App("text-analyzer")` also left in `handler.py`, causing `NameError: name 'skyscale' is not defined`. Fixed by stripping unindented lines containing `skyscale.App(` or `skyscale.Image`.

After those three fixes, the deploy worked:

```
$ skyscale deploy text_analysis.py
  ✓  analyze
     .../proxy/text-analyzer--analyze/analyze
  ✓  ReadabilityScorer.score
     .../proxy/text-analyzer--readabilityscorer-score/readability

✅ App deployed.
```

And hitting the readability endpoint three times in a row:

```json
{"flesch_reading_ease": 73.5, "grade_level": "Fairly Easy (7th grade)", "container_call_count": 1}
{"flesch_reading_ease": 73.5, "grade_level": "Fairly Easy (7th grade)", "container_call_count": 2}
{"flesch_reading_ease": 73.5, "grade_level": "Fairly Easy (7th grade)", "container_call_count": 3}
```

`container_call_count` ticking up proves the instance is alive and stateful between requests. The `@enter()` hook ran once. That's the behaviour.

---

## What This Is and Isn't

This is not Modal. Not even close. Modal has:

- Container image building with layer caching
- Auto-scaling (Skyscale has a hardcoded warm pool of 2 VMs)
- Secrets management
- Volumes, distributed dicts, queues
- Scheduled functions
- Proper multi-tenant auth (Skyscale's auth middleware isn't even wired up to the routes yet)
- A global infrastructure footprint across multiple clouds

What Skyscale *does* have now is the **core abstraction**: write a Python function, decorate it, deploy with one command, get a URL. The mechanics of how Modal does this — serialize code, ship it to a remote process, generate a web server wrapper, proxy requests — those are all here and working.

The GPU path (routing `gpu="a100"` to an Akash deployment) is also wired in, just not something I can cheaply demo without burning Akash tokens. The `GetCompute` / `ReleaseCompute` interface handles it transparently.

---

## What I'd Do Next

The most important missing piece for real usability is the **image build cache**. Right now every cold-start re-runs `pip install` in a fresh venv. On the VPS with a warm network cache that's maybe 30–60 seconds for `pandas`. With `vllm` it would be completely untenable. The right solution is snapshotting the Firecracker VM after the first `pip install` completes, and restoring from that snapshot on subsequent boots — which gets you sub-second cold starts. Firecracker supports this natively; it's just not plumbed in yet.

After that: proper auto-scaling (replace the hardcoded pool of 2 with a demand-driven allocator), then secrets injection, then maybe volumes.

But the decorator SDK working end-to-end on a real server? That felt good.

---

*The code is all on [GitHub](https://github.com/Shubham-Rasal/skyscale). Commit `92d02e2` is where this landed.*
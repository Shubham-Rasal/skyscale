# Skyscale

**Serverless compute and GPU.** Run code and models without managing servers: CPU workloads on Firecracker microVMs, GPU workloads on decentralized GPU (Akash), plus FaaS invocations, persistent sandboxes for agents, and Modal-style Python apps with HTTP endpoints.

## Overview

Skyscale is a **serverless compute platform** with a **GPU path** for training, inference, and long-running services. You ship functions or container-backed apps; the control plane provisions **CPU** isolation (Firecracker) or **GPU** capacity (Akash), routes traffic, and tracks state so you focus on code—not clusters or schedulers.

At the core is **Lambda-style FaaS**: functions run in isolated micro-VMs, triggered by HTTP or events, with optional async execution and warm pools for lower cold starts.

For **GPU**, the same control plane can schedule heavy jobs (e.g. training) and **deployed web workloads** (inference, APIs) onto GPU-backed environments, while **CPU** stays the default for lightweight and bursty work.

The **Sandbox API** targets **AI agents**: persistent sessions, multi-step exec, and file I/O in isolated environments—same isolation model, different ergonomics.

The **Python App SDK** (`skyscale`) lets you declare `App`, `Image`, and decorators for HTTP routes; deployments get a **persistent FastAPI + uvicorn** process (`/serve` on the daemon) and public URLs under **`/proxy/{slug}{path}`**—on CPU or GPU, depending on how you declare hardware in code.

![Architecture Diagram](arch.png)

## Features

### Platform: serverless CPU and GPU
- **CPU compute** — Firecracker microVMs per invocation or session: strong isolation, predictable serverless semantics
- **GPU compute** — route eligible workloads to GPU providers (e.g. Akash) for training and inference-class services
- **Unified control plane** — one API and scheduler story for FaaS jobs, sandboxes, and App deployments
- **No server management** — you register code or deploy apps; Skyscale maps them to the right hardware class

### FaaS (Serverless Functions)
- **Python Function Support**: Lambda-style functions with AWS Lambda-compatible `event`/`context` interface
- **CLI & API Management**: REST API and command-line interface for function management
- **Isolated Execution**: Each function runs in a secure Firecracker micro-VM
- **Warm VM Pool**: Pre-warmed VMs reduce cold start latency
- **Async Execution**: Fire-and-forget invocations with result polling
- **State Persistence**: SQLite for function metadata and execution history

### Sandbox API (AI agents on serverless compute)
- **Persistent Sessions**: A sandbox is a long-lived VM exclusively held for one agent session
- **Multi-language Execution**: Run Python 3 or Bash code synchronously
- **Persistent Filesystem**: Files written in one exec call are available in the next
- **File Upload/Download**: Transfer files to and from the sandbox workspace
- **TTL-based Cleanup**: Sandboxes auto-expire and their VMs are reclaimed
- **Python SDK**: `SandboxClient` with context-manager support for easy agent integration

### Python App SDK (HTTP on durable CPU or GPU workloads)
- **`skyscale` package**: `App`, `Image.pip_install(...)`, `@app.function` / `@app.cls`, `@skyscale.web_endpoint`, `@skyscale.enter`
- **Deployments API**: `POST /api/deployments` with a JSON spec (per web route); response includes a public **`url`**
- **Routing**: `GET`/`POST` `/proxy/{slug}{path}` reverse-proxies to the workload (CPU or GPU)
- **CLI**: `skyscale deploy myapp.py` introspects the file and posts specs; `skyscale deploy myfunc/` still registers a classic FaaS function from `handler.py`
- **Environment**: set `SKYSCALE_PUBLIC_BASE` (e.g. `http://api.example.com:8080`) so returned URLs are correct; `SKYSCALE_SDK_PYTHON` points the CLI at the repo’s `sdk/python` if auto-detection fails

## Architecture

Serverless workloads flow from clients and SDKs through the control plane to **CPU VMs (Firecracker)** or **GPU deployments (Akash)**, depending on hardware selection.

```
AI Agent / SDK
     |
     v
Sandbox HTTP API   (/api/sandboxes/*)
     |
     v
SandboxManager     (control-plane/sandbox/)
     |
     v
VMManager          (control-plane/vm/)  — CPU pool + GPU (Akash)
     |
     v
Firecracker VM  |  GPU host (Akash)
     |
In-VM Daemon       (cmd/daemon/) — FaaS `/execute`, `/serve` (FastAPI), sandbox exec, file I/O
```

### Components

**Control Plane**
- **Function Registry** — function metadata, code, configurations
- **VM Manager** — Firecracker VM lifecycle and warm pool; GPU provisioning via Akash when requested
- **Scheduler** — dispatches invocations to CPU VMs or GPU backends (e.g. Akash) per job options
- **Sandbox Manager** — creates/manages long-lived sandbox sessions
- **State Manager** — SQLite state for functions, executions, VMs, sandboxes
- **API Server** — REST API (FaaS + Sandbox + Deployments)
- **Deployment manager** — long-lived web routes, proxy to workloads (`control-plane/deployment/`)
- **Auth Service** — API key management

**Execution environment**
- **Firecracker micro-VMs (CPU)** — hardware-level isolation for serverless function and sandbox workloads
- **GPU hosts (Akash)** — for training and GPU-class App deployments when hardware selects GPU
- **In-VM Daemon (Go)** — FaaS execution, `/serve` for deployed apps (uvicorn), sandbox sync exec
- **Persistent Workspace** — `/sandbox/workspace` inside each VM, persists across exec calls
- **CNI Networking** — ptp + tc-redirect-tap, VMs on 192.168.1.0/24

**SDK**
- **Python** — `skyscale_sandbox` (Sandbox API client), `skyscale` (App / deploy DSL + `skyscale._deploy` spec emission)

## Getting Started

### Prerequisites
- Linux (required for Firecracker + KVM on the control plane host)
- GPU-related features (Akash) need a configured Akash wallet / provider path as in your deployment setup
- Go 1.21+
- Python 3.8+ (for SDK and function runtime)
- Firecracker binary at `/usr/local/bin/firecracker`
- CNI plugins: `ptp`, `tc-redirect-tap` in `/opt/cni/bin`

### Build

```bash
# Control plane (needs Linux or cross-compile targets that match your Firecracker hosts)
cd control-plane
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o skyscale-cp .

# In-VM daemon (paths are relative to the repo root)
cd ../cmd/daemon
go build -o skyscale-daemon .
# Optional: static Linux binary for VM images
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o skyscale-daemon-linux .

# CLI
cd ../cli
go build -o skyscale .
```

### Configuration

Create `control-plane/.env`:

```env
PORT=8080
DB_PATH=/path/to/skyscale.db
FAAS_VM_KERNEL_PATH=/path/to/vmlinux
FAAS_VM_ROOTFS_PATH=/path/to/rootfs.ext4
FAAS_VM_MEMORY_MB=128
FAAS_VM_CPU_COUNT=1
API_KEY_SALT=your-salt
JWT_SECRET=your-secret
```

### Start the control plane

```bash
./skyscale-cp
```

---

## App deployments (Modal-style)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/deployments` | Create one **App** deployment (one web route per POST body) |
| `GET` | `/api/deployments` | List deployments |
| * | `/proxy/{slug}{path}` | Reverse-proxy to the workload backing that deployment |

Set `SKYSCALE_PUBLIC_BASE` on the control plane so deployment responses use the correct origin.

Minimal example (`examples/skyscale_web_app.py`):

Emit deployment JSON (inspect only):

```bash
pip install -e sdk/python   # installs skyscale + skyscale_sandbox
PYTHONPATH=sdk/python python3 -m skyscale._deploy examples/skyscale_web_app.py | jq .
```

Deploy with the CLI (control plane must be running; needs Linux + Firecracker for CPU VMs unless you use your own integration):

```bash
skyscale --api-url http://localhost:8080 deploy examples/skyscale_web_app.py
```

Call the returned URL, e.g.:

```bash
curl -s -X POST 'http://localhost:8080/proxy/demo-web--process/process' \
  -H 'Content-Type: application/json' \
  -d '{"data":{"a":[1,2,3],"b":[3,2,1]}}'
```

**Local daemon-only check (no Firecracker)** — useful on macOS: build `cmd/daemon`, run it on `:8081`, then `POST /serve` with the JSON from `skyscale._deploy` and hit the uvicorn port directly to validate FastAPI wiring.

---

## FaaS Usage

Register a function:
```bash
curl -X POST localhost:8080/api/functions \
  -H "Content-Type: application/json" \
  -d '{
    "name": "greet",
    "runtime": "python3",
    "code": "def handle(event, context):\n    return {\"message\": \"Hello, \" + event[\"name\"]}",
    "timeout": 30,
    "memory": 128
  }'
```

Invoke it:
```bash
curl -X POST localhost:8080/api/functions/name/greet/invoke \
  -H "Content-Type: application/json" \
  -d '{"input": {"name": "world"}, "sync": true}'
```

---

## Sandbox API

### REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/sandboxes` | Create a new sandbox |
| `GET` | `/api/sandboxes` | List all sandboxes |
| `GET` | `/api/sandboxes/{id}` | Get sandbox status |
| `DELETE` | `/api/sandboxes/{id}` | Destroy sandbox |
| `POST` | `/api/sandboxes/{id}/exec` | Execute code synchronously |
| `POST` | `/api/sandboxes/{id}/files/{path}` | Upload a file |
| `GET` | `/api/sandboxes/{id}/files/{path}` | Download a file |

Deploy (Python App DSL) endpoints live under **Deployments** alongside the **`/proxy/...`** route — see **App deployments (Modal-style)** above.

### Quick start (curl)

```bash
# Create a sandbox
SB=$(curl -s -X POST localhost:8080/api/sandboxes \
  -H "Content-Type: application/json" \
  -d '{"runtime":"python3","ttl_seconds":300}' | jq -r .id)

# Run code
curl -s -X POST localhost:8080/api/sandboxes/$SB/exec \
  -d '{"code":"x = 42\nprint(x)"}'
# → {"stdout":"42\n","exit_code":0,...}

# Write a file and read it back in a later call
curl -X POST localhost:8080/api/sandboxes/$SB/exec \
  -d '{"code":"open(\"data.txt\",\"w\").write(\"hello\")"}'
curl -X POST localhost:8080/api/sandboxes/$SB/exec \
  -d '{"code":"print(open(\"data.txt\").read())"}'
# → {"stdout":"hello","exit_code":0,...}

# Upload a file
curl -X POST localhost:8080/api/sandboxes/$SB/files/input.csv \
  --data-binary @local_file.csv

# Download a file
curl localhost:8080/api/sandboxes/$SB/files/output.csv -o output.csv

# Destroy
curl -X DELETE localhost:8080/api/sandboxes/$SB
```

### Python SDKs

```bash
pip install -e sdk/python
```

#### Sandbox (`skyscale_sandbox`)

```python
from skyscale_sandbox import SandboxClient

client = SandboxClient("http://localhost:8080")

# Context manager — sandbox is automatically destroyed on exit
with client.create(runtime="python3", ttl=600) as sb:
    r = sb.exec("print('hello from Skyscale')")
    print(r.stdout)  # "hello from Skyscale\n"

    # Files persist across exec calls within the same sandbox
    sb.exec("open('state.txt','w').write('42')")
    r = sb.exec("print(open('state.txt').read())")
    print(r.stdout)  # "42\n"

    # Upload/download files
    sb.upload_file("data.csv", open("data.csv", "rb").read())
    r = sb.exec("import csv; print(sum(1 for _ in open('data.csv')))")
    print(r.stdout)

    output = sb.download_file("result.json")
```

#### App DSL (`skyscale`)

```python
import skyscale

app = skyscale.App("demo")
image = skyscale.Image.pip_install("pandas")

@app.function(image=image)
@skyscale.web_endpoint(method="POST", path="/process")
def process(data: dict) -> dict:
    import pandas as pd
    return {"result": pd.DataFrame(data).describe().to_dict()}
```

---

## Testing

### Unit tests
```bash
# State models
go test ./control-plane/state/...

# Sandbox manager
go test ./control-plane/sandbox/...

# API handlers
go test ./control-plane/api/...

# Daemon handlers (requires Python3 and Bash installed locally)
go test ./cmd/daemon/...

# Python SDK
cd sdk/python && pip install -e . && python -m pytest tests/
```

### Integration / E2E tests
Requires a running control plane with real Firecracker VMs:
```bash
SKYSCALE_URL=http://localhost:8080 \
  go test -tags=integration ./tests/e2e/... -v -timeout 300s
```

E2E tests cover: full sandbox lifecycle, file persistence, VM isolation between sandboxes, TTL-based cleanup, and concurrent sandboxes.

---

## Project Structure

```
.
├── cmd/
│   ├── cli/          # CLI tool
│   └── daemon/       # In-VM daemon (FaaS + sandbox exec + file I/O)
├── control-plane/
│   ├── api/          # HTTP handlers (FaaS + sandbox)
│   ├── auth/         # API key management
│   ├── registry/     # Function registry
│   ├── sandbox/      # Sandbox manager
│   ├── scheduler/    # FaaS execution scheduler
│   ├── deployment/   # Long-lived deployments + proxy
│   ├── state/        # SQLite state (functions, executions, VMs, sandboxes)
│   └── vm/           # Firecracker VM lifecycle + warm pool
├── sdk/
│   └── python/       # skyscale (App/deploy), skyscale_sandbox (Sandbox client)
├── tests/
│   └── e2e/          # Integration tests (build tag: integration)
├── examples/         # Example functions
└── scripts/          # Build and setup scripts
```

---

## Configuration Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `SKYSCALE_PUBLIC_BASE` | — | Origin for deployment URLs (`http(s)://host:port`, no trailing slash) |
| `SKYSCALE_SDK_PYTHON` | — | Absolute path to `sdk/python` for `skyscale deploy` when not run from the repo |
| `PORT` | `8080` | Control plane HTTP port |
| `DB_PATH` | `skyscale.db` | SQLite database path |
| `FAAS_VM_KERNEL_PATH` | — | Path to Firecracker kernel image |
| `FAAS_VM_ROOTFS_PATH` | — | Path to VM root filesystem |
| `FAAS_VM_MEMORY_MB` | `128` | Memory per VM in MB |
| `FAAS_VM_CPU_COUNT` | `1` | vCPUs per VM |
| `API_KEY_SALT` | — | Salt for API key hashing |
| `JWT_SECRET` | — | JWT signing secret |

---

## License

MIT License — see the LICENSE file for details.

## Acknowledgements
- [Firecracker](https://github.com/firecracker-microvm/firecracker) — secure and fast microVMs
- [firecracker-go-sdk](https://github.com/firecracker-microvm/firecracker-go-sdk) — Go SDK for Firecracker
- [tc-redirect-tap](https://github.com/awslabs/tc-redirect-tap) — CNI plugin for Firecracker networking
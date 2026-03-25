# Skyscale
A serverless platform and AI agent sandbox built on Firecracker microVMs

## Overview

Skyscale is a serverless platform that enables you to run functions in the cloud without managing infrastructure. The platform handles the provisioning, scaling, and management of the underlying compute resources, allowing you to focus solely on writing your code.

The core of Skyscale is the Lambda-style function — code that is executed in isolated micro-VMs powered by Firecracker, triggered by events or direct invocation.

On top of the FaaS layer, Skyscale now provides a **Sandbox API** designed for AI agents: create a persistent, isolated environment, run arbitrary code across multiple calls, read and write files, and destroy the environment when done — all backed by the same Firecracker VM isolation.

![Architecture Diagram](arch.png)

## Features

### FaaS (Serverless Functions)
- **Python Function Support**: Lambda-style functions with AWS Lambda-compatible `event`/`context` interface
- **CLI & API Management**: REST API and command-line interface for function management
- **Isolated Execution**: Each function runs in a secure Firecracker micro-VM
- **Warm VM Pool**: Pre-warmed VMs reduce cold start latency
- **Async Execution**: Fire-and-forget invocations with result polling
- **State Persistence**: SQLite for function metadata and execution history

### Sandbox API (AI Agents)
- **Persistent Sessions**: A sandbox is a long-lived VM exclusively held for one agent session
- **Multi-language Execution**: Run Python 3 or Bash code synchronously
- **Persistent Filesystem**: Files written in one exec call are available in the next
- **File Upload/Download**: Transfer files to and from the sandbox workspace
- **TTL-based Cleanup**: Sandboxes auto-expire and their VMs are reclaimed
- **Python SDK**: `SandboxClient` with context-manager support for easy agent integration

## Architecture

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
VMManager          (control-plane/vm/)
     |
     v
Firecracker VM
     |
In-VM Daemon       (cmd/daemon/) — sync exec + file I/O + legacy FaaS handler
```

### Components

**Control Plane**
- **Function Registry** — function metadata, code, configurations
- **VM Manager** — Firecracker VM lifecycle and warm pool
- **Scheduler** — dispatches FaaS invocations to VMs
- **Sandbox Manager** — creates/manages long-lived sandbox sessions
- **State Manager** — SQLite state for functions, executions, VMs, sandboxes
- **API Server** — REST API (FaaS + Sandbox)
- **Auth Service** — API key management

**Execution Environment**
- **Firecracker Micro-VMs** — hardware-level isolation (separate kernel per VM)
- **In-VM Daemon (Go)** — handles both FaaS execution callbacks and sandbox sync exec
- **Persistent Workspace** — `/sandbox/workspace` inside each VM, persists across exec calls
- **CNI Networking** — ptp + tc-redirect-tap, VMs on 192.168.1.0/24

**SDK**
- **Python SDK** — `sdk/python/skyscale_sandbox`

## Getting Started

### Prerequisites
- Linux (required for Firecracker + KVM)
- Go 1.21+
- Python 3.8+ (for SDK and function runtime)
- Firecracker binary at `/usr/local/bin/firecracker`
- CNI plugins: `ptp`, `tc-redirect-tap` in `/opt/cni/bin`

### Build

```bash
# Control plane
cd control-plane
go build -o skyscale-cp .

# In-VM daemon (cross-compile for Linux)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o skyscale-daemon ./cmd/daemon/

# CLI
cd cmd/cli
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

### Python SDK

```bash
pip install -e sdk/python
```

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
│   ├── state/        # SQLite state (functions, executions, VMs, sandboxes)
│   └── vm/           # Firecracker VM lifecycle + warm pool
├── sdk/
│   └── python/       # Python SDK (skyscale_sandbox)
├── tests/
│   └── e2e/          # Integration tests (build tag: integration)
├── examples/         # Example functions
└── scripts/          # Build and setup scripts
```

---

## Configuration Reference

| Variable | Default | Description |
|----------|---------|-------------|
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
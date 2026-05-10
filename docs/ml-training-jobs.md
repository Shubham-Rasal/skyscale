# Custom ML Training Jobs

Skyscale GPU training jobs run a Docker image on a selected GPU provider. The
container owns the training loop and reports progress back to the control plane.

## Container Contract

Your image should:

- Start the training process from its default `CMD`, or from the command used by
  the provider.
- Read configuration from environment variables.
- Read `SKYSCALE_JOB_ID` or `JOB_ID` as the logical job id.
- Read `EXECUTION_ID` as the Skyscale execution id.
- Read `CONTROL_PLANE_URL` as the public control-plane URL.
- POST metrics to `POST $CONTROL_PLANE_URL/api/training/metrics`.
- POST completion to `POST $CONTROL_PLANE_URL/api/executions/$EXECUTION_ID/complete`.

Minimal Python callback helpers:

```python
import os
import time
import requests

JOB_ID = os.environ.get("SKYSCALE_JOB_ID") or os.environ.get("JOB_ID", "local-dev")
EXECUTION_ID = os.environ.get("EXECUTION_ID", JOB_ID)
CONTROL_PLANE_URL = os.environ["CONTROL_PLANE_URL"]


def push_metric(step: int, loss: float, gpu_util: int = 0) -> None:
    requests.post(
        f"{CONTROL_PLANE_URL}/api/training/metrics",
        json={
            "job_id": JOB_ID,
            "step": step,
            "loss": loss,
            "gpu_util": gpu_util,
            "timestamp": int(time.time() * 1000),
        },
        timeout=5,
    )


def mark_complete(output: str) -> None:
    requests.post(
        f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/complete",
        json={"status": "completed", "output": output},
        timeout=10,
    )
```

## Example Trainer

```python
import os
import torch
import torch.nn as nn
import torch.optim as optim

epochs = int(os.environ.get("EPOCHS", "3"))
device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

model = nn.Sequential(nn.Linear(32, 64), nn.ReLU(), nn.Linear(64, 2)).to(device)
opt = optim.Adam(model.parameters(), lr=float(os.environ.get("LR", "1e-3")))
loss_fn = nn.CrossEntropyLoss()

for step in range(epochs * 100):
    x = torch.randn(256, 32, device=device)
    y = torch.randint(0, 2, (256,), device=device)
    opt.zero_grad()
    loss = loss_fn(model(x), y)
    loss.backward()
    opt.step()

    if step % 10 == 0:
        push_metric(step=step, loss=float(loss.item()))

mark_complete(f"finished on {device}")
```

## Dockerfile

```dockerfile
FROM pytorch/pytorch:2.6.0-cuda12.4-cudnn9-runtime

WORKDIR /app
RUN pip install --no-cache-dir requests
COPY train.py /app/train.py

CMD ["python", "/app/train.py"]
```

Build and push the image:

```bash
docker build -t ghcr.io/YOUR_ORG/my-trainer:v1 .
docker push ghcr.io/YOUR_ORG/my-trainer:v1
```

## Submit With Curl

Set `provider` to `huggingface` or `akash`. The control plane stores the job as
queued, then the background dispatcher starts the provider deployment.

```bash
curl -X POST "$SKYSCALE_API_URL/api/training/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "job_id": "my-custom-trainer",
    "provider": "huggingface",
    "docker_image": "ghcr.io/YOUR_ORG/my-trainer:v1",
    "gpu_model": "a10g",
    "control_plane_url": "https://your-control-plane.example.com",
    "env_vars": {
      "EPOCHS": "3",
      "LR": "1e-3"
    }
  }'
```

## Submit With Python

```python
import os
import requests

api_url = os.environ.get("SKYSCALE_API_URL", "http://localhost:8080")

resp = requests.post(
    f"{api_url}/api/training/jobs",
    json={
        "job_id": "my-custom-trainer",
        "provider": "huggingface",
        "docker_image": "ghcr.io/YOUR_ORG/my-trainer:v1",
        "gpu_model": "a10g",
        "control_plane_url": "https://your-control-plane.example.com",
        "env_vars": {
            "EPOCHS": "3",
            "LR": "1e-3",
        },
    },
    timeout=30,
)
resp.raise_for_status()
print(resp.json())
```

## Provider Notes

- `huggingface` uses Hugging Face Jobs. Configure the control plane with
  `HF_TOKEN` and `HF_NAMESPACE`.
- `akash` uses the Akash deployment path. Configure the Akash wallet/API
  environment expected by the control plane.
- `control_plane_url` must be reachable from the provider. `localhost`,
  `127.0.0.1`, and private LAN addresses point at the remote container from
  Hugging Face or Akash, not your laptop. For Hugging Face jobs, Skyscale now
  rejects local callback URLs before launching the job. Set `SKYSCALE_PUBLIC_BASE`
  or `HF_CALLBACK_BASE` to a public control-plane URL.

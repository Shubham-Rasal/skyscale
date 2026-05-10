"""MNIST CNN trainer — streams metrics to Skyscale control plane during training."""

import os
import threading
import time

import requests
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import DataLoader
from torchvision import datasets, transforms
from flask import Flask, jsonify

JOB_ID = os.environ.get("SKYSCALE_JOB_ID") or os.environ.get("JOB_ID", "local-dev")
EXECUTION_ID = os.environ.get("EXECUTION_ID", JOB_ID)
CONTROL_PLANE_URL = os.environ.get("CONTROL_PLANE_URL", "http://localhost:8080")
METRICS_PORT = int(os.environ.get("METRICS_PORT", "8082"))
EPOCHS = int(os.environ.get("EPOCHS", "5"))
BATCH_SIZE = int(os.environ.get("BATCH_SIZE", "256"))
LR = float(os.environ.get("LR", "1e-3"))

# --- GPU util ---

def get_gpu_util() -> int:
    try:
        import pynvml
        pynvml.nvmlInit()
        handle = pynvml.nvmlDeviceGetHandleByIndex(0)
        util = pynvml.nvmlDeviceGetUtilizationRates(handle)
        return int(util.gpu)
    except Exception:
        return 0

def get_gpu_memory() -> tuple:
    try:
        import pynvml
        pynvml.nvmlInit()
        handle = pynvml.nvmlDeviceGetHandleByIndex(0)
        info = pynvml.nvmlDeviceGetMemoryInfo(handle)
        return int(info.used // 1024 // 1024), int(info.total // 1024 // 1024)
    except Exception:
        return 0, 0

# --- Metrics HTTP server ---

app = Flask(__name__)
_current_metrics: dict = {}

@app.route("/metrics")
def metrics_endpoint():
    return jsonify(_current_metrics)

@app.route("/health")
def health():
    return "ok"

def _run_metrics_server():
    import logging
    logging.getLogger("werkzeug").setLevel(logging.ERROR)
    app.run(host="0.0.0.0", port=METRICS_PORT)

# --- Model ---

class MnistCNN(nn.Module):
    def __init__(self):
        super().__init__()
        self.features = nn.Sequential(
            nn.Conv2d(1, 32, 3, padding=1), nn.ReLU(),
            nn.Conv2d(32, 64, 3, padding=1), nn.ReLU(),
            nn.MaxPool2d(2),
            nn.Conv2d(64, 128, 3, padding=1), nn.ReLU(),
            nn.MaxPool2d(2),
        )
        self.classifier = nn.Sequential(
            nn.Flatten(),
            nn.Linear(128 * 7 * 7, 256), nn.ReLU(),
            nn.Dropout(0.3),
            nn.Linear(256, 10),
        )

    def forward(self, x):
        return self.classifier(self.features(x))

# --- Metric push ---

def push_metric(metric: dict):
    global _current_metrics
    _current_metrics = metric
    try:
        requests.post(
            f"{CONTROL_PLANE_URL}/api/training/metrics",
            json=metric,
            timeout=2,
        )
    except Exception as exc:
        print(f"[metrics] push failed: {exc}")

# --- Main ---

def main():
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    print(f"[mnist] job_id={JOB_ID}  device={device}  control_plane={CONTROL_PLANE_URL}")
    print(f"[mnist] training MNIST CNN for {EPOCHS} epochs, batch_size={BATCH_SIZE}")

    threading.Thread(target=_run_metrics_server, daemon=True).start()

    transform = transforms.Compose([
        transforms.ToTensor(),
        transforms.Normalize((0.1307,), (0.3081,)),
    ])

    train_dataset = datasets.MNIST("/data", train=True, download=True, transform=transform)
    val_dataset = datasets.MNIST("/data", train=False, download=True, transform=transform)
    train_loader = DataLoader(train_dataset, batch_size=BATCH_SIZE, shuffle=True, num_workers=2, pin_memory=(device.type == "cuda"))
    val_loader = DataLoader(val_dataset, batch_size=BATCH_SIZE, shuffle=False, num_workers=2)

    model = MnistCNN().to(device)
    optimizer = optim.Adam(model.parameters(), lr=LR)
    criterion = nn.CrossEntropyLoss()
    scheduler = optim.lr_scheduler.CosineAnnealingLR(optimizer, T_max=EPOCHS)

    global_step = 0
    for epoch in range(1, EPOCHS + 1):
        model.train()
        running_loss = 0.0
        correct = 0
        total = 0

        for batch_idx, (images, labels) in enumerate(train_loader):
            images, labels = images.to(device), labels.to(device)
            optimizer.zero_grad()
            outputs = model(images)
            loss = criterion(outputs, labels)
            loss.backward()
            optimizer.step()

            running_loss += loss.item()
            _, predicted = outputs.max(1)
            correct += predicted.eq(labels).sum().item()
            total += labels.size(0)
            global_step += 1

            if batch_idx % 50 == 0:
                gpu_util = get_gpu_util()
                gpu_used, gpu_total = get_gpu_memory()
                avg_loss = running_loss / (batch_idx + 1)
                acc = 100.0 * correct / total
                print(
                    f"[epoch {epoch}/{EPOCHS}  batch {batch_idx:>3}]  "
                    f"loss={avg_loss:.4f}  acc={acc:.1f}%  gpu={gpu_util}%"
                )
                push_metric({
                    "job_id": JOB_ID,
                    "step": global_step,
                    "epoch": epoch,
                    "loss": avg_loss,
                    "train_acc": acc,
                    "gpu_util": gpu_util,
                    "gpu_memory_used_mb": gpu_used,
                    "gpu_memory_total_mb": gpu_total,
                    "timestamp": int(time.time() * 1000),
                })

        # Validation
        model.eval()
        val_correct = 0
        val_total = 0
        with torch.no_grad():
            for images, labels in val_loader:
                images, labels = images.to(device), labels.to(device)
                outputs = model(images)
                _, predicted = outputs.max(1)
                val_correct += predicted.eq(labels).sum().item()
                val_total += labels.size(0)

        val_acc = 100.0 * val_correct / val_total
        scheduler.step()
        print(f"[epoch {epoch}/{EPOCHS}] val_acc={val_acc:.2f}%")

        push_metric({
            "job_id": JOB_ID,
            "step": global_step,
            "epoch": epoch,
            "loss": running_loss / len(train_loader),
            "train_acc": 100.0 * correct / total,
            "val_acc": val_acc,
            "gpu_util": get_gpu_util(),
            "timestamp": int(time.time() * 1000),
        })

    print("[mnist] training complete!")

    try:
        requests.post(
            f"{CONTROL_PLANE_URL}/api/executions/{EXECUTION_ID}/complete",
            json={"status": "completed", "output": f"MNIST CNN trained for {EPOCHS} epochs, val_acc={val_acc:.2f}%"},
            timeout=10,
        )
        print(f"[mnist] completion signalled to {CONTROL_PLANE_URL}")
    except Exception as exc:
        print(f"[mnist] completion callback failed: {exc}")


if __name__ == "__main__":
    main()

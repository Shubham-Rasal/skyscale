#!/usr/bin/env bash
# Runs on the AWS GPU instance after the repo tarball is extracted to ~/skyscale.
set -euo pipefail

ROOT="${1:-$HOME/skyscale}"
cd "${ROOT}"

log() { echo "[aws-remote] $*"; }

log "GPU inventory"
nvidia-smi || true

if ! command -v docker >/dev/null 2>&1; then
  log "Installing Docker"
  curl -fsSL https://get.docker.com | sudo sh
  sudo usermod -aG docker "${USER}" || true
fi

if docker info >/dev/null 2>&1; then
  DOCKER="docker"
else
  log "Using sudo for Docker"
  DOCKER="sudo docker"
fi

gpu_smoke() {
  ${DOCKER} run --rm --gpus all nvidia/cuda:12.9.0-base-ubuntu22.04 nvidia-smi
}

if ! gpu_smoke >/dev/null 2>&1; then
  log "Installing NVIDIA Container Toolkit"
  curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
  curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
    | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
    | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
  sudo apt-get update
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y nvidia-container-toolkit
  sudo nvidia-container-toolkit runtime configure --runtime=docker
  sudo systemctl restart docker
  DOCKER="sudo docker"
  gpu_smoke
fi

if [[ -f /opt/pytorch/bin/activate ]]; then
  # shellcheck disable=SC1091
  source /opt/pytorch/bin/activate
  python3 - <<'PY' || true
import torch
print("torch", torch.__version__, "cuda", torch.cuda.is_available())
if torch.cuda.is_available():
    print("device", torch.cuda.get_device_name(0))
PY
fi

log "Building slime runtime image (may take 10-20 minutes)"
${DOCKER} build -t skyscale/slime:local -f training/slime/Dockerfile training/slime

log "Verifying pinned runtime inside image"
${DOCKER} run --rm --gpus all skyscale/slime:local python /opt/skyscale/slime/runtime_versions.py

log "GPU smoke inside slime image"
${DOCKER} run --rm --gpus all skyscale/slime:local nvidia-smi

if command -v kubectl >/dev/null 2>&1; then
  log "Rendering Kubernetes manifests"
  kubectl kustomize deploy/k8s >/tmp/skyscale-k8s.yaml
  wc -l /tmp/skyscale-k8s.yaml
else
  log "kubectl not installed; skipping kustomize render on instance"
fi

MODEL_ROOT="${MODEL_ROOT:-/models}"
QWEN_REVISION="${QWEN_HF_REVISION:-main}"
if [[ -n "${HF_TOKEN:-}" ]]; then
  log "Preparing Qwen3-0.6B artifacts (revision=${QWEN_REVISION})"
  sudo mkdir -p "${MODEL_ROOT}"
  sudo chown -R "${USER}:${USER}" "${MODEL_ROOT}"
  ${DOCKER} run --rm --gpus all \
    -e HF_TOKEN \
    -e MODEL_ROOT=/models \
    -v "${MODEL_ROOT}:/models" \
    skyscale/slime:local \
    python /opt/skyscale/slime/prepare_model.py \
      --model Qwen/Qwen3-0.6B \
      --revision "${QWEN_REVISION}" \
      --output-root /models
  log "Model prep complete; full M1 train.py still needs control plane + RayJob"
else
  log "HF_TOKEN not set; skipping model download"
fi

log "Remote bootstrap finished successfully"

#!/usr/bin/env bash
# M1 end-to-end: control plane (test mode) + sandbox rewards + one slime rollout.
set -euo pipefail

ROOT="${1:-$HOME/skyscale}"
cd "${ROOT}"

log() { echo "[aws-e2e] $*"; }

MODEL_ROOT="${MODEL_ROOT:-/models}"
RUN_ID="${SKYSCALE_RUN_ID:-aws-e2e-$(date +%s)}"
CP_URL="${SKYSCALE_CONTROL_PLANE_URL:-http://127.0.0.1:8080}"
BIN_DIR="${ROOT}/.aws-e2e-bin"
DAEMON_BIN="${BIN_DIR}/skyscale-daemon"
CP_BIN="${BIN_DIR}/skyscale-control-plane"
LOG_DIR="${ROOT}/.aws-e2e-logs"
mkdir -p "${LOG_DIR}"

if docker info >/dev/null 2>&1; then
  DOCKER="docker"
else
  DOCKER="sudo docker"
fi

if [[ ! -f "${MODEL_ROOT}/model-artifact.json" ]]; then
  log "model artifacts missing under ${MODEL_ROOT}; run bootstrap with HF_TOKEN first"
  exit 1
fi

if [[ ! -x "${DAEMON_BIN}" || ! -x "${CP_BIN}" ]]; then
  log "missing linux control-plane/daemon binaries in ${BIN_DIR}"
  exit 1
fi

cleanup() {
  if [[ -n "${CP_PID:-}" ]] && kill -0 "${CP_PID}" 2>/dev/null; then
    kill "${CP_PID}" 2>/dev/null || true
  fi
  if [[ -n "${DAEMON_PID:-}" ]] && kill -0 "${DAEMON_PID}" 2>/dev/null; then
    kill "${DAEMON_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local tries="${2:-60}"
  for _ in $(seq 1 "${tries}"); do
    if curl -sf "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

log "starting sandbox daemon"
SANDBOX_WORKSPACE="${ROOT}/.aws-e2e-sandbox" \
  VM_ID=host-vm-test VM_IP=127.0.0.1 \
  "${DAEMON_BIN}" >"${LOG_DIR}/daemon.log" 2>&1 &
DAEMON_PID=$!
wait_http "http://127.0.0.1:8081/health" 30

log "starting control plane (test mode) run_id=${RUN_ID}"
export DAEMON_PATH="${DAEMON_BIN}"
"${CP_BIN}" -test >"${LOG_DIR}/control-plane.log" 2>&1 &
CP_PID=$!
wait_http "${CP_URL}/health" 60

log "smoke: sample task + sandbox reward"
curl -sf -X POST "${CP_URL}/api/rl/env/tasks/sample" \
  -H 'Content-Type: application/json' \
  -d "{\"run_id\":\"${RUN_ID}\",\"seed\":1}" | tee "${LOG_DIR}/task-sample.json"
curl -sf -X POST "${CP_URL}/api/rl/env/evaluate" \
  -H 'Content-Type: application/json' \
  -d '{"run_id":"'"${RUN_ID}"'","task_id":"he-1","code":"def add(a,b):\n    return a+b"}' \
  | tee "${LOG_DIR}/evaluate-smoke.json"

log "running slime one-GPU RL loop (this may take 20-60 minutes)"
export SKYSCALE_CONTROL_PLANE_URL="${CP_URL}"
export SKYSCALE_RUN_ID="${RUN_ID}"
export MODEL_ROOT
${DOCKER} run --rm --gpus all --network host --ipc=host \
  -v "${MODEL_ROOT}:${MODEL_ROOT}" \
  -e HF_TOKEN="${HF_TOKEN:-}" \
  -e MODEL_ROOT \
  -e SKYSCALE_CONTROL_PLANE_URL \
  -e SKYSCALE_RUN_ID \
  -e PYTHONPATH=/opt/skyscale/slime:/root/slime:/root/Megatron-LM \
  skyscale/slime:local \
  bash /opt/skyscale/slime/configs/one_gpu_qwen3_0_6b.sh \
  2>&1 | tee "${LOG_DIR}/slime-train.log"

log "end-to-end RL training finished successfully (run_id=${RUN_ID})"

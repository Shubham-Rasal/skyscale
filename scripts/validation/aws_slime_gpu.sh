#!/usr/bin/env bash
# Launch an AWS spot/on-demand GPU box, run slime bootstrap tests, then terminate.
#
# Usage:
#   set -a && source .env.aws && set +a
#   bash scripts/validation/aws_slime_gpu.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GPU_PY="${ROOT}/.claude/skills/aws-gpu-spot/gpu.py"
NAME="${AWS_GPU_NAME:-skyscale-slime}"
TYPE="${AWS_GPU_TYPE:-g6.xlarge}"
VOLUME="${AWS_GPU_VOLUME_GB:-200}"
FALLBACK_TYPES="${AWS_GPU_FALLBACK_TYPES:-g5.xlarge,g4dn.xlarge,p5.4xlarge}"
TARBALL="/tmp/skyscale-aws-test.tar.gz"

if [[ ! -f "${GPU_PY}" ]]; then
  echo "missing ${GPU_PY}" >&2
  exit 1
fi

build_e2e_binaries() {
  local bin_dir="${ROOT}/.aws-e2e-bin"
  mkdir -p "${bin_dir}"
  echo "== Building linux/amd64 control plane + daemon for EC2 E2E =="
  (
    cd "${ROOT}/control-plane"
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${bin_dir}/skyscale-control-plane" .
  )
  (
    cd "${ROOT}/cmd/daemon"
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${bin_dir}/skyscale-daemon" .
  )
}

if [[ "${AWS_SLIME_E2E:-0}" == "1" ]]; then
  if [[ -z "${HF_TOKEN:-}" ]]; then
    echo "AWS_SLIME_E2E=1 requires HF_TOKEN in .env.aws" >&2
    exit 1
  fi
  build_e2e_binaries
fi

echo "== AWS identity =="
python3 "${GPU_PY}" whoami

echo "== Existing managed instances =="
python3 "${GPU_PY}" status || true

launch_instance() {
  local instance_type="$1"
  echo "== Launch ${NAME} (${instance_type}, volume ${VOLUME} GiB) =="
  python3 "${GPU_PY}" launch \
    --name "${NAME}" \
    --type "${instance_type}" \
    --market spot-then-ondemand \
    --volume-size "${VOLUME}" || return 1
}

if [[ "${AWS_GPU_SKIP_LAUNCH:-0}" != "1" ]]; then
  launched=0
  IFS=',' read -r -a try_types <<< "${TYPE},${FALLBACK_TYPES}"
  for instance_type in "${try_types[@]}"; do
    instance_type="$(echo "${instance_type}" | xargs)"
    [[ -z "${instance_type}" ]] && continue
    if launch_instance "${instance_type}"; then
      launched=1
      break
    fi
    echo "Type ${instance_type} failed; trying next fallback" >&2
    python3 "${GPU_PY}" terminate --name "${NAME}" 2>/dev/null || true
  done
  if [[ "${launched}" -ne 1 ]]; then
    echo "FAILED: no GPU instance type launched (capacity or quota)" >&2
    exit 1
  fi
else
  echo "Skipping launch (AWS_GPU_SKIP_LAUNCH=1)"
fi

STATE_DIR="${GPU_SKILL_STATE_DIR:-$HOME/.cache/aws-gpu-spot}"
STATE_FILE="${STATE_DIR}/${NAME}.json"
if [[ ! -f "${STATE_FILE}" ]]; then
  echo "state file missing: ${STATE_FILE}" >&2
  exit 1
fi

PEM="$(python3 - <<PY
import json
st=json.load(open("${STATE_FILE}"))
print(st["pem"])
PY
)"
IP="$(python3 - <<PY
import json
st=json.load(open("${STATE_FILE}"))
print(st.get("ip",""))
PY
)"

if [[ -z "${IP}" ]]; then
  echo "instance has no public IP in state" >&2
  exit 1
fi

SSH=(ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=20 -o ServerAliveInterval=30 -o ServerAliveCountMax=120 -i "${PEM}" "ubuntu@${IP}")
SCP=(scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ServerAliveInterval=30 -o ServerAliveCountMax=120 -i "${PEM}")

echo "== GPU smoke via gpu.py =="
python3 "${GPU_PY}" smoke --name "${NAME}"

echo "== Packaging repo =="
TAR_EXCLUDES=(
  --exclude=.git
  --exclude=node_modules
  --exclude='**/__pycache__'
  --exclude=minimodal
  --exclude=dashboard
  --exclude=.next
  --exclude=.cursor
  --exclude=.DS_Store
  --exclude=.env.aws
  --exclude=.env
  --exclude=.aws-e2e-bin
  --exclude=perf
  --exclude=tests
  --exclude=examples
  --exclude=sdk
  --exclude=control-plane/function-storage
  --exclude=control-plane/vm-storage
  --exclude=control-plane/skyscale-control-plane
  --exclude=control-plane/skyscale-control-plane-linux
  --exclude=control-plane/skyscale-cp-new
  --exclude=control-plane/control-plane
  --exclude='*.db'
  --exclude='*.log'
)
tar -czf "${TARBALL}" "${TAR_EXCLUDES[@]}" -C "${ROOT}" .
ls -lh "${TARBALL}"

echo "== Uploading repo to ${IP} =="
"${SCP[@]}" "${TARBALL}" "ubuntu@${IP}:/tmp/skyscale-aws-test.tar.gz"
"${SSH[@]}" 'rm -rf ~/skyscale && mkdir -p ~/skyscale && tar -xzf /tmp/skyscale-aws-test.tar.gz -C ~/skyscale'
if [[ "${AWS_SLIME_E2E:-0}" == "1" ]]; then
  echo "== Uploading E2E binaries =="
  "${SSH[@]}" 'mkdir -p ~/skyscale/.aws-e2e-bin'
  "${SCP[@]}" "${ROOT}/.aws-e2e-bin/skyscale-control-plane" "${ROOT}/.aws-e2e-bin/skyscale-daemon" "ubuntu@${IP}:~/skyscale/.aws-e2e-bin/"
fi

echo "== Remote bootstrap =="
REMOTE_ENV=""
if [[ -n "${HF_TOKEN:-}" ]]; then
  REMOTE_ENV="export HF_TOKEN='${HF_TOKEN}';"
fi
if [[ -n "${QWEN_HF_REVISION:-}" ]]; then
  REMOTE_ENV="${REMOTE_ENV} export QWEN_HF_REVISION='${QWEN_HF_REVISION}';"
fi
"${SSH[@]}" "chmod +x ~/skyscale/scripts/validation/aws_remote_bootstrap.sh && ${REMOTE_ENV} bash ~/skyscale/scripts/validation/aws_remote_bootstrap.sh ~/skyscale"

if [[ "${AWS_SLIME_E2E:-0}" == "1" ]]; then
  echo "== Remote end-to-end RL training =="
  E2E_ENV="${REMOTE_ENV} export SKYSCALE_RUN_ID='${SKYSCALE_RUN_ID:-aws-e2e}';"
  "${SSH[@]}" "chmod +x ~/skyscale/scripts/validation/aws_remote_e2e_train.sh && ${E2E_ENV} bash ~/skyscale/scripts/validation/aws_remote_e2e_train.sh ~/skyscale"
  echo "== AWS EC2 slime E2E RL loop passed on ${IP} (${NAME}) =="
else
  echo "== AWS EC2 slime bootstrap passed on ${IP} (${NAME}) =="
fi

if [[ "${AWS_GPU_SKIP_TERMINATE:-0}" != "1" ]]; then
  echo "== Terminating instance =="
  python3 "${GPU_PY}" terminate --name "${NAME}"
else
  echo "AWS_GPU_SKIP_TERMINATE=1 — terminate manually:"
  echo "  python3 ${GPU_PY} terminate --name ${NAME}"
fi

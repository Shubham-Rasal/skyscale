#!/usr/bin/env bash
set -euo pipefail

: "${MODEL_ROOT:=/models}"
: "${ROLLOUT_EXTERNAL_ENGINE_ADDRS:?comma-separated SGLang /server_info addresses required}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/qwen3_0_6b_model_args.sh"

exec python train.py \
  "${QWEN3_06B_MODEL_ARGS[@]}" \
  --actor-num-nodes 1 \
  --actor-num-gpus-per-node 1 \
  --rollout-external-engine-addrs "${ROLLOUT_EXTERNAL_ENGINE_ADDRS}" \
  --hf-checkpoint "${MODEL_ROOT}/hf" \
  --ref-load "${MODEL_ROOT}/torch_dist" \
  --load "${MODEL_ROOT}/resume" \
  --save "${MODEL_ROOT}/resume" \
  --save-interval 1 \
  --tensor-model-parallel-size 1 \
  --pipeline-model-parallel-size 1 \
  --context-parallel-size 1 \
  --n-samples-per-prompt 2 \
  --rollout-max-response-len 256 \
  --custom-generate-function-path skyscale.adapters.generate_from_skyscale \
  --custom-rm-path skyscale.adapters.async_sandbox_reward \
  --rollout-all-samples-process-path skyscale.adapters.publish_rollout_samples \
  "$@"

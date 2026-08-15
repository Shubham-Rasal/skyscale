#!/usr/bin/env bash
set -euo pipefail

: "${MODEL_ROOT:=/models}"
: "${ROLLOUT_EXTERNAL_ENGINE_ADDRS:?comma-separated SGLang /server_info addresses required}"

exec python train.py \
  --actor-num-nodes 1 \
  --actor-num-gpus-per-node 1 \
  --rollout-external-engine-addrs "${ROLLOUT_EXTERNAL_ENGINE_ADDRS}" \
  --hf-checkpoint "${MODEL_ROOT}/hf" \
  --ref-load "${MODEL_ROOT}/torch_dist" \
  --load "${MODEL_ROOT}/resume" \
  --save "${MODEL_ROOT}/resume" \
  --tensor-model-parallel-size 1 \
  --pipeline-model-parallel-size 1 \
  --context-parallel-size 1 \
  --n-samples-per-prompt 2 \
  --max-response-len 256 \
  --custom-generate-function-path skyscale.adapters.SkyScaleDataSource \
  --custom-rm-path skyscale.adapters.sandbox_reward \
  "$@"

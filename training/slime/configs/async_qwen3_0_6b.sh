#!/usr/bin/env bash
set -euo pipefail

: "${MODEL_ROOT:=/models}"
: "${ROLLOUT_EXTERNAL_ENGINE_ADDRS:?required}"
: "${MAX_POLICY_LAG_STEPS:=2}"
: "${MAX_QUEUE_AGE_SECONDS:=300}"
: "${OFF_POLICY_ACTION:=reweight}"

export SKYSCALE_MAX_POLICY_LAG_STEPS="${MAX_POLICY_LAG_STEPS}"
export SKYSCALE_MAX_QUEUE_AGE_SECONDS="${MAX_QUEUE_AGE_SECONDS}"
export SKYSCALE_OFF_POLICY_ACTION="${OFF_POLICY_ACTION}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/qwen3_0_6b_model_args.sh"

exec python train_async.py \
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

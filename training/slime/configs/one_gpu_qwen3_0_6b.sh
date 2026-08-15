#!/usr/bin/env bash
set -euo pipefail

# Hardware-gated correctness profile: one whole GPU, one prompt group, two
# responses, one optimizer step. No fractional GPU sharing is permitted.
: "${MODEL_ROOT:=/models}"
: "${SKYSCALE_CONTROL_PLANE_URL:?required}"
: "${SKYSCALE_RUN_ID:?required}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/qwen3_0_6b_model_args.sh"

exec python train.py \
  "${QWEN3_06B_MODEL_ARGS[@]}" \
  --actor-num-nodes 1 \
  --actor-num-gpus-per-node 1 \
  --colocate \
  --rollout-num-gpus-per-engine 1 \
  --hf-checkpoint "${MODEL_ROOT}/hf" \
  --ref-load "${MODEL_ROOT}/torch_dist" \
  --load "${MODEL_ROOT}/resume" \
  --save "${MODEL_ROOT}/resume" \
  --save-interval 1 \
  --num-rollout 1 \
  --rollout-batch-size 1 \
  --n-samples-per-prompt 2 \
  --global-batch-size 2 \
  --rollout-max-response-len 256 \
  --sglang-mem-fraction-static 0.35 \
  --tensor-model-parallel-size 1 \
  --pipeline-model-parallel-size 1 \
  --context-parallel-size 1 \
  --custom-generate-function-path skyscale.adapters.generate_from_skyscale \
  --custom-rm-path skyscale.adapters.async_sandbox_reward \
  --rollout-all-samples-process-path skyscale.adapters.publish_rollout_samples \
  "$@"

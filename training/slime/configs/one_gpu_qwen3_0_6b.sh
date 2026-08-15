#!/usr/bin/env bash
set -euo pipefail

# Hardware-gated correctness profile: one whole GPU, one prompt group, two
# responses, one optimizer step. No fractional GPU sharing is permitted.
: "${MODEL_ROOT:=/models}"
: "${SKYSCALE_CONTROL_PLANE_URL:?required}"
: "${SKYSCALE_RUN_ID:?required}"

# Must match upstream slime scripts/models/qwen3-0.6B.sh and prepare_model.py.
QWEN3_06B_MODEL_ARGS=(
  --swiglu
  --num-layers 28
  --hidden-size 1024
  --ffn-hidden-size 3072
  --num-attention-heads 16
  --group-query-attention
  --num-query-groups 8
  --use-rotary-position-embeddings
  --disable-bias-linear
  --normalization RMSNorm
  --norm-epsilon 1e-6
  --rotary-base 1000000
  --vocab-size 151936
  --kv-channels 128
  --qk-layernorm
)

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
  --max-response-len 256 \
  --sglang-mem-fraction-static 0.35 \
  --tensor-model-parallel-size 1 \
  --pipeline-model-parallel-size 1 \
  --context-parallel-size 1 \
  --custom-generate-function-path skyscale.adapters.SkyScaleDataSource \
  --custom-rm-path skyscale.adapters.sandbox_reward \
  "$@"

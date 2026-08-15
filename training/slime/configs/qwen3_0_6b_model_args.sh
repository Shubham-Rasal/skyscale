#!/usr/bin/env bash

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

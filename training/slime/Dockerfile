# Upstream slime image resolved on 2026-08-01. The digest, source revision,
# and matching upstream Megatron/SGLang/TE pins make the runtime reproducible.
ARG SLIME_PLATFORM=linux/amd64
FROM --platform=${SLIME_PLATFORM} slimerl/slime@sha256:a97ec147e37bef050337a9b229036eda00b4aa9c4d02b31a0109dc850f8ca342

ARG SLIME_COMMIT=aaf5c2092b01219fa0d5c2d323741d409086ca32
ARG MEGATRON_COMMIT=1dcf0dafa884ad52ffb243625717a3471643e087
ARG SGLANG_VERSION=0.5.15.post1
ARG TRANSFORMER_ENGINE_VERSION=2.16.1
ARG CUDA_VERSION=12.9

LABEL org.opencontainers.image.source="https://github.com/bluequbit/skyscale" \
      dev.skyscale.slime.commit="${SLIME_COMMIT}" \
      dev.skyscale.megatron.commit="${MEGATRON_COMMIT}" \
      dev.skyscale.sglang.version="${SGLANG_VERSION}" \
      dev.skyscale.transformer-engine.version="${TRANSFORMER_ENGINE_VERSION}" \
      dev.skyscale.cuda.version="${CUDA_VERSION}"

RUN cd /root/slime \
    && git fetch --depth=1 origin "${SLIME_COMMIT}" \
    && git checkout --detach "${SLIME_COMMIT}" \
    && pip install -e . --no-deps \
    && test "$(git rev-parse HEAD)" = "${SLIME_COMMIT}"

RUN cd /root/Megatron-LM \
    && git remote get-url origin | grep -q 'NVIDIA/Megatron-LM' \
    && git fetch --depth=1 origin "${MEGATRON_COMMIT}" \
    && git checkout --detach "${MEGATRON_COMMIT}" \
    && pip install -e . --no-deps \
    && test "$(git rev-parse HEAD)" = "${MEGATRON_COMMIT}"

RUN pip install --no-deps \
      "sglang==${SGLANG_VERSION}" \
      "transformer-engine==${TRANSFORMER_ENGINE_VERSION}"

COPY skyscale /opt/skyscale/slime/skyscale
COPY prepare_model.py /opt/skyscale/slime/prepare_model.py
COPY register_engine.py /opt/skyscale/slime/register_engine.py
COPY runtime_versions.py /opt/skyscale/slime/runtime_versions.py
COPY configs /opt/skyscale/slime/configs

RUN SLIME_COMMIT="${SLIME_COMMIT}" \
    MEGATRON_COMMIT="${MEGATRON_COMMIT}" \
    SGLANG_VERSION="${SGLANG_VERSION}" \
    TRANSFORMER_ENGINE_VERSION="${TRANSFORMER_ENGINE_VERSION}" \
    CUDA_VERSION="${CUDA_VERSION}" \
    python /opt/skyscale/slime/runtime_versions.py

ENV SLIME_COMMIT=${SLIME_COMMIT} \
    MEGATRON_COMMIT=${MEGATRON_COMMIT} \
    SGLANG_VERSION=${SGLANG_VERSION} \
    TRANSFORMER_ENGINE_VERSION=${TRANSFORMER_ENGINE_VERSION} \
    CUDA_VERSION=${CUDA_VERSION} \
    PYTHONPATH="/opt/skyscale/slime:/root/slime:/root/Megatron-LM" \
    PYTHONUNBUFFERED=1
WORKDIR /root/slime

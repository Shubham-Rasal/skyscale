# KubeRay + slime production backend

`backend=slime` is a Kubernetes-native path. It does not use Modal or the
single-container provider `DeploySpec`; `backend=skyscale` remains available
for existing workloads.

## Runtime boundary

- SkyScale persists immutable run snapshots, attempts, ownership, samples,
  policy/checkpoint lineage, usage, and audit records.
- KubeRay owns finite trainer/evaluator execution. The reconciler treats
  RayJob observed status as authoritative; container callbacks are
  supplementary.
- slime owns numerical RL. SGLang rollout Deployments are managed separately
  and register as external engines.
- Grouped sample payloads are immutable object-store objects. PostgreSQL stores
  transactional indexes and leases. SQLite/in-memory storage is local
  development only.
- Sample identity is `(tenant_id, run_id, sample_id)`. The metadata adapter
  migrates the former `sample_rows` table into `rl_grouped_samples_v2` only
  when tenant/run lineage can be recovered from immutable blob keys; ambiguous
  rows fail migration instead of being silently merged.
- Weight manifests, engine acknowledgements, active/last-good versions, and
  parent chains are persisted per run in the metadata database. Artifacts are
  fetched and checksum-verified before materialization. Rollout registrars
  download the verified artifact, invoke SGLang's disk update endpoint, and
  acknowledge the applied checksum before fleet activation.

## Required infrastructure

Production requires:

1. Kubernetes with the KubeRay operator and `ray.io/v1` RayJob CRD.
2. NVIDIA GPU operator/device plugin. Training requests whole GPUs.
3. PostgreSQL via `DATABASE_URL`.
4. S3-compatible storage via `S3_ENDPOINT`, `S3_BUCKET`, credentials, TLS, and
   bucket-side encryption/KMS policy.
5. A secret containing `SKYSCALE_RUNTIME_TOKEN` for the control plane and
   runtime pods, plus model/object-store credentials.
6. Prometheus and, for the sample HPA, a `custom.metrics.k8s.io` adapter.
7. Optional Kueue for queue admission/gang scheduling and a cluster autoscaler
   configured for the selected GPU node pools.

Replace every `REPLACE_*` image/model revision in `deploy/k8s` with an immutable
digest or revision. Apply the base control plane with:

```bash
kubectl apply -k deploy/k8s
```

`tenant-example.yaml`, `model-preparation-job.yaml`,
`rollout-autoscaling-example.yaml`, and `kueue-example.yaml` are deployment
templates and require site-specific values.

The control plane and every tenant namespace must receive the same runtime
token. The control-plane Secret key is `runtime-token`; each tenant's
`skyscale-runtime` Secret exposes it as `SKYSCALE_RUNTIME_TOKEN`. The dashboard
submits a compact slime preset and the control plane expands it into an
immutable `RLRunSpec` using `SKYSCALE_SLIME_IMAGE`, `SKYSCALE_SGLANG_IMAGE`,
`SKYSCALE_MODEL_REVISION`, and `SKYSCALE_MODEL_PVC`.

The model-preparation Job and runtime pods must use the same PVC. Trainer,
Ray head, submitter, rollout, and checkpoint-reporter containers mount it at
`/models` by default. The reporter watches Megatron's checkpoint marker,
creates a content-hashed manifest, and commits optimizer progress and
checkpoint lineage to `/api/rl/v1`.

SkyScale reconciliation is the authoritative rollout scaler. Do not apply the
example HPA to a fleet while controller scaling is enabled, because two writers
to `Deployment.spec.replicas` produce unstable scaling. Queue depth, generation
p95, token throughput, trainer demand, and durable sample backpressure are
reported to `/api/rl/v1/runs/{id}/rollout-metrics`; backpressure, suspension,
weight transitions, and cancellation drain the Deployment to zero.

## One-GPU correctness profile

`training/slime/configs/one_gpu_qwen3_0_6b.sh` fixes TP/PP/CP to 1, requests one
rollout, two responses per prompt, one rollout batch, 256 response tokens, and
a conservative SGLang memory fraction. `prepare_model.py` downloads an
immutable Qwen revision, retains HF/tokenizer files, converts to Megatron
`torch_dist`, and atomically writes a hashed artifact manifest.

The image pins:

- slime `aaf5c2092b01219fa0d5c2d323741d409086ca32`
- upstream Megatron `1dcf0dafa884ad52ffb243625717a3471643e087`
- SGLang `0.5.15.post1`
- Transformer Engine `2.16.1`
- CUDA `12.9`

The Docker build checks out both git repositories at the exact revisions,
checks exact installed Python distribution versions and CUDA, and writes
`/opt/skyscale/slime/runtime-versions.json`. A mismatch fails the image build.
Build and push the image, then replace its digest in the run contract.

## Recovery and promotion

Suspension sets `RayJob.spec.suspend=true` and rollout replicas to zero;
resumption explicitly clears suspension and restores the controller's desired
replicas. On restart, the reconciler discovers exactly labeled orphan
RayJobs/Deployments/Services/Jobs/PVCs, reconstructs ownership, verifies UIDs,
and waits for foreground deletion before declaring cancellation complete.

Committed optimizer/RNG resume checkpoints remain distinct from serving weight
artifacts. Missing in-memory run pointers are recovered from the latest
committed checkpoint. Periodic/final evaluator RayJobs run frozen, hash-checked
suites against the stable rollout service. Passing gates create an isolated
canary Deployment and Service plus a canary evaluator RayJob. The controller
promotes only after canary gates pass; failures durably request rollback to the
last-good version.

## Validation

Run all local contract, lease, lineage, delta-sync, async-policy, reconciliation,
multi-tenant quota, rendering, and adapter checks:

```bash
bash scripts/validation/validate_slime_local.sh
```

Hardware/infrastructure-gated validation is intentionally separate from local
tests:

- M1: one 24 GB NVIDIA L4 has passed rollout →
  sandbox reward → optimizer step → checkpoint → cleanup.
- M2: two GPUs, PostgreSQL, and object storage for trainer/rollout
  disaggregation and restart recovery.
- M3: two GPUs for full/delta publication, canary acknowledgements, checksum
  failure, rollback, and parent-chain retention.
- M4: two GPUs for synchronous-baseline numerical comparison, bounded
  staleness, cancellation, and convergence gates.
- M5: at least three GPUs for concurrent tenants, quota contention,
  autoscaling, isolation, and cost-accounting soak tests.
- M6: multi-node RDMA/NCCL infrastructure for topology-aware scale and node
  failure injection.

Before promotion, inject pod/node loss, API restart, object-store interruption,
failed delta apply, stale replay, and cancellation during publication. Preserve
logs externally before RayJob TTL cleanup. Promotion must pass configured
reward/pass-rate/KL/entropy/regression/safety and weight-health gates.

## AWS spot GPU (M1 bootstrap)

Use the bundled [`aws-gpu-spot`](../.claude/skills/aws-gpu-spot/SKILL.md) skill to
validate GPU access, build the pinned slime runtime, and optionally prepare
Qwen3-0.6B on a single EC2 box before standing up EKS/KubeRay.

This path does **not** replace production Kubernetes. It proves:

- NVIDIA driver + container toolkit on a GPU instance
- PyTorch DLAMI CUDA smoke (`gpu.py smoke`)
- slime Docker build and `runtime_versions.py` pin verification
- optional `prepare_model.py` when `HF_TOKEN` is set

### Setup

```bash
cp .env.aws.example .env.aws   # fill AWS keys; region must be us-east-1 (not "global")
set -a && source .env.aws && set +a
pip install boto3
chmod +x scripts/validation/aws_slime_gpu.sh scripts/validation/aws_remote_bootstrap.sh
```

### Run (always terminates the instance)

```bash
bash scripts/validation/aws_slime_gpu.sh
```

Defaults: spot-first `g5.xlarge` (A10G 24 GB), 200 GiB root volume, instance name
`skyscale-slime`. On capacity/quota failure the script retries `p5.4xlarge`.
Override with `AWS_GPU_TYPE`, `AWS_GPU_FALLBACK_TYPE`, `AWS_GPU_NAME`.

Manual equivalents:

```bash
GPU=.claude/skills/aws-gpu-spot/gpu.py
python3 "$GPU" launch --name skyscale-slime --type g5.xlarge --volume-size 200
python3 "$GPU" smoke --name skyscale-slime
# ... upload repo and run aws_remote_bootstrap.sh on the instance ...
python3 "$GPU" terminate --name skyscale-slime
```

The EC2 path now proves full M1 numerical training (`train.py` + sandbox
rewards) in a single Docker container. It does not validate KubeRay
reconciliation, PostgreSQL/S3 persistence, multi-pod weight publication, or
restart recovery; those remain cluster-gated M2+ checks.
Rotate any AWS keys that were ever pasted into chat or logs.

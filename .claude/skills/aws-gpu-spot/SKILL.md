---
name: aws-gpu-spot
description: Launch, use, and tear down AWS GPU instances (spot-first, on-demand fallback) using AWS credentials from environment variables. Use when an agent needs a temporary GPU box on AWS — run a CUDA/PyTorch job, smoke-test a GPU, or stand up an instance for training/inference — and must reliably clean it up afterward. Triggers: "spin up a GPU", "rent a GPU on AWS", "run this on a GPU", "AWS spot GPU", "launch a g4dn/g5/g6/p5/g7e", "GPU smoke test".
---

# aws-gpu-spot

Provision AWS GPU instances **spot-first** with automatic Availability-Zone
capacity fallback, run work on them over SSH, and **always tear them down**.

All AWS access is via `boto3` reading credentials **from the environment**.
This skill contains **no secrets** — it is safe to share.

## Credentials (required, from env — never hardcode)

The calling agent MUST have these set in its environment before invoking:

```bash
export AWS_ACCESS_KEY_ID=...          # provided out-of-band, NOT stored here
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1   # optional; default us-east-1
```

If either key is missing, every command exits with a clear error. Do not paste
credentials into files, commits, or logs. Treat them as sensitive; if they leak,
they must be rotated by the account owner.

Requirements: `python3` with `boto3`, plus `ssh`/`curl` on PATH.
(`pip install boto3` if needed.)

## Usage

Run everything through the bundled CLI (`gpu.py` lives next to this file):

```bash
python3 gpu.py whoami                         # verify creds -> account + IAM ARN
python3 gpu.py list-gpus --spot               # GPU types + current spot $/hr
python3 gpu.py launch --name job1 --type g4dn.xlarge   # spot, AZ-fallback, waits for SSH
python3 gpu.py smoke --name job1              # PyTorch/CUDA matmul smoke test
python3 gpu.py run --name job1 -- "nvidia-smi"         # arbitrary command over SSH
python3 gpu.py status                         # list all managed instances
python3 gpu.py terminate --name job1          # terminate + delete key + SG
python3 gpu.py cleanup-all                    # nuke ALL managed instances/keys/SGs
```

`launch` defaults: `--market spot-then-ondemand` (try spot in every AZ, then
on-demand), latest **PyTorch Deep Learning AMI** (`/opt/pytorch` venv preinstalled),
120 GB gp3 root, SSH locked to the caller's detected public IP. Override with
`--type`, `--market {spot,on-demand,spot-then-ondemand}`, `--ami`, `--azs`,
`--volume-size`, `--ssh-cidr`, `--name` (run several boxes with distinct names).

## The golden rule: always terminate

GPU instances bill by the second (T4 spot ~$0.26/hr → g7e.24xlarge spot ~$7/hr).
**Every launch must be paired with a terminate.** After any run — success or
failure — call `terminate` (or `cleanup-all`). `status` shows anything still alive.
State (instance id, key `.pem`, SG id) is saved under `~/.cache/aws-gpu-spot/`.

## How it behaves (and why)

- **Spot-first with AZ sweep.** Spot capacity varies by AZ, so `launch` tries each
  AZ in turn. `InsufficientInstanceCapacity` → next AZ. First success wins.
- **Cleanup ordering matters.** On terminate: kill the instance, delete the key
  immediately, but the security group can only be deleted **after** the instance
  fully terminates (its network interface must detach first) — the script waits
  and retries. Don't delete the SG early; you'll get `DependencyViolation`.
- **Tagging.** Everything is tagged `ManagedBy=aws-gpu-spot` so `status` /
  `cleanup-all` can always find and reclaim it, even across sessions.
- **SSH.** User is `ubuntu`; host-key checking is disabled (ephemeral boxes).

## Interpreting launch errors

| Error | Meaning | Fix |
|---|---|---|
| `InsufficientInstanceCapacity` | No spot capacity in that AZ right now | Script auto-tries next AZ; or use `--market spot-then-ondemand` / a different `--type` |
| `MaxSpotInstanceCountExceeded` | **Spot vCPU quota** too low for that instance family | Account owner raises the per-family **Spot** quota (see below) |
| `VcpuLimitExceeded` | **On-demand vCPU quota** too low for that family | Account owner raises the **On-Demand** G/P quota |
| `AuthFailure.ServiceLinkedRoleCreationNotPermitted` | EC2-Spot service-linked role missing | Account owner runs `aws iam create-service-linked-role --aws-service-name spot.amazonaws.com` once |

### AWS quota facts (learned the hard way)
Spot and on-demand quotas are **separate**, and spot quotas are **per instance
family** — raising one family/type does nothing for another:

- **On-Demand G & VT** (T4/L4/A10G/L40S/RTX-PRO): quota code `L-DB2E81BA`
- **On-Demand P** (V100/A100/H100/H200/B200): `L-417A185B`
- **Spot — G & VT** (all `g*` GPU types): quota code **`L-3819A6DF`**  ← GPU spot
- **Spot — Standard** (CPU c/m/r/t…): `L-34B43A08`  ← *not* GPUs; a common mix-up
- **Spot — P**: separate again

Quotas are **per region**. A vCPU count is what's limited (e.g. `g4dn.xlarge`=4 vCPU;
`g7e.24xlarge`=96 vCPU). To raise:
`aws service-quotas request-service-quota-increase --service-code ec2 --quota-code <CODE> --desired-value <vCPUs>`
(needs AWS approval; can take minutes to a day). Reading quotas needs
`servicequotas:GetServiceQuota`, which the runtime creds may not have — in that
case, diagnose from the launch error codes above.

## Instance quick-reference (us-east-1, single-GPU)

| Type | GPU | VRAM | ~spot $/hr |
|---|---|---|---|
| `g4dn.xlarge` | T4 | 16 GB | ~0.26 |
| `g6.xlarge` | L4 | 22 GB | ~0.39 |
| `g5.xlarge` | A10G | 22 GB | ~0.47 |
| `g6e.xlarge` | L40S | 44 GB | ~1.86 |
| `p5.4xlarge` | H100 | 80 GB | ~2.63 |
| `g7e.2xlarge` | RTX PRO 6000 | 96 GB | (large) |

Prices drift — run `list-gpus --spot` for live numbers.

## Notes for non-smoke workloads

- `smoke` assumes the **PyTorch DLAMI** (activates `/opt/pytorch`). For Docker /
  custom images, launch with `--ami <base-DLAMI>` and drive it via
  `run --name N -- "<docker/build commands>"`.
- For big multi-GPU boxes (e.g. `g7e.24xlarge`), the whole family spot quota may be
  consumed by one instance — check `status` before launching another.

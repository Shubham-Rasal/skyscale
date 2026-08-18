"""SkyScale adapters for slime task sampling, rewards, and grouped samples."""

from __future__ import annotations

import asyncio
import json
import os
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any


class SkyScaleAPIError(RuntimeError):
    pass


@dataclass(frozen=True)
class SkyScaleClient:
    base_url: str
    run_id: str
    token: str | None = None
    timeout: float = 120

    @classmethod
    def from_env(cls) -> "SkyScaleClient":
        return cls(
            base_url=os.environ["SKYSCALE_CONTROL_PLANE_URL"].rstrip("/"),
            run_id=os.environ["SKYSCALE_RUN_ID"],
            token=os.environ.get("SKYSCALE_RUNTIME_TOKEN"),
        )

    def post(self, path: str, payload: dict[str, Any]) -> dict[str, Any]:
        request = urllib.request.Request(
            self.base_url + path,
            data=json.dumps(payload, separators=(",", ":")).encode(),
            headers={
                "Content-Type": "application/json",
                **({"Authorization": f"Bearer {self.token}"} if self.token else {}),
            },
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            detail = error.read(4096).decode(errors="replace")
            raise SkyScaleAPIError(f"{path} returned HTTP {error.code}: {detail}") from error

    def publish_grouped_sample(self, envelope: dict[str, Any]) -> dict[str, Any]:
        """Publish the native token/mask/log-prob envelope without flattening it."""
        required = {
            "api_version", "tenant_id", "project_id", "attempt_id", "rollout_id",
            "prompt_group_id", "sample_id", "prompt_token_ids", "response_token_ids",
            "loss_mask", "reward_components", "policy_version", "environment_version",
            "generated_at",
        }
        missing = sorted(required - envelope.keys())
        if missing:
            raise ValueError(f"grouped sample fields missing: {', '.join(missing)}")
        return self.post("/api/rl/v1/samples", {**envelope, "run_id": self.run_id})


class SkyScaleDataSource:
    """Task source preserving slime's grouped sample structure."""

    def __init__(self, client: SkyScaleClient | None = None, seed: int = 42):
        self.client = client or SkyScaleClient.from_env()
        self.seed = seed
        self.cursor = 0

    def sample(self, count: int) -> list[dict[str, Any]]:
        tasks = []
        for _ in range(count):
            task = self.client.post(
                "/api/rl/env/tasks/sample",
                {"run_id": self.client.run_id, "seed": self.seed + self.cursor},
            )
            self.cursor += 1
            tasks.append(
                {
                    "prompt": task["prompt"],
                    "metadata": {
                        "task_id": task["task_id"],
                        "environment_version": task["environment_version"],
                        "dataset_cursor": self.cursor - 1,
                    },
                    "status": "pending",
                }
            )
        return tasks


async def generate_from_skyscale(args: Any, sample: Any, sampling_params: dict[str, Any]) -> Any:
    """Fetch a SkyScale task, then delegate token generation to slime/SGLang."""
    api = SkyScaleClient.from_env()
    sample_index = int(sample.index or 0)
    rollout_seed = int(getattr(args, "rollout_seed", 42))
    task = await asyncio.to_thread(
        api.post,
        "/api/rl/env/tasks/sample",
        {"run_id": api.run_id, "seed": rollout_seed + sample_index},
    )
    sample.prompt = task["prompt"]
    sample.metadata = {
        **(sample.metadata or {}),
        "task_id": task["task_id"],
        "environment_version": task["environment_version"],
        "dataset_cursor": sample_index,
    }

    # Import lazily to keep the adapter importable in local unit tests that do
    # not install slime.
    from slime.rollout.sglang_rollout import generate

    return await generate(args, sample, sampling_params)


def sandbox_reward(sample: Any, client: SkyScaleClient | None = None) -> dict[str, Any]:
    """Atomic deterministic reward hook; sandbox cleanup is server-side."""
    api = client or SkyScaleClient.from_env()
    metadata = sample["metadata"] if isinstance(sample, dict) else sample.metadata
    response = sample.get("response", "") if isinstance(sample, dict) else sample.response
    result = api.post(
        "/api/rl/env/evaluate",
        {"run_id": api.run_id, "task_id": metadata["task_id"], "code": response},
    )
    return {
        "reward": result["reward"],
        "reward_components": result["reward_components"],
        "status": "complete",
        "environment_version": result["environment_version"],
        "metadata": {
            **metadata,
            "passed_tests": result["passed_tests"],
            "total_tests": result["total_tests"],
        },
    }


async def async_sandbox_reward(args: Any, sample: Any, **_: Any) -> float:
    """slime-compatible async reward hook with the `(args, sample)` contract."""
    api = SkyScaleClient.from_env()
    result = await asyncio.to_thread(
        api.post,
        "/api/rl/env/evaluate",
        {
            "run_id": api.run_id,
            "task_id": sample.metadata["task_id"],
            "code": sample.response,
        },
    )
    sample.metadata = {
        **(sample.metadata or {}),
        "reward_components": result["reward_components"],
        "environment_version": result["environment_version"],
        "passed_tests": result["passed_tests"],
        "total_tests": result["total_tests"],
    }
    return float(result["reward"])


def publish_rollout_samples(args: Any, samples: list[list[Any]], data_source: Any) -> None:
    """Publish slime's native grouped rollout samples to SkyScale."""
    attempt_id = os.environ.get("SKYSCALE_ATTEMPT_ID")
    if not attempt_id:
        return
    client = getattr(data_source, "client", None) or SkyScaleClient.from_env()
    tenant_id = os.environ["SKYSCALE_TENANT_ID"]
    project_id = os.environ["SKYSCALE_PROJECT_ID"]
    generated_at = datetime.now(timezone.utc).isoformat()
    for group_position, group in enumerate(samples):
        for sample_position, sample in enumerate(group):
            response_length = int(sample.response_length)
            prompt_length = len(sample.tokens) - response_length
            metadata = sample.metadata or {}
            rollout_id = sample.rollout_id if sample.rollout_id is not None else sample.index
            group_id = sample.group_index if sample.group_index is not None else group_position
            sample_id = sample.index if sample.index is not None else sample_position
            versions = list(sample.weight_versions or [])
            reward_components = metadata.get("reward_components")
            if not isinstance(reward_components, dict):
                reward_components = {"total": float(sample.reward or 0)}
            client.publish_grouped_sample(
                {
                    "api_version": "rl.skyscale.dev/v1alpha1",
                    "tenant_id": tenant_id,
                    "project_id": project_id,
                    "attempt_id": attempt_id,
                    "rollout_id": str(rollout_id),
                    "prompt_group_id": str(group_id),
                    "sample_id": str(sample_id),
                    "prompt_token_ids": list(sample.tokens[:prompt_length]),
                    "response_token_ids": list(sample.tokens[prompt_length:]),
                    "response_start": prompt_length,
                    "loss_mask": list(sample.loss_mask or [1] * response_length),
                    "behavior_log_probs": list(sample.rollout_log_probs or []),
                    "reward_components": reward_components,
                    "status": getattr(sample.status, "value", str(sample.status)),
                    "policy_version": versions[-1] if versions else "initial",
                    "environment_version": metadata["environment_version"],
                    "generated_at": generated_at,
                }
            )

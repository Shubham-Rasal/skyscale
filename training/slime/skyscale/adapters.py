"""SkyScale adapters for slime task sampling, rewards, and grouped samples."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from dataclasses import dataclass
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

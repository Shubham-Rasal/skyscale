import unittest
from types import SimpleNamespace
from unittest.mock import patch

from skyscale.adapters import (
    SkyScaleClient,
    SkyScaleDataSource,
    publish_rollout_samples,
    sandbox_reward,
)


class FakeClient:
    run_id = "run"

    def __init__(self):
        self.calls = []

    def post(self, path, payload):
        self.calls.append((path, payload))
        if path.endswith("/sample"):
            return {
                "task_id": "task-1",
                "prompt": "solve",
                "environment_version": "env-v1",
            }
        return {
            "reward": 1.0,
            "reward_components": {"tests": 1.0},
            "environment_version": "env-v1",
            "passed_tests": 2,
            "total_tests": 2,
        }


class AdapterTests(unittest.TestCase):
    def test_data_source_has_deterministic_cursor_and_lineage(self):
        client = FakeClient()
        source = SkyScaleDataSource(client=client, seed=10)
        rows = source.sample(2)
        self.assertEqual([row["metadata"]["dataset_cursor"] for row in rows], [0, 1])
        self.assertEqual([call[1]["seed"] for call in client.calls], [10, 11])

    def test_reward_preserves_components_and_environment(self):
        result = sandbox_reward(
            {"response": "def answer(): pass", "metadata": {"task_id": "task-1"}},
            client=FakeClient(),
        )
        self.assertEqual(result["reward"], 1.0)
        self.assertEqual(result["environment_version"], "env-v1")
        self.assertEqual(result["metadata"]["passed_tests"], 2)

    def test_grouped_sample_requires_native_fields(self):
        client = SkyScaleClient("http://example", "run")
        with self.assertRaises(ValueError):
            client.publish_grouped_sample({"sample_id": "sample"})

    def test_rollout_hook_preserves_tokens_masks_and_lineage(self):
        class GroupedClient:
            def __init__(self):
                self.envelopes = []

            def publish_grouped_sample(self, envelope):
                self.envelopes.append(envelope)

        client = GroupedClient()
        sample = SimpleNamespace(
            response_length=2,
            tokens=[10, 11, 12, 20, 21],
            metadata={"environment_version": "code-v1", "reward_components": {"tests": 1}},
            rollout_id=4,
            group_index=2,
            index=7,
            weight_versions=["policy-3"],
            reward=1.0,
            loss_mask=[1, 0],
            rollout_log_probs=[-0.1, -0.2],
            status=SimpleNamespace(value="complete"),
        )
        with patch.dict(
            "os.environ",
            {
                "SKYSCALE_ATTEMPT_ID": "attempt-1",
                "SKYSCALE_TENANT_ID": "tenant-a",
                "SKYSCALE_PROJECT_ID": "project-a",
            },
            clear=False,
        ):
            publish_rollout_samples(None, [[sample]], SimpleNamespace(client=client))
        envelope = client.envelopes[0]
        self.assertEqual(envelope["prompt_token_ids"], [10, 11, 12])
        self.assertEqual(envelope["response_token_ids"], [20, 21])
        self.assertEqual(envelope["loss_mask"], [1, 0])
        self.assertEqual(envelope["policy_version"], "policy-3")


if __name__ == "__main__":
    unittest.main()

import unittest

from skyscale.adapters import SkyScaleClient, SkyScaleDataSource, sandbox_reward


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


if __name__ == "__main__":
    unittest.main()

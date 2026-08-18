import json
from pathlib import Path
import tempfile
import unittest

from skyscale.reporter import checkpoint_manifest, latest_step, report_checkpoint


class FakeClient:
    run_id = "run-a"

    def __init__(self):
        self.calls = []

    def post(self, path, payload):
        self.calls.append((path, payload))
        return {}


class ReporterTests(unittest.TestCase):
    def test_manifest_is_stable_and_excludes_reporter_output(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "weights.bin").write_bytes(b"weights")
            (root / "skyscale-checkpoint-manifest.json").write_text("old")
            manifest, digest = checkpoint_manifest(root)
            self.assertEqual([item["path"] for item in manifest["files"]], ["weights.bin"])
            self.assertEqual(len(digest), 64)

    def test_checkpoint_reports_progress_and_lineage(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "latest_checkpointed_iteration.txt").write_text("7")
            (root / "weights.bin").write_bytes(b"weights")
            self.assertEqual(latest_step(root), 8)

            client = FakeClient()
            report_checkpoint(client, "attempt-1", root, 8)

            self.assertEqual(client.calls[0][0], "/api/rl/v1/runs/run-a/trainer-progress")
            checkpoint = client.calls[1][1]
            self.assertEqual(checkpoint["policy_version"], "run-a-step-8")
            self.assertEqual(checkpoint["resume_uri"], str(root))
            self.assertEqual(len(checkpoint["manifest_sha256"]), 64)
            saved = json.loads((root / "skyscale-checkpoint-manifest.json").read_text())
            self.assertEqual(saved["version"], 1)


if __name__ == "__main__":
    unittest.main()

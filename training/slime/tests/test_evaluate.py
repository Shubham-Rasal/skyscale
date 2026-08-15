import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from skyscale.evaluate import generated_text, load_suite


class EvaluatorTests(unittest.TestCase):
    def test_frozen_suite_hash_is_enforced(self):
        payload = json.dumps([{"task_id": "t", "prompt": "solve"}]).encode()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "suite.json"
            path.write_bytes(payload)
            suite = load_suite("file://" + str(path), hashlib.sha256(payload).hexdigest())
            self.assertEqual(suite[0]["task_id"], "t")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                load_suite("file://" + str(path), "wrong")

    def test_sglang_generated_text_contract(self):
        self.assertEqual(generated_text({"text": ["answer"]}), "answer")
        with self.assertRaisesRegex(ValueError, "generated text"):
            generated_text({})


if __name__ == "__main__":
    unittest.main()

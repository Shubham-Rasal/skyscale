from pathlib import Path
import unittest


class RuntimePinTests(unittest.TestCase):
    def test_build_checks_every_numerical_dependency(self):
        dockerfile = (Path(__file__).parents[1] / "Dockerfile").read_text()
        required = {
            "slime": "git checkout --detach \"${SLIME_COMMIT}\"",
            "megatron": "git checkout --detach \"${MEGATRON_COMMIT}\"",
            "sglang": 'SGLANG_VERSION="${SGLANG_VERSION}"',
            "transformer-engine": 'TRANSFORMER_ENGINE_VERSION="${TRANSFORMER_ENGINE_VERSION}"',
            "cuda": 'CUDA_VERSION="${CUDA_VERSION}"',
            "validator": "python /opt/skyscale/slime/runtime_versions.py",
        }
        for dependency, command in required.items():
            self.assertIn(command, dockerfile, f"{dependency} is not build-verified")

    def test_runtime_validator_uses_exact_distribution_versions(self):
        validator = (Path(__file__).parents[1] / "runtime_versions.py").read_text()
        self.assertIn('value != expected[key]', validator)
        self.assertIn('git_revision("/root/Megatron-LM")', validator)
        self.assertIn('git_revision("/root/slime")', validator)


if __name__ == "__main__":
    unittest.main()

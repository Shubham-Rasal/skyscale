#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

(
  cd "${ROOT}/control-plane"
  go test ./contracts ./dataplane ./k8s ./observability ./rlservice ./state ./weights
  go test -race ./contracts ./dataplane ./k8s ./observability ./rlservice ./state ./weights
  go vet ./contracts ./dataplane ./k8s ./observability ./rlservice ./state ./weights
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...
)

(
  cd "${ROOT}/training/slime"
  PYTHONPATH=. python3 -m unittest discover -s tests -v
  python3 -m py_compile prepare_model.py register_engine.py runtime_versions.py skyscale/adapters.py skyscale/evaluate.py
  bash -n configs/*.sh
  docker build --check -f Dockerfile .
)

kubectl kustomize "${ROOT}/deploy/k8s" >/dev/null

echo "local slime/KubeRay contract, data-plane, rendering, and runtime validation passed"

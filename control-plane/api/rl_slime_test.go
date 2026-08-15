package api

import (
	"net/http/httptest"
	"testing"
)

func TestSlimeSpecFromPresetBuildsProductionContract(t *testing.T) {
	t.Setenv("SKYSCALE_SLIME_IMAGE", "ghcr.io/skyscale/slime-runtime")
	t.Setenv("SKYSCALE_SLIME_IMAGE_DIGEST", "sha256:deadbeef")
	t.Setenv("SKYSCALE_SGLANG_IMAGE", "ghcr.io/skyscale/sglang@sha256:cafe")
	request := httptest.NewRequest("POST", "/api/rl/runs", nil)
	request.Header.Set("X-Skyscale-Tenant", "tenant-a")
	request.Header.Set("X-Skyscale-Project", "project-a")

	spec := slimeSpecFromPreset(slimeRunPreset{
		BaseModel: "Qwen/Qwen3-0.6B", NumWorkers: 2, GPUModel: "l4", ProblemSet: "default",
	}, request)

	if spec.Metadata.TenantID != "tenant-a" || spec.Metadata.ProjectID != "project-a" {
		t.Fatalf("unexpected tenancy: %#v", spec.Metadata)
	}
	if spec.Topology.Mode != "disaggregated" || !spec.Topology.Rollout.External || spec.Topology.Rollout.Replicas != 2 {
		t.Fatalf("unexpected rollout topology: %#v", spec.Topology)
	}
	if spec.Model.VolumeClaim != "qwen3-0-6b-models" || spec.Image.Digest != "sha256:deadbeef" {
		t.Fatalf("runtime artifacts are not configured: model=%#v image=%#v", spec.Model, spec.Image)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("generated preset must be valid: %v", err)
	}
}

func TestSlimeSingleWorkerPresetUsesOneGPUColocated(t *testing.T) {
	t.Setenv("SKYSCALE_SLIME_IMAGE", "ghcr.io/skyscale/slime-runtime@sha256:deadbeef")
	request := httptest.NewRequest("POST", "/api/rl/runs", nil)
	spec := slimeSpecFromPreset(slimeRunPreset{
		BaseModel: "Qwen/Qwen3-0.6B", NumWorkers: 1, GPUModel: "l4",
	}, request)
	if spec.Topology.Mode != "colocated" || spec.Topology.Rollout.External {
		t.Fatalf("single-GPU preset must colocate trainer and rollout: %#v", spec.Topology)
	}
}

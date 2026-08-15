package k8s

import (
	"strings"
	"testing"

	"github.com/bluequbit/faas/control-plane/contracts"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func renderSpec(mode string) contracts.RLRunSpec {
	s := contracts.DefaultRunSpec()
	s.Metadata = contracts.ObjectMeta{TenantID: "tenant-a", ProjectID: "project-a", RunID: "run-a"}
	s.Image.Digest = "sha256:deadbeef"
	s.Image.SGLang = "ghcr.io/skyscale/sglang@sha256:abc"
	s.Security.ServiceAccountName = "skyscale-rl-runner"
	s.Topology.Mode = mode
	if mode == "disaggregated" {
		s.Topology.Rollout.External = true
		s.Topology.Rollout.Resources.GPUs = 1
	}
	return s
}

func TestRenderColocatedRayJobUsesWholeGPU(t *testing.T) {
	job := RenderRayJob(renderSpec("colocated"), "attempt-1")
	if job.GetAPIVersion() != "ray.io/v1" || job.GetKind() != "RayJob" {
		t.Fatalf("unexpected resource: %s %s", job.GetAPIVersion(), job.GetKind())
	}
	gpu, found, err := unstructured.NestedInt64(job.Object, "spec", "rayClusterSpec", "workerGroupSpecs", "0", "template")
	if err == nil && found && gpu != 0 {
		t.Fatal("unexpected path behavior")
	}
	raw := job.Object["spec"].(map[string]any)["rayClusterSpec"].(map[string]any)["workerGroupSpecs"].([]any)[0].(map[string]any)
	template := raw["template"].(map[string]any)
	container := template["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
	limit := container["resources"].(map[string]any)["limits"].(map[string]any)["nvidia.com/gpu"]
	if limit != int64(1) {
		t.Fatalf("expected one whole GPU, got %#v", limit)
	}
}

func TestDisaggregatedTrainerRegistersExternalEngine(t *testing.T) {
	job := RenderRayJob(renderSpec("disaggregated"), "attempt-1")
	entrypoint, _, _ := unstructured.NestedString(job.Object, "spec", "entrypoint")
	if !strings.Contains(entrypoint, "ROLLOUT_EXTERNAL_ENGINE_ADDRS=") || !strings.Contains(entrypoint, "run-a-rollout") {
		t.Fatalf("missing external engine registration: %s", entrypoint)
	}
	if !strings.Contains(entrypoint, "disaggregated_qwen3_0_6b.sh") {
		t.Fatalf("trainer does not use the validated disaggregated profile: %s", entrypoint)
	}
	deployment := RenderRolloutDeployment(renderSpec("disaggregated"), "policy-7")
	if deployment.GetKind() != "Deployment" {
		t.Fatalf("unexpected kind %s", deployment.GetKind())
	}
}

func TestNamespaceIsolationIsStable(t *testing.T) {
	if got := NamespaceFor("Tenant_A", "Project A"); got != "skyscale-tenant-a-project-a" {
		t.Fatalf("unexpected namespace %q", got)
	}
}

func TestAsyncAndRecoveryEntrypoint(t *testing.T) {
	spec := renderSpec("disaggregated")
	spec.Algorithm.Strategy = "asynchronous"
	spec.Checkpoint.ResumeFrom = "s3://checkpoints/run/step-7"
	job := RenderRayJob(spec, "attempt-2")
	entrypoint, _, _ := unstructured.NestedString(job.Object, "spec", "entrypoint")
	for _, expected := range []string{"async_qwen3_0_6b.sh", "--load", "s3://checkpoints/run/step-7"} {
		if !strings.Contains(entrypoint, expected) {
			t.Fatalf("entrypoint missing %q: %s", expected, entrypoint)
		}
	}
}

func TestRuntimePodsMountPreparedModelArtifacts(t *testing.T) {
	spec := renderSpec("disaggregated")
	job := RenderRayJob(spec, "attempt-1")
	raw := job.Object["spec"].(map[string]any)["rayClusterSpec"].(map[string]any)["workerGroupSpecs"].([]any)[0].(map[string]any)
	podSpec := raw["template"].(map[string]any)["spec"].(map[string]any)
	volumes := podSpec["volumes"].([]any)
	foundModelVolume := false
	for _, volume := range volumes {
		volumeObject := volume.(map[string]any)
		if claim, ok := volumeObject["persistentVolumeClaim"].(map[string]any); ok {
			if claim["claimName"] != "qwen3-0-6b-models" {
				t.Fatalf("unexpected model claim: %#v", claim)
			}
			foundModelVolume = true
		}
	}
	if !foundModelVolume {
		t.Fatalf("model PVC volume is missing: %#v", volumes)
	}
	head := job.Object["spec"].(map[string]any)["rayClusterSpec"].(map[string]any)["headGroupSpec"].(map[string]any)
	headContainers := head["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
	if len(headContainers) != 2 || headContainers[1].(map[string]any)["name"] != "skyscale-reporter" {
		t.Fatalf("checkpoint reporter is not attached to the Ray head: %#v", headContainers)
	}
	deployment := RenderRolloutDeployment(spec, "policy-7")
	deploymentSpec := deployment.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	args := deploymentSpec["containers"].([]any)[0].(map[string]any)["args"].([]any)
	if !strings.Contains(strings.Join(anyStrings(args), " "), "/models/hf") {
		t.Fatalf("rollout does not use prepared model artifacts: %v", args)
	}
}

func TestEvaluatorAndCanaryResourcesAreIsolated(t *testing.T) {
	spec := renderSpec("disaggregated")
	spec.Evaluation = contracts.EvaluationPolicy{
		SuiteURI: "https://signed.example/suite.json", SuiteHash: "hash", CanaryPercent: 10,
	}
	evaluator := RenderEvaluatorRayJobPhase(spec, "eval-1", "v2", "canary")
	entrypoint, _, _ := unstructured.NestedString(evaluator.Object, "spec", "entrypoint")
	if !strings.Contains(entrypoint, "--phase' 'canary") || !strings.Contains(entrypoint, "rollout-canary") {
		t.Fatalf("canary evaluator entrypoint is not isolated: %s", entrypoint)
	}
	canary := RenderCanaryRolloutDeployment(spec, "v2", 1, false)
	if canary.GetName() != spec.Metadata.RunID+"-rollout-canary" {
		t.Fatalf("unexpected canary deployment name: %s", canary.GetName())
	}
	selector, _, _ := unstructured.NestedStringMap(canary.Object, "spec", "selector", "matchLabels")
	if selector["rl.skyscale.dev/role"] != "rollout-canary" {
		t.Fatalf("canary selector can route stable traffic: %v", selector)
	}
	service := RenderCanaryRolloutService(spec)
	serviceSelector, _, _ := unstructured.NestedStringMap(service.Object, "spec", "selector")
	if serviceSelector["rl.skyscale.dev/role"] != "rollout-canary" {
		t.Fatalf("canary service selector mismatch: %v", serviceSelector)
	}
}

func TestRolloutControllerOwnsReplicasAndWeightVolume(t *testing.T) {
	spec := renderSpec("disaggregated")
	deployment := RenderRolloutDeploymentState(spec, "v2", 0, true)
	replicas, _, _ := unstructured.NestedInt64(deployment.Object, "spec", "replicas")
	if replicas != 0 || deployment.GetAnnotations()["rl.skyscale.dev/autoscaler"] != "skyscale-controller" {
		t.Fatalf("controller scaling metadata missing: replicas=%d annotations=%v", replicas, deployment.GetAnnotations())
	}
	containers, _, _ := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	if len(containers) != 2 {
		t.Fatalf("expected engine and registrar: %d", len(containers))
	}
	for _, raw := range containers {
		container := raw.(map[string]any)
		if len(container["volumeMounts"].([]any)) == 0 {
			t.Fatalf("container %s cannot access materialized weights", container["name"])
		}
	}
}

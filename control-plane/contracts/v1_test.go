package contracts

import (
	"testing"
	"time"
)

func validSpec() RLRunSpec {
	s := DefaultRunSpec()
	s.Metadata = ObjectMeta{TenantID: "tenant-a", ProjectID: "project-a"}
	s.Image.Digest = "sha256:test"
	s.CreatedAt = time.Unix(100, 0).UTC()
	return s
}

func TestSnapshotDeterministic(t *testing.T) {
	a := validSpec()
	a.Security.SecretRefs = []string{"z", "a"}
	b := validSpec()
	b.Security.SecretRefs = []string{"a", "z"}

	sa, err := Snapshot(a)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := Snapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if sa.SHA256 != sb.SHA256 {
		t.Fatalf("snapshot hashes differ: %s != %s", sa.SHA256, sb.SHA256)
	}
}

func TestRunValidationRejectsFractionalOrUnsafeConfiguration(t *testing.T) {
	s := validSpec()
	s.Topology.Trainer.Resources.GPUs = 0
	s.Algorithm.Arguments = map[string]string{"--foo;rm": "x"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSampleEnvelopePreservesGroupedNativeFields(t *testing.T) {
	s := SampleEnvelope{
		APIVersion: APIVersion, TenantID: "tenant", ProjectID: "project", RunID: "run",
		AttemptID: "attempt", RolloutID: "rollout", PromptGroupID: "group", SampleID: "sample",
		PolicyVersion: "policy-1", PromptTokenIDs: []int64{1, 2}, ResponseTokenIDs: []int64{3, 4},
		ResponseStart: 2, LossMask: []float32{1, 1}, BehaviorLogProbs: []float32{-0.1, -0.2},
		EnvironmentVersion: "env-1", GeneratedAt: time.Unix(100, 0).UTC(),
	}
	if err := s.Validate(10); err != nil {
		t.Fatal(err)
	}
	s.LossMask = []float32{1}
	if err := s.Validate(10); err == nil {
		t.Fatal("expected loss-mask validation error")
	}
}

func TestSampleEnvelopeRejectsIncompleteLineage(t *testing.T) {
	s := SampleEnvelope{
		APIVersion: APIVersion, RunID: "run", PromptGroupID: "group", SampleID: "sample",
		PolicyVersion: "policy-1", PromptTokenIDs: []int64{1}, ResponseTokenIDs: []int64{2},
		ResponseStart: 1, LossMask: []float32{1},
	}
	if err := s.Validate(10); err == nil {
		t.Fatal("expected incomplete lineage validation error")
	}
}

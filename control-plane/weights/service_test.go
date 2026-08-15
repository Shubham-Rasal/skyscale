package weights

import (
	"context"
	"testing"
	"time"
)

func durableManifest(runID, version, parent, format, uri string, artifact, result []byte, step int64) Manifest {
	checksums := map[string]string{"artifact": checksum(artifact)}
	if result != nil {
		checksums["result"] = checksum(result)
	}
	return Manifest{
		APIVersion: "rl.skyscale.dev/v1alpha1", RunID: runID, Version: version,
		ParentVersion: parent, BaseCheckpoint: "base", Format: format, DType: "bf16",
		Checksums: checksums, ByteSize: int64(len(artifact)), OptimizerStep: step,
		ArtifactURI: uri, CreatedAt: time.Now(),
	}
}

func TestDurableRunScopedDeltaActivationAndRecovery(t *testing.T) {
	ctx := context.Background()
	repository, artifacts := NewMemoryRepository(), NewMemoryArtifactStore()
	service, _ := NewService(repository, artifacts)
	parent := []byte{1, 2, 3, 4}
	target := []byte{5, 6, 7, 8}
	delta := []byte{parent[0] ^ target[0], parent[1] ^ target[1], parent[2] ^ target[2], parent[3] ^ target[3]}
	if err := artifacts.PutImmutable(ctx, "full", parent, checksum(parent)); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.PutImmutable(ctx, "delta", delta, checksum(delta)); err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterEngine(ctx, "run-a", "engine", ""); err != nil {
		t.Fatal(err)
	}
	full := durableManifest("run-a", "v1", "", "full", "full", parent, parent, 1)
	if err := service.Publish(ctx, full, []string{"engine"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, resultHash, err := service.Materialize(ctx, "run-a", "v1"); err != nil || resultHash != checksum(parent) {
		t.Fatalf("materialize full: hash=%s err=%v", resultHash, err)
	}
	if active, err := service.Acknowledge(ctx, "run-a", "engine", "v1", checksum(parent)); err != nil || !active {
		t.Fatalf("activate full: active=%v err=%v", active, err)
	}
	if err := service.MarkGood(ctx, "run-a", "v1"); err != nil {
		t.Fatal(err)
	}
	deltaManifest := durableManifest("run-a", "v2", "v1", "delta", "delta", delta, target, 2)
	if err := service.Publish(ctx, deltaManifest, []string{"engine"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	uri, resultHash, err := service.Materialize(ctx, "run-a", "v2")
	if err != nil || resultHash != checksum(target) {
		t.Fatalf("materialize delta: uri=%s hash=%s err=%v", uri, resultHash, err)
	}
	materialized, _ := artifacts.Fetch(ctx, uri)
	if string(materialized) != string(target) {
		t.Fatalf("delta result mismatch: %v", materialized)
	}
	instruction, err := service.EngineInstruction(ctx, "run-a", "engine")
	if err != nil || instruction.DesiredVersion != "v2" || instruction.SHA256 != resultHash {
		t.Fatalf("engine instruction missing durable publication: %#v err=%v", instruction, err)
	}
	served, servedHash, err := service.MaterializedArtifact(ctx, "run-a", "v2")
	if err != nil || servedHash != resultHash || string(served) != string(target) {
		t.Fatalf("served artifact mismatch: hash=%s payload=%v err=%v", servedHash, served, err)
	}
	if active, err := service.Acknowledge(ctx, "run-a", "engine", "v2", resultHash); err != nil || !active {
		t.Fatalf("activate delta: active=%v err=%v", active, err)
	}

	// A new process/service instance reconstructs active state from durable storage.
	recovered, _ := NewService(repository, artifacts)
	state, err := recovered.State(ctx, "run-a")
	if err != nil || state.Active != "v2" || state.LastGood != "v1" {
		t.Fatalf("recover state: active=%s lastGood=%s err=%v", state.Active, state.LastGood, err)
	}
	rolledBack, err := recovered.Rollback(ctx, "run-a", "canary failed")
	if err != nil || rolledBack != "v1" {
		t.Fatalf("rollback=%s err=%v", rolledBack, err)
	}
	if active, err := recovered.Acknowledge(ctx, "run-a", "engine", "v1", checksum(parent)); err != nil || !active {
		t.Fatalf("engine could not acknowledge rollback: active=%v err=%v", active, err)
	}
	rollbackState, _ := recovered.State(ctx, "run-a")
	if rollbackState.Engines["engine"].Draining {
		t.Fatal("engine remained draining after rollback acknowledgement")
	}
}

func TestWeightStateAndEngineIDsAreRunScoped(t *testing.T) {
	ctx := context.Background()
	repository, artifacts := NewMemoryRepository(), NewMemoryArtifactStore()
	service, _ := NewService(repository, artifacts)
	for _, runID := range []string{"run-a", "run-b"} {
		payload := []byte(runID)
		uri := "artifact-" + runID
		_ = artifacts.PutImmutable(ctx, uri, payload, checksum(payload))
		if err := service.RegisterEngine(ctx, runID, "same-engine", ""); err != nil {
			t.Fatal(err)
		}
		manifest := durableManifest(runID, "v1", "", "full", uri, payload, payload, 1)
		if err := service.Publish(ctx, manifest, []string{"same-engine"}, time.Minute); err != nil {
			t.Fatal(err)
		}
		if _, _, err := service.Materialize(ctx, runID, "v1"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Acknowledge(ctx, runID, "same-engine", "v1", checksum(payload)); err != nil {
			t.Fatal(err)
		}
	}
	stateA, _ := service.State(ctx, "run-a")
	stateB, _ := service.State(ctx, "run-b")
	if stateA.Engines["same-engine"] == stateB.Engines["same-engine"] {
		t.Fatal("engine state leaked between runs")
	}
}

func TestDeltaRejectsWrongParentAndChecksum(t *testing.T) {
	ctx := context.Background()
	service, _ := NewService(NewMemoryRepository(), NewMemoryArtifactStore())
	artifacts := service.artifacts.(*MemoryArtifactStore)
	delta := []byte{1, 2}
	_ = artifacts.PutImmutable(ctx, "delta", delta, checksum(delta))
	_ = service.RegisterEngine(ctx, "run", "engine", "")
	manifest := durableManifest("run", "v2", "missing", "delta", "delta", delta, []byte{3, 4}, 2)
	if err := service.Publish(ctx, manifest, []string{"engine"}, time.Minute); err == nil {
		t.Fatal("expected missing parent rejection")
	}
}

func TestExpiredPublicationRecoversFleetAndGarbageCollectsArtifacts(t *testing.T) {
	ctx := context.Background()
	repository, artifacts := NewMemoryRepository(), NewMemoryArtifactStore()
	service, _ := NewService(repository, artifacts)
	_ = service.RegisterEngine(ctx, "run", "engine", "")
	base := []byte("base")
	_ = artifacts.PutImmutable(ctx, "base", base, checksum(base))
	v1 := durableManifest("run", "v1", "", "full", "base", base, base, 1)
	_ = service.Publish(ctx, v1, []string{"engine"}, time.Minute)
	_, hash, _ := service.Materialize(ctx, "run", "v1")
	_, _ = service.Acknowledge(ctx, "run", "engine", "v1", hash)
	_ = service.MarkGood(ctx, "run", "v1")

	candidate := []byte("candidate")
	_ = artifacts.PutImmutable(ctx, "candidate", candidate, checksum(candidate))
	v2 := durableManifest("run", "v2", "", "full", "candidate", candidate, candidate, 2)
	if err := service.Publish(ctx, v2, []string{"engine"}, time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	_, _, _ = service.Materialize(ctx, "run", "v2")
	time.Sleep(time.Millisecond)
	expired, err := service.Expire(ctx, "run", time.Now())
	if err != nil || len(expired) != 1 || expired[0] != "v2" {
		t.Fatalf("expire candidate: %v err=%v", expired, err)
	}
	state, _ := service.State(ctx, "run")
	if state.Engines["engine"].DesiredVersion != "v1" {
		t.Fatalf("fleet did not recover desired version: %#v", state.Engines["engine"])
	}
	removed, err := service.GarbageCollect(ctx, "run", 0)
	if err != nil || len(removed) != 1 || removed[0].Version != "v2" {
		t.Fatalf("garbage collect failed publication: %#v err=%v", removed, err)
	}
	if _, err := artifacts.Fetch(ctx, "candidate"); err == nil {
		t.Fatal("garbage-collected source artifact still exists")
	}
}

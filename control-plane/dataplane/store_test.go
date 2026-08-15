package dataplane

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
)

func testStore(t *testing.T, high int) *Store {
	t.Helper()
	store, err := New(NewMemoryMetadataStore(), NewMemoryBlobStore(), Options{
		MaxTokens: 32, HighWatermarkGroups: high, LowWatermarkGroups: high - 1, Retention: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func sample(id, group string) contracts.SampleEnvelope {
	return contracts.SampleEnvelope{
		APIVersion: contracts.APIVersion, TenantID: "tenant", ProjectID: "project", RunID: "run",
		AttemptID: "attempt", RolloutID: "rollout", PromptGroupID: group, SampleID: id,
		PromptTokenIDs: []int64{1}, ResponseTokenIDs: []int64{2}, ResponseStart: 1,
		LossMask: []float32{1}, RewardComponents: map[string]float64{"tests": 1},
		Status: "complete", PolicyVersion: "policy-1", EnvironmentVersion: "env-1", GeneratedAt: time.Now(),
	}
}

func TestGroupedLeaseDoesNotConsumePartialGroup(t *testing.T) {
	ctx := context.Background()
	store := testStore(t, 10)
	_, _, _ = store.Put(ctx, sample("a", "group-a"))
	if _, _, err := store.Claim(ctx, "tenant", "run", 2, "trainer", time.Minute); !errors.Is(err, ErrNoCompleteGroup) {
		t.Fatalf("expected partial group to remain unavailable, got %v", err)
	}
	_, _, _ = store.Put(ctx, sample("b", "group-a"))
	lease, payloads, err := store.Claim(ctx, "tenant", "run", 2, "trainer", time.Minute)
	if err != nil || len(payloads) != 2 {
		t.Fatalf("claim complete group: payloads=%d err=%v", len(payloads), err)
	}
	if err := store.Ack(ctx, lease.ID, "trainer"); err != nil {
		t.Fatal(err)
	}
}

func TestDeduplicationAndConflict(t *testing.T) {
	ctx := context.Background()
	store := testStore(t, 10)
	first := sample("same", "group")
	if _, inserted, err := store.Put(ctx, first); err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}
	if _, inserted, err := store.Put(ctx, first); err != nil || inserted {
		t.Fatalf("idempotent insert: inserted=%v err=%v", inserted, err)
	}
	changed := first
	changed.ResponseTokenIDs = []int64{3}
	if _, _, err := store.Put(ctx, changed); !errors.Is(err, ErrDuplicateConflict) {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}
}

func TestBackpressureAndLeaseRecovery(t *testing.T) {
	ctx := context.Background()
	store := testStore(t, 2)
	for i := 0; i < 2; i++ {
		_, _, err := store.Put(ctx, sample(fmt.Sprintf("s-%d", i), fmt.Sprintf("g-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := store.Put(ctx, sample("overflow", "g-3")); !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected backpressure, got %v", err)
	}
	lease, _, err := store.Claim(ctx, "tenant", "run", 1, "trainer", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := store.Ack(ctx, lease.ID, "trainer"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expected expired lease rejection, got %v", err)
	}
	released, _, err := store.Sweep(ctx, time.Now())
	if err != nil || released != 1 {
		t.Fatalf("release expired lease: released=%d err=%v", released, err)
	}
}

func TestSampleIdentityIsTenantAndRunScoped(t *testing.T) {
	ctx := context.Background()
	store := testStore(t, 10)
	first := sample("same", "group")
	if _, inserted, err := store.Put(ctx, first); err != nil || !inserted {
		t.Fatalf("insert first identity: %v", err)
	}
	second := first
	second.TenantID, second.ProjectID, second.RunID = "tenant-b", "project-b", "run-b"
	if _, inserted, err := store.Put(ctx, second); err != nil || !inserted {
		t.Fatalf("same sample ID in another tenant/run must be independent: inserted=%v err=%v", inserted, err)
	}
	if _, rows, err := store.Claim(ctx, "tenant-b", "run-b", 1, "trainer-b", time.Minute); err != nil || len(rows) != 1 {
		t.Fatalf("claim scoped sample: rows=%d err=%v", len(rows), err)
	}
}

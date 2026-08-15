package rlservice

import (
	"testing"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
)

func TestBoundedStalenessActions(t *testing.T) {
	now := time.Now()
	policy := contracts.AlgorithmSpec{MaxPolicyLagSteps: 2, MaxQueueAgeSeconds: 60, OffPolicyAction: "reweight"}
	fresh := EvaluateSample(policy, 10, 9, now.Add(-time.Second), now, 10)
	if fresh.Action != "accept" || fresh.Weight != 1 {
		t.Fatalf("fresh decision: %#v", fresh)
	}
	stale := EvaluateSample(policy, 10, 3, now.Add(-time.Second), now, 10)
	if stale.Action != "reweight" || stale.Weight != 2 {
		t.Fatalf("stale decision: %#v", stale)
	}
}

func TestEvaluationGates(t *testing.T) {
	gates := []contracts.EvaluationGate{{Metric: "pass_rate", Operator: ">=", Threshold: .8}, {Metric: "kl", Operator: "<", Threshold: .2}}
	if err := EvaluateGates(gates, map[string]float64{"pass_rate": .9, "kl": .1}); err != nil {
		t.Fatal(err)
	}
	if err := EvaluateGates(gates, map[string]float64{"pass_rate": .7, "kl": .1}); err == nil {
		t.Fatal("expected failed gate")
	}
}

func TestFairQueueAlternatesTenants(t *testing.T) {
	q := NewFairQueue()
	q.Push("a", "a1")
	q.Push("a", "a2")
	q.Push("b", "b1")
	first, _ := q.Pop()
	second, _ := q.Pop()
	if first != "a1" || second != "b1" {
		t.Fatalf("expected fair order, got %s, %s", first, second)
	}
}

func TestMultiTenantQuotaRejectsConcurrentAndGPUOverage(t *testing.T) {
	topology := contracts.DefaultRunSpec().Topology
	if err := ValidateQuota(2, 0, topology, TenantQuota{MaxConcurrentRuns: 2, MaxGPUs: 8}); err == nil {
		t.Fatal("expected concurrent run quota rejection")
	}
	topology.Mode = "disaggregated"
	topology.Rollout.Resources.GPUs = 2
	topology.Rollout.Replicas = 2
	if err := ValidateQuota(0, 1, topology, TenantQuota{MaxConcurrentRuns: 2, MaxGPUs: 4}); err == nil {
		t.Fatal("expected GPU quota rejection")
	}
}

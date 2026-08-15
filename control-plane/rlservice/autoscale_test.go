package rlservice

import (
	"testing"

	"github.com/bluequbit/faas/control-plane/contracts"
)

func TestRolloutAutoscalingHonorsBoundsAndBackpressure(t *testing.T) {
	topology := contracts.RolloutTopology{MinReplicas: 1, MaxReplicas: 4}
	if got := DesiredRolloutReplicas(topology, 1, RolloutMetrics{QueueDepth: 100, TokensPerSecond: 10}); got != 4 {
		t.Fatalf("expected max scale-up, got %d", got)
	}
	if got := DesiredRolloutReplicas(topology, 4, RolloutMetrics{Backpressured: true}); got != 1 {
		t.Fatalf("expected backpressure scale-down, got %d", got)
	}
}

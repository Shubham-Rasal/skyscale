package rlservice

import (
	"math"

	"github.com/bluequbit/faas/control-plane/contracts"
)

type RolloutMetrics struct {
	QueueDepth       int
	GenerationP95Sec float64
	TokensPerSecond  float64
	TrainerDemand    int
	Backpressured    bool
}

// DesiredRolloutReplicas scales on queue pressure and trainer demand while
// honoring sample-store backpressure and explicit topology bounds.
func DesiredRolloutReplicas(topology contracts.RolloutTopology, current int, metrics RolloutMetrics) int {
	if metrics.Backpressured {
		return topology.MinReplicas
	}
	desired := current
	capacity := math.Max(metrics.TokensPerSecond, 1)
	queueTarget := int(math.Ceil(float64(metrics.QueueDepth) / capacity))
	if queueTarget > desired {
		desired = queueTarget
	}
	if metrics.TrainerDemand > desired {
		desired = metrics.TrainerDemand
	}
	if metrics.GenerationP95Sec > 30 && desired == current {
		desired++
	}
	if desired < topology.MinReplicas {
		desired = topology.MinReplicas
	}
	if desired > topology.MaxReplicas {
		desired = topology.MaxReplicas
	}
	return desired
}

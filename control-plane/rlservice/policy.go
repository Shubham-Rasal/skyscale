package rlservice

import (
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
)

type SampleDecision struct {
	Action    string
	PolicyLag int64
	Age       time.Duration
	Weight    float64
	Reason    string
}

func EvaluateSample(policy contracts.AlgorithmSpec, trainerStep, behaviorStep int64, generatedAt, now time.Time, importanceRatio float64) SampleDecision {
	lag := trainerStep - behaviorStep
	if lag < 0 {
		lag = 0
	}
	age := now.Sub(generatedAt)
	stale := lag > int64(policy.MaxPolicyLagSteps) ||
		(policy.MaxQueueAgeSeconds > 0 && age > time.Duration(policy.MaxQueueAgeSeconds)*time.Second)
	if !stale {
		return SampleDecision{Action: "accept", PolicyLag: lag, Age: age, Weight: 1}
	}
	decision := SampleDecision{Action: policy.OffPolicyAction, PolicyLag: lag, Age: age, Reason: "bounded staleness exceeded"}
	switch policy.OffPolicyAction {
	case "accept":
		decision.Weight = 1
	case "reweight":
		// Truncated importance sampling avoids unbounded variance.
		decision.Weight = math.Max(0.1, math.Min(2.0, importanceRatio))
	case "quarantine", "discard":
		decision.Weight = 0
	default:
		decision.Action = "discard"
	}
	return decision
}

func EvaluateGates(gates []contracts.EvaluationGate, metrics map[string]float64) error {
	for _, gate := range gates {
		value, ok := metrics[gate.Metric]
		if !ok {
			return errors.New("required evaluation metric missing: " + gate.Metric)
		}
		pass := false
		switch gate.Operator {
		case ">=":
			pass = value >= gate.Threshold
		case ">":
			pass = value > gate.Threshold
		case "<=":
			pass = value <= gate.Threshold
		case "<":
			pass = value < gate.Threshold
		}
		if !pass {
			return errors.New("evaluation gate failed: " + gate.Metric)
		}
	}
	return nil
}

type TenantQuota struct {
	MaxConcurrentRuns int
	MaxGPUs           int
}

func ValidateQuota(activeRuns, activeGPUs int, requested contracts.TopologySpec, quota TenantQuota) error {
	if quota.MaxConcurrentRuns > 0 && activeRuns >= quota.MaxConcurrentRuns {
		return errors.New("tenant concurrent run quota exceeded")
	}
	requestedGPUs := requested.Trainer.Nodes * requested.Trainer.Resources.GPUs
	if requested.Mode == "disaggregated" {
		requestedGPUs += requested.Rollout.Replicas * requested.Rollout.Resources.GPUs
	}
	if quota.MaxGPUs > 0 && activeGPUs+requestedGPUs > quota.MaxGPUs {
		return errors.New("tenant GPU quota exceeded")
	}
	return nil
}

// FairQueue is a deficit-round-robin queue used for bounded-staleness work.
type FairQueue struct {
	mu       sync.Mutex
	byTenant map[string][]string
	order    []string
	cursor   int
}

func NewFairQueue() *FairQueue {
	return &FairQueue{byTenant: map[string][]string{}}
}

func (q *FairQueue) Push(tenantID, item string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.byTenant[tenantID]; !exists {
		q.order = append(q.order, tenantID)
		sort.Strings(q.order)
	}
	q.byTenant[tenantID] = append(q.byTenant[tenantID], item)
}

func (q *FairQueue) Pop() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.order) == 0 {
		return "", false
	}
	for checked := 0; checked < len(q.order); checked++ {
		q.cursor %= len(q.order)
		tenant := q.order[q.cursor]
		q.cursor++
		items := q.byTenant[tenant]
		if len(items) == 0 {
			continue
		}
		item := items[0]
		q.byTenant[tenant] = items[1:]
		return item, true
	}
	return "", false
}

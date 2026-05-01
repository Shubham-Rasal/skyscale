package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// TrainingMetric is a single training step metric from a GPU job.
type TrainingMetric struct {
	JobID         string  `json:"job_id"`
	Step          int     `json:"step"`
	EpisodeReward float64 `json:"episode_reward"`
	Loss          float64 `json:"loss"`
	GPUUtil       int     `json:"gpu_util"`
	Timestamp     int64   `json:"timestamp"`
}

const metricsRingSize = 200

// TrainingMetricsStore holds per-job metric ring buffers in memory.
type TrainingMetricsStore struct {
	mu   sync.RWMutex
	data map[string][]TrainingMetric // job_id → ring slice
}

func NewTrainingMetricsStore() *TrainingMetricsStore {
	return &TrainingMetricsStore{data: make(map[string][]TrainingMetric)}
}

func (s *TrainingMetricsStore) Push(m TrainingMetric) {
	if m.Timestamp == 0 {
		m.Timestamp = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := s.data[m.JobID]
	ring = append(ring, m)
	if len(ring) > metricsRingSize {
		ring = ring[len(ring)-metricsRingSize:]
	}
	s.data[m.JobID] = ring
}

// Snapshot returns a copy of all metrics (job_id → slice).
func (s *TrainingMetricsStore) Snapshot() map[string][]TrainingMetric {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]TrainingMetric, len(s.data))
	for k, v := range s.data {
		cp := make([]TrainingMetric, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// TrainingMetricsHandler is the exported HTTP handler for POST /api/training/metrics.
func (h *APIHandler) TrainingMetricsHandler(w http.ResponseWriter, r *http.Request) {
	h.trainingMetricsHandler(w, r)
}

// trainingMetricsHandler accepts metric POSTs from training containers.
func (h *APIHandler) trainingMetricsHandler(w http.ResponseWriter, r *http.Request) {
	var m TrainingMetric
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if m.JobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}
	h.trainingMetrics.Push(m)
	w.WriteHeader(http.StatusAccepted)
}

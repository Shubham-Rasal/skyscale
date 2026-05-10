package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bluequbit/faas/control-plane/state"
	"github.com/gorilla/mux"
)

// type aliases to avoid import cycle verbosity
type stateExecution = state.Execution
type stateVM = state.VM

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
	// Write-through to SQLite so metrics survive restarts.
	rec := &state.TrainingMetricRecord{
		JobID:         m.JobID,
		Step:          m.Step,
		EpisodeReward: m.EpisodeReward,
		Loss:          m.Loss,
		GPUUtil:       m.GPUUtil,
		Timestamp:     m.Timestamp,
	}
	if err := h.stateManager.SaveMetric(rec); err != nil {
		h.logger.Warnf("trainingMetrics: db write: %v", err)
	}
	w.WriteHeader(http.StatusAccepted)
}

// trainingMetricsGetHandler returns all metrics for a specific job.
// Serves from the in-memory ring first; falls back to DB if ring is empty (e.g. after restart).
func (h *APIHandler) trainingMetricsGetHandler(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["job_id"]
	snapshot := h.trainingMetrics.Snapshot()
	metrics, ok := snapshot[jobID]
	if !ok || len(metrics) == 0 {
		// Fallback: load from DB.
		rows, err := h.stateManager.GetMetrics(jobID)
		if err != nil {
			h.logger.Warnf("trainingMetrics: db read: %v", err)
		}
		metrics = make([]TrainingMetric, 0, len(rows))
		for _, r := range rows {
			metrics = append(metrics, TrainingMetric{
				JobID:         r.JobID,
				Step:          r.Step,
				EpisodeReward: r.EpisodeReward,
				Loss:          r.Loss,
				GPUUtil:       r.GPUUtil,
				Timestamp:     r.Timestamp,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// submitTrainingJobHandler accepts a job submission from the dashboard, enqueues it,
// and returns the execution ID immediately. Workers dispatch via the provider registry.
func (h *APIHandler) submitTrainingJobHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		JobID           string            `json:"job_id"`
		DockerImage     string            `json:"docker_image"`
		GPUModel        string            `json:"gpu_model"`
		Provider        string            `json:"provider"`
		EnvVars         map[string]string `json:"env_vars"`
		ControlPlaneURL string            `json:"control_plane_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.DockerImage == "" {
		http.Error(w, "docker_image required", http.StatusBadRequest)
		return
	}
	if body.JobID == "" {
		body.JobID = "job-" + fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	if body.GPUModel == "" {
		body.GPUModel = "a100"
	}
	if body.ControlPlaneURL == "" {
		body.ControlPlaneURL = defaultControlPlaneURL(r)
	}

	executionID := "exec-" + body.JobID

	// Seed execution record immediately so dashboard shows "queued".
	exec := &stateExecution{
		ID:           executionID,
		FunctionID:   body.JobID,
		Status:       "queued",
		StartTime:    time.Now(),
		JobType:      "training_run",
		HardwareType: "gpu",
		GPUModel:     body.GPUModel,
	}
	if err := h.stateManager.SaveExecution(exec); err != nil {
		h.logger.Errorf("submitTrainingJob: seed execution: %v", err)
	}

	// Encode env vars to JSON for the queue item.
	envJSON, _ := json.Marshal(body.EnvVars)

	item := &state.JobQueueItem{
		ID:              body.JobID,
		ExecutionID:     executionID,
		DockerImage:     body.DockerImage,
		GPUModel:        body.GPUModel,
		ProviderName:    body.Provider,
		EnvVarsJSON:     string(envJSON),
		ControlPlaneURL: body.ControlPlaneURL,
		Priority:        0,
		Status:          "queued",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := h.stateManager.EnqueueJob(item); err != nil {
		h.logger.Errorf("submitTrainingJob: enqueue: %v", err)
		http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
		return
	}
	h.logger.Infof("submitTrainingJob: enqueued job=%s exec=%s", body.JobID, executionID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"execution_id": executionID,
		"job_id":       body.JobID,
		"status":       "queued",
	})
}

func defaultControlPlaneURL(r *http.Request) string {
	if base := os.Getenv("SKYSCALE_PUBLIC_BASE"); base != "" {
		return base
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func (h *APIHandler) markExecutionFailed(executionID, errMsg string) {
	exec, err := h.stateManager.GetExecution(executionID)
	if err != nil {
		return
	}
	exec.Status = "error"
	exec.Error = errMsg
	exec.EndTime = time.Now()
	h.stateManager.SaveExecution(exec)
}

// seedExecutionHandler creates an execution record in state so the dashboard can show it.
// Called by the akash-test tool (and eventually the scheduler) before dispatching to Akash.
func (h *APIHandler) seedExecutionHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID           string `json:"id"`
		JobID        string `json:"job_id"`
		Status       string `json:"status"`
		JobType      string `json:"job_type"`
		HardwareType string `json:"hardware_type"`
		GPUModel     string `json:"gpu_model"`
		VMID         string `json:"vm_id"` // Akash dseq — drives the "dseq present" check
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		body.ID = body.JobID
	}
	if body.Status == "" {
		body.Status = "running"
	}

	exec := &stateExecution{
		ID:           body.ID,
		FunctionID:   body.JobID,
		Status:       body.Status,
		StartTime:    time.Now(),
		JobType:      body.JobType,
		HardwareType: body.HardwareType,
		GPUModel:     body.GPUModel,
		VMID:         body.VMID,
	}
	if err := h.stateManager.SaveExecution(exec); err != nil {
		h.logger.Errorf("seedExecution: %v", err)
		http.Error(w, "failed to save execution", http.StatusInternalServerError)
		return
	}
	h.logger.Infof("Seeded training execution: %s", body.ID)
	w.WriteHeader(http.StatusCreated)
}

// seedVMHandler registers an Akash deployment as a GPU VM in state.
func (h *APIHandler) seedVMHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID                string `json:"id"`
		Status            string `json:"status"`
		HardwareType      string `json:"hardware_type"`
		GPUModel          string `json:"gpu_model"`
		Provider          string `json:"provider"`
		AkashDeploymentID string `json:"akash_deployment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Status == "" {
		body.Status = "busy"
	}

	vm := &stateVM{
		ID:                body.ID,
		Status:            body.Status,
		HardwareType:      body.HardwareType,
		GPUModel:          body.GPUModel,
		ProviderAddr:      body.Provider,
		AkashDeploymentID: body.AkashDeploymentID,
		CreatedAt:         time.Now(),
	}
	if err := h.stateManager.SaveVM(vm); err != nil {
		h.logger.Errorf("seedVM: %v", err)
		http.Error(w, "failed to save vm", http.StatusInternalServerError)
		return
	}
	h.logger.Infof("Seeded GPU VM record: %s", body.ID)
	w.WriteHeader(http.StatusCreated)
}

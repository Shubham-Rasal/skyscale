package api

import (
	"encoding/json"
	"net/http"

	"github.com/bluequbit/faas/control-plane/state"
)

// rlBufferPushHandler: POST /api/rl/buffer/push
// Called by rollout workers to add a trajectory to the experience buffer.
func (h *APIHandler) rlBufferPushHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID     string  `json:"run_id"`
		ProblemID string  `json:"problem_id"`
		Prompt    string  `json:"prompt"`
		Code      string  `json:"code"`
		Reward    float64 `json:"reward"`
		Done      bool    `json:"done"`
		StepN     int     `json:"step_n"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.RunID == "" {
		http.Error(w, "run_id required", http.StatusBadRequest)
		return
	}

	t := &state.Trajectory{
		RunID:     body.RunID,
		ProblemID: body.ProblemID,
		Prompt:    body.Prompt,
		Code:      body.Code,
		Reward:    body.Reward,
		Done:      body.Done,
		StepN:     body.StepN,
	}
	if err := h.stateManager.SaveTrajectory(t); err != nil {
		h.logger.Errorf("rlBufferPush: %v", err)
		http.Error(w, "failed to save trajectory", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// rlBufferSampleHandler: POST /api/rl/buffer/sample
// Called by the trainer to get a batch of unconsumed trajectories.
func (h *APIHandler) rlBufferSampleHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID     string `json:"run_id"`
		BatchSize int    `json:"batch_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.RunID == "" {
		http.Error(w, "run_id required", http.StatusBadRequest)
		return
	}
	if body.BatchSize <= 0 {
		body.BatchSize = 32
	}

	rows, err := h.stateManager.SampleTrajectories(body.RunID, body.BatchSize)
	if err != nil {
		h.logger.Errorf("rlBufferSample: %v", err)
		http.Error(w, "failed to sample trajectories", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"trajectories": rows,
		"count":        len(rows),
	})
}

// rlBufferStatsHandler: GET /api/rl/buffer/stats?run_id=<id>
// Returns buffer size (unconsumed trajectories) for a run.
func (h *APIHandler) rlBufferStatsHandler(w http.ResponseWriter, r *http.Request) {
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		http.Error(w, "run_id query param required", http.StatusBadRequest)
		return
	}
	count, err := h.stateManager.BufferSize(runID)
	if err != nil {
		http.Error(w, "failed to count buffer", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"run_id": runID, "size": count})
}

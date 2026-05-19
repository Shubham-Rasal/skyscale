package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bluequbit/faas/control-plane/state"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// rlStartRunHandler: POST /api/rl/runs
// Starts a distributed RL training run: spawns policy server, trainer, and N rollout workers.
func (h *APIHandler) rlStartRunHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseModel      string `json:"base_model"`
		NumWorkers     int    `json:"num_workers"`
		GPUModel       string `json:"gpu_model"`
		ProblemSet     string `json:"problem_set"` // "default" for now
		ControlPlaneURL string `json:"control_plane_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.BaseModel == "" {
		body.BaseModel = "Qwen/Qwen3-0.6B"
	}
	if body.NumWorkers <= 0 {
		body.NumWorkers = 2
	}
	if body.GPUModel == "" {
		body.GPUModel = "a100"
	}
	if body.ControlPlaneURL == "" {
		body.ControlPlaneURL = defaultControlPlaneURL(r)
	}

	runID := "rl-" + uuid.New().String()[:8]
	now := time.Now()

	run := &state.RLRun{
		ID:         runID,
		Status:     "starting",
		BaseModel:  body.BaseModel,
		NumWorkers: body.NumWorkers,
		GPUModel:   body.GPUModel,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.stateManager.SaveRLRun(run); err != nil {
		http.Error(w, "failed to save run", http.StatusInternalServerError)
		return
	}

	// Spawn policy server job (GPU)
	policyExecID := "exec-policy-" + runID
	policyEnv := map[string]string{
		"MODEL_NAME":        body.BaseModel,
		"RUN_ID":            runID,
		"CONTROL_PLANE_URL": body.ControlPlaneURL,
		"EXECUTION_ID":      policyExecID,
	}
	policyEnvJSON, _ := json.Marshal(policyEnv)
	policyJob := &state.JobQueueItem{
		ID:              "job-policy-" + runID,
		ExecutionID:     policyExecID,
		DockerImage:     "ghcr.io/skyscale/policy-server:latest",
		GPUModel:        body.GPUModel,
		EnvVarsJSON:     string(policyEnvJSON),
		ControlPlaneURL: body.ControlPlaneURL,
		Priority:        10, // high priority — workers block on it
		Status:          "queued",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	policyExec := &state.Execution{
		ID:           policyExecID,
		FunctionID:   runID,
		Status:       "queued",
		StartTime:    now,
		JobType:      "policy_server",
		HardwareType: "gpu",
		GPUModel:     body.GPUModel,
	}
	h.stateManager.SaveExecution(policyExec)
	if err := h.stateManager.EnqueueJob(policyJob); err != nil {
		h.logger.Errorf("rlStartRun: enqueue policy server: %v", err)
	}

	// Spawn trainer job (GPU)
	trainerExecID := "exec-trainer-" + runID
	trainerEnv := map[string]string{
		"RUN_ID":            runID,
		"BASE_MODEL":        body.BaseModel,
		"CONTROL_PLANE_URL": body.ControlPlaneURL,
		"EXECUTION_ID":      trainerExecID,
		"POLICY_EXEC_ID":    policyExecID,
	}
	trainerEnvJSON, _ := json.Marshal(trainerEnv)
	trainerJob := &state.JobQueueItem{
		ID:              "job-trainer-" + runID,
		ExecutionID:     trainerExecID,
		DockerImage:     "ghcr.io/skyscale/rl-trainer:latest",
		GPUModel:        body.GPUModel,
		EnvVarsJSON:     string(trainerEnvJSON),
		ControlPlaneURL: body.ControlPlaneURL,
		Priority:        9,
		Status:          "queued",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	trainerExec := &state.Execution{
		ID:           trainerExecID,
		FunctionID:   runID,
		Status:       "queued",
		StartTime:    now,
		JobType:      "rl_trainer",
		HardwareType: "gpu",
		GPUModel:     body.GPUModel,
	}
	h.stateManager.SaveExecution(trainerExec)
	if err := h.stateManager.EnqueueJob(trainerJob); err != nil {
		h.logger.Errorf("rlStartRun: enqueue trainer: %v", err)
	}

	// Spawn N rollout worker jobs (CPU)
	workerExecIDs := make([]string, body.NumWorkers)
	for i := 0; i < body.NumWorkers; i++ {
		workerExecID := fmt.Sprintf("exec-worker-%s-%d", runID, i)
		workerExecIDs[i] = workerExecID
		workerEnv := map[string]string{
			"RUN_ID":            runID,
			"WORKER_INDEX":      fmt.Sprintf("%d", i),
			"CONTROL_PLANE_URL": body.ControlPlaneURL,
			"EXECUTION_ID":      workerExecID,
			"POLICY_EXEC_ID":    policyExecID,
		}
		workerEnvJSON, _ := json.Marshal(workerEnv)
		workerJob := &state.JobQueueItem{
			ID:              fmt.Sprintf("job-worker-%s-%d", runID, i),
			ExecutionID:     workerExecID,
			DockerImage:     "ghcr.io/skyscale/rl-worker:latest",
			GPUModel:        "",
			EnvVarsJSON:     string(workerEnvJSON),
			ControlPlaneURL: body.ControlPlaneURL,
			Priority:        5,
			Status:          "queued",
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		workerExec := &state.Execution{
			ID:           workerExecID,
			FunctionID:   runID,
			Status:       "queued",
			StartTime:    now,
			JobType:      "rl_rollout_worker",
			HardwareType: "cpu",
		}
		h.stateManager.SaveExecution(workerExec)
		if err := h.stateManager.EnqueueJob(workerJob); err != nil {
			h.logger.Errorf("rlStartRun: enqueue worker %d: %v", i, err)
		}
	}

	// Save worker exec IDs and trainer exec ID to run record
	workerIDsJSON, _ := json.Marshal(workerExecIDs)
	run.TrainerExecID = trainerExecID
	run.WorkerExecIDs = string(workerIDsJSON)
	run.Status = "running"
	run.UpdatedAt = time.Now()
	h.stateManager.SaveRLRun(run)

	h.logger.Infof("Started RL run %s: %d workers, trainer=%s, policy=%s",
		runID, body.NumWorkers, trainerExecID, policyExecID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"run_id":           runID,
		"status":           "running",
		"trainer_exec_id":  trainerExecID,
		"policy_exec_id":   policyExecID,
		"worker_exec_ids":  workerExecIDs,
	})
}

// rlListRunsHandler: GET /api/rl/runs
func (h *APIHandler) rlListRunsHandler(w http.ResponseWriter, r *http.Request) {
	runs, err := h.stateManager.ListRLRuns()
	if err != nil {
		http.Error(w, "failed to list runs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

// rlGetRunHandler: GET /api/rl/runs/{id}
func (h *APIHandler) rlGetRunHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	run, err := h.stateManager.GetRLRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	// Enrich with trainer + worker execution statuses
	var workerIDs []string
	json.Unmarshal([]byte(run.WorkerExecIDs), &workerIDs)

	type execStatus struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	workerStatuses := make([]execStatus, 0, len(workerIDs))
	for _, wid := range workerIDs {
		exec, err := h.stateManager.GetExecution(wid)
		if err == nil {
			workerStatuses = append(workerStatuses, execStatus{ID: wid, Status: exec.Status})
		}
	}

	trainerStatus := ""
	if run.TrainerExecID != "" {
		if exec, err := h.stateManager.GetExecution(run.TrainerExecID); err == nil {
			trainerStatus = exec.Status
		}
	}

	bufSize, _ := h.stateManager.BufferSize(id)

	// Fetch latest training metrics for this run
	metrics, _ := h.stateManager.GetMetrics(id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"run":             run,
		"trainer_status":  trainerStatus,
		"worker_statuses": workerStatuses,
		"buffer_size":     bufSize,
		"metrics":         metrics,
	})
}

// rlStopRunHandler: DELETE /api/rl/runs/{id}
func (h *APIHandler) rlStopRunHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	run, err := h.stateManager.GetRLRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	run.Status = "stopped"
	run.UpdatedAt = time.Now()
	h.stateManager.SaveRLRun(run)

	// Mark all child executions as stopped
	var workerIDs []string
	json.Unmarshal([]byte(run.WorkerExecIDs), &workerIDs)
	for _, wid := range append(workerIDs, run.TrainerExecID) {
		if exec, err := h.stateManager.GetExecution(wid); err == nil {
			if exec.Status == "queued" || exec.Status == "running" {
				exec.Status = "stopped"
				exec.EndTime = time.Now()
				h.stateManager.SaveExecution(exec)
			}
		}
	}

	h.logger.Infof("Stopped RL run %s", id)
	w.WriteHeader(http.StatusNoContent)
}

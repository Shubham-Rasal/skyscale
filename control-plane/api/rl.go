package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bluequbit/faas/control-plane/observability"
	"github.com/bluequbit/faas/control-plane/rlevents"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func rlSmokeDefaults() (maxSteps, batchSize, minBatch, checkpointEvery, workerMaxSteps string) {
	return firstEnv("RL_MAX_STEPS", "3"),
		firstEnv("RL_BATCH_SIZE", "4"),
		firstEnv("RL_MIN_BATCH_SIZE", "4"),
		firstEnv("RL_CHECKPOINT_EVERY", "3"),
		firstEnv("RL_WORKER_MAX_STEPS", "10")
}

func firstEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustAtoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return 1
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func appendHFToken(env map[string]string) {
	if token := os.Getenv("HF_TOKEN"); token != "" {
		env["HF_TOKEN"] = token
	}
}

// rlStartRunHandler: POST /api/rl/runs
// Starts a distributed RL training run: spawns policy server, trainer, and N rollout workers.
func (h *APIHandler) rlStartRunHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseModel       string `json:"base_model"`
		NumWorkers      int    `json:"num_workers"`
		GPUModel        string `json:"gpu_model"`
		ProblemSet      string `json:"problem_set"` // "default" for now
		ControlPlaneURL string `json:"control_plane_url"`
		MaxSteps        int    `json:"max_steps"`
		MinBatchSize    int    `json:"min_batch_size"`
		BatchSize       int    `json:"batch_size"`
		WorkerMaxSteps  int    `json:"worker_max_steps"`
		ClosedLoop      bool   `json:"closed_loop"`
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
		body.GPUModel = "a10g"
	}
	if body.ControlPlaneURL == "" {
		body.ControlPlaneURL = defaultControlPlaneURL(r)
	}

	maxSteps, batchSize, minBatch, checkpointEvery, workerMaxSteps := rlSmokeDefaults()
	if body.MaxSteps > 0 {
		maxSteps = fmt.Sprintf("%d", body.MaxSteps)
	}
	if body.BatchSize > 0 {
		batchSize = fmt.Sprintf("%d", body.BatchSize)
	}
	if body.MinBatchSize > 0 {
		minBatch = fmt.Sprintf("%d", body.MinBatchSize)
	}
	if body.WorkerMaxSteps > 0 {
		workerMaxSteps = fmt.Sprintf("%d", body.WorkerMaxSteps)
	}
	if os.Getenv("RL_CLOSED_LOOP") == "1" {
		body.ClosedLoop = true
	}

	runID := "rl-" + uuid.New().String()[:8]
	now := time.Now()

	run := &state.RLRun{
		ID:         runID,
		Status:     "starting",
		BaseModel:  body.BaseModel,
		NumWorkers: body.NumWorkers,
		GPUModel:   body.GPUModel,
		ClosedLoop: body.ClosedLoop,
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
		"SKYSCALE_ROLE":     "policy_server",
		"ENTRYPOINT":        "python /app/serve.py",
	}
	appendHFToken(policyEnv)
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
		"MAX_STEPS":         maxSteps,
		"BATCH_SIZE":        batchSize,
		"MIN_BATCH_SIZE":    minBatch,
		"CHECKPOINT_EVERY":  checkpointEvery,
		"ENTRYPOINT":        "python /app/trainer.py",
	}
	if body.ClosedLoop {
		trainerEnv["CLOSED_LOOP"] = "1"
		trainerEnv["CHECKPOINT_EVERY"] = "1"
		trainerEnv["NUM_WORKERS"] = fmt.Sprintf("%d", body.NumWorkers)
		trainerEnv["TRAJECTORIES_PER_WORKER"] = fmt.Sprintf("%d", maxInt(1, (mustAtoi(minBatch)+body.NumWorkers-1)/maxInt(1, body.NumWorkers)))
	}
	appendHFToken(trainerEnv)
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

	// Spawn N rollout worker jobs (CPU) — skipped in closed-loop mode (trainer triggers collect rounds)
	workerExecIDs := make([]string, 0, body.NumWorkers)
	if !body.ClosedLoop {
		workerExecIDs = make([]string, body.NumWorkers)
		for i := 0; i < body.NumWorkers; i++ {
			workerExecID := fmt.Sprintf("exec-worker-%s-%d", runID, i)
			workerExecIDs[i] = workerExecID
			workerEnv := map[string]string{
				"RUN_ID":            runID,
				"WORKER_INDEX":      fmt.Sprintf("%d", i),
				"CONTROL_PLANE_URL": body.ControlPlaneURL,
				"EXECUTION_ID":      workerExecID,
				"POLICY_EXEC_ID":    policyExecID,
				"MODEL":             body.BaseModel,
				"MAX_STEPS":         workerMaxSteps,
				"DIFFICULTY":        "easy",
				"ENTRYPOINT":        "python /app/worker.py",
			}
			workerEnvJSON, _ := json.Marshal(workerEnv)
			workerJob := &state.JobQueueItem{
				ID:              fmt.Sprintf("job-worker-%s-%d", runID, i),
				ExecutionID:     workerExecID,
				DockerImage:     "ghcr.io/skyscale/rl-worker:latest",
				GPUModel:        "cpu",
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
	recordRLEvent(runID, "coordinator", "info",
		fmt.Sprintf("run started model=%s workers=%d gpu=%s closed_loop=%v", body.BaseModel, body.NumWorkers, body.GPUModel, body.ClosedLoop))
	if body.ClosedLoop {
		recordRLEvent(runID, "coordinator", "info", "closed-loop mode: trainer orchestrates collect → train → policy redeploy")
	} else {
		recordRLEvent(runID, "coordinator", "info",
			"dispatch order: policy server first (blocks ~3-5min on Modal), then workers+trainer")
	}

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
	metricRows, _ := h.stateManager.GetMetrics(id)
	metrics := make([]map[string]any, 0, len(metricRows))
	for _, m := range metricRows {
		metrics = append(metrics, map[string]any{
			"step":           m.Step,
			"episode_reward": m.EpisodeReward,
			"loss":           m.Loss,
			"gpu_util":       m.GPUUtil,
			"timestamp":      m.Timestamp,
		})
	}

	policyStatus := "waiting"
	if run.PolicyServerURL != "" {
		policyStatus = "ready"
	}
	stage := "starting"
	switch {
	case run.PolicyServerURL == "":
		stage = "waiting_for_policy"
	case bufSize == 0:
		stage = "waiting_for_rollouts"
	case trainerStatus == "running" && len(metrics) == 0:
		stage = "training"
	default:
		stage = "running"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"run":             run,
		"trainer_status":  trainerStatus,
		"worker_statuses": workerStatuses,
		"buffer_size":     bufSize,
		"metrics":         metrics,
		"stage":           stage,
		"policy_status":   policyStatus,
		"event_count":     len(rlevents.Default.List(id, 0)),
		"grafana_url":     observability.GrafanaRunURL(id),
	})
}

// rlSetPolicyServerHandler: POST /api/rl/runs/{id}/policy-server
func (h *APIHandler) rlSetPolicyServerHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}
	run, err := h.stateManager.GetRLRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	run.PolicyServerURL = body.URL
	run.UpdatedAt = time.Now()
	if err := h.stateManager.SaveRLRun(run); err != nil {
		http.Error(w, "failed to save run", http.StatusInternalServerError)
		return
	}
	h.logger.Infof("RL run %s policy server URL set: %s", id, body.URL)
	recordRLEvent(id, "policy", "info", "policy server URL registered: "+body.URL)
	w.WriteHeader(http.StatusOK)
}

// rlPolicyReloadHandler: POST /api/rl/runs/{id}/policy-reload
// Trainer uploads a checkpoint, then calls this to hot-reload policy server weights.
func (h *APIHandler) rlPolicyReloadHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		CheckpointURL string `json:"checkpoint_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CheckpointURL == "" {
		http.Error(w, "checkpoint_url required", http.StatusBadRequest)
		return
	}
	run, err := h.stateManager.GetRLRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if run.PolicyServerURL == "" {
		http.Error(w, "policy server URL not set", http.StatusServiceUnavailable)
		return
	}

	reloadURL := strings.TrimRight(run.PolicyServerURL, "/") + "/reload"
	payload, _ := json.Marshal(map[string]string{"checkpoint_url": body.CheckpointURL})
	resp, err := http.Post(reloadURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		h.logger.Errorf("policy reload POST failed for %s: %v", id, err)
		recordRLEvent(id, "policy", "error", "reload request failed: "+err.Error())
		http.Error(w, "reload request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		h.logger.Errorf("policy reload %s returned %s: %s", id, resp.Status, string(respBody))
		recordRLEvent(id, "policy", "error", fmt.Sprintf("reload HTTP %s", resp.Status))
		http.Error(w, "policy reload failed: "+string(respBody), resp.StatusCode)
		return
	}

	h.logger.Infof("RL run %s policy reloaded from %s", id, body.CheckpointURL)
	recordRLEvent(id, "policy", "info", "weights reloaded from checkpoint")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

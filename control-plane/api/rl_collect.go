package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bluequbit/faas/control-plane/providers"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/gorilla/mux"
)

// rlCollectHandler: POST /api/rl/runs/{id}/collect
// Enqueues one round of rollout workers (closed-loop mode).
func (h *APIHandler) rlCollectHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	run, err := h.stateManager.GetRLRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if run.PolicyServerURL == "" {
		http.Error(w, "policy server not ready", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		Round                 int `json:"round"`
		TrajectoriesPerWorker int `json:"trajectories_per_worker"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Round <= 0 {
		body.Round = run.TrainingRound + 1
	}
	if body.TrajectoriesPerWorker <= 0 {
		body.TrajectoriesPerWorker = 2
	}

	controlPlaneURL := defaultControlPlaneURL(r)
	execIDs, err := h.enqueueWorkerRound(run, body.Round, body.TrajectoriesPerWorker, controlPlaneURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	run.TrainingRound = body.Round
	run.UpdatedAt = time.Now()
	_ = h.stateManager.SaveRLRun(run)

	recordRLEvent(id, "coordinator", "info",
		fmt.Sprintf("collect round=%d workers=%d traj_per_worker=%d", body.Round, len(execIDs), body.TrajectoriesPerWorker))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"round":            body.Round,
		"worker_exec_ids":  execIDs,
		"trajectories_per_worker": body.TrajectoriesPerWorker,
	})
}

func (h *APIHandler) enqueueWorkerRound(run *state.RLRun, round, trajectoriesPerWorker int, controlPlaneURL string) ([]string, error) {
	now := time.Now()
	execIDs := make([]string, run.NumWorkers)
	policyExecID := "exec-policy-" + run.ID

	for i := 0; i < run.NumWorkers; i++ {
		workerExecID := fmt.Sprintf("exec-worker-%s-r%d-%d", run.ID, round, i)
		execIDs[i] = workerExecID
		workerEnv := map[string]string{
			"RUN_ID":                  run.ID,
			"WORKER_INDEX":            fmt.Sprintf("%d", i),
			"COLLECT_ROUND":           fmt.Sprintf("%d", round),
			"CONTROL_PLANE_URL":       controlPlaneURL,
			"EXECUTION_ID":            workerExecID,
			"POLICY_EXEC_ID":          policyExecID,
			"MODEL":                   run.BaseModel,
			"MAX_STEPS":               fmt.Sprintf("%d", trajectoriesPerWorker),
			"DIFFICULTY":              "easy",
			"ENTRYPOINT":              "python /app/worker.py",
			"REFRESH_POLICY_URL":      "1",
		}
		workerEnvJSON, _ := json.Marshal(workerEnv)
		workerJob := &state.JobQueueItem{
			ID:              fmt.Sprintf("job-worker-%s-r%d-%d", run.ID, round, i),
			ExecutionID:     workerExecID,
			DockerImage:     "ghcr.io/skyscale/rl-worker:latest",
			GPUModel:        "cpu",
			EnvVarsJSON:     string(workerEnvJSON),
			ControlPlaneURL: controlPlaneURL,
			Priority:        8,
			Status:          "queued",
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		workerExec := &state.Execution{
			ID:           workerExecID,
			FunctionID:   run.ID,
			Status:       "queued",
			StartTime:    now,
			JobType:      "rl_rollout_worker",
			HardwareType: "cpu",
		}
		if err := h.stateManager.SaveExecution(workerExec); err != nil {
			return nil, err
		}
		if err := h.stateManager.EnqueueJob(workerJob); err != nil {
			return nil, err
		}
	}
	return execIDs, nil
}

// rlPolicyRedeployHandler: POST /api/rl/runs/{id}/policy-redeploy
// Terminates the current policy sandbox and launches a new one from a checkpoint.
func (h *APIHandler) rlPolicyRedeployHandler(w http.ResponseWriter, r *http.Request) {
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

	policyExecID := "exec-policy-" + id
	if exec, err := h.stateManager.GetExecution(policyExecID); err == nil && exec.VMID != "" {
		modal := providers.NewModalProvider(h.logger)
		_ = modal.Terminate(context.Background(), exec.VMID)
		recordRLEvent(id, "policy", "info", "terminated previous policy sandbox")
	}

	run.PolicyServerURL = ""
	run.UpdatedAt = time.Now()
	_ = h.stateManager.SaveRLRun(run)

	controlPlaneURL := defaultControlPlaneURL(r)
	now := time.Now()
	policyEnv := map[string]string{
		"MODEL_NAME":        run.BaseModel,
		"CHECKPOINT_URL":    body.CheckpointURL,
		"RUN_ID":            id,
		"CONTROL_PLANE_URL": controlPlaneURL,
		"EXECUTION_ID":      policyExecID,
		"SKYSCALE_ROLE":     "policy_server",
		"ENTRYPOINT":        "python /app/serve.py",
	}
	appendHFToken(policyEnv)
	policyEnvJSON, _ := json.Marshal(policyEnv)
	policyJob := &state.JobQueueItem{
		ID:              "job-policy-" + id + "-r" + fmt.Sprintf("%d", run.TrainingRound),
		ExecutionID:     policyExecID,
		DockerImage:     "ghcr.io/skyscale/policy-server:latest",
		GPUModel:        run.GPUModel,
		EnvVarsJSON:     string(policyEnvJSON),
		ControlPlaneURL: controlPlaneURL,
		Priority:        10,
		Status:          "queued",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	policyExec := &state.Execution{
		ID:           policyExecID,
		FunctionID:   id,
		Status:       "queued",
		StartTime:    now,
		JobType:      "policy_server",
		HardwareType: "gpu",
		GPUModel:     run.GPUModel,
	}
	_ = h.stateManager.SaveExecution(policyExec)
	if err := h.stateManager.EnqueueJob(policyJob); err != nil {
		http.Error(w, "failed to enqueue policy redeploy", http.StatusInternalServerError)
		return
	}

	recordRLEvent(id, "policy", "info", "policy redeploy queued from "+body.CheckpointURL)
	h.logger.Infof("RL run %s policy redeploy queued checkpoint=%s", id, body.CheckpointURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "queued"})
}

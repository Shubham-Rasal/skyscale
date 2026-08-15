package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
	"github.com/bluequbit/faas/control-plane/rlservice"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func (h *APIHandler) rlStartSlimeRunHandler(w http.ResponseWriter, r *http.Request, raw []byte) {
	if h.rlReconciler == nil {
		http.Error(w, "slime backend requires SKYSCALE_RL_KUBERNETES=1 and Kubernetes credentials", http.StatusServiceUnavailable)
		return
	}
	var wrapper struct {
		Spec *contracts.RLRunSpec `json:"spec"`
	}
	var spec contracts.RLRunSpec
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		http.Error(w, "invalid run contract", http.StatusBadRequest)
		return
	}
	if wrapper.Spec != nil {
		spec = *wrapper.Spec
	} else if err := json.Unmarshal(raw, &spec); err != nil {
		http.Error(w, "invalid run contract", http.StatusBadRequest)
		return
	}
	spec.Normalize()
	spec.Backend = "slime"
	spec.Metadata.RunID = "rl-" + uuid.New().String()[:12]
	spec.CreatedAt = time.Now().UTC()
	snapshot, err := contracts.Snapshot(spec)
	if err != nil {
		http.Error(w, "invalid run contract: "+err.Error(), http.StatusBadRequest)
		return
	}
	activeRuns, err := h.stateManager.CountActiveTenantRLRuns(spec.Metadata.TenantID)
	if err != nil {
		http.Error(w, "failed to evaluate quota", http.StatusInternalServerError)
		return
	}
	quota := rlservice.TenantQuota{
		MaxConcurrentRuns: envInt("SKYSCALE_RL_TENANT_MAX_RUNS", 2),
		MaxGPUs:           envInt("SKYSCALE_RL_TENANT_MAX_GPUS", 8),
	}
	if err := rlservice.ValidateQuota(int(activeRuns), 0, spec.Topology, quota); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	snapshotJSON, _ := json.Marshal(snapshot)
	now := time.Now()
	run := &state.RLRun{
		ID: spec.Metadata.RunID, APIVersion: spec.APIVersion, TenantID: spec.Metadata.TenantID,
		ProjectID: spec.Metadata.ProjectID, Backend: "slime", Status: "starting",
		DesiredState: "running", ObservedState: "pending",
		Namespace:    "skyscale-" + spec.Metadata.TenantID + "-" + spec.Metadata.ProjectID,
		SnapshotJSON: string(snapshotJSON), SnapshotSHA256: snapshot.SHA256,
		BaseModel: spec.Model.Source, GPUModel: spec.Topology.Trainer.Resources.GPUType,
		NumWorkers: spec.Topology.Rollout.Replicas, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.stateManager.SaveRLRun(run); err != nil {
		http.Error(w, "failed to persist immutable run", http.StatusInternalServerError)
		return
	}
	actor := r.Header.Get("X-Skyscale-Actor")
	if actor == "" {
		actor = "authenticated-api-client"
	}
	_ = h.stateManager.SaveRLAudit(&state.RLAuditRecord{
		TenantID: run.TenantID, ProjectID: run.ProjectID, RunID: run.ID, Actor: actor,
		Action: "create", Resource: "rlrun", Outcome: "accepted", RemoteIP: r.RemoteAddr, CreatedAt: time.Now(),
	})
	recordRLEvent(run.ID, "controller", "info", fmt.Sprintf("accepted slime run snapshot=%s namespace=%s", snapshot.SHA256, run.Namespace))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api_version": spec.APIVersion, "run_id": run.ID, "status": run.Status,
		"backend": run.Backend, "snapshot_sha256": snapshot.SHA256, "namespace": run.Namespace,
	})
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func (h *APIHandler) rlSuspendRunHandler(w http.ResponseWriter, r *http.Request) {
	h.setSlimeDesiredState(w, mux.Vars(r)["id"], "suspended")
}

func (h *APIHandler) rlResumeRunHandler(w http.ResponseWriter, r *http.Request) {
	h.setSlimeDesiredState(w, mux.Vars(r)["id"], "running")
}

func (h *APIHandler) setSlimeDesiredState(w http.ResponseWriter, id, desired string) {
	run, err := h.stateManager.GetRLRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if run.Backend != "slime" {
		http.Error(w, "operation is only supported for slime runs", http.StatusConflict)
		return
	}
	if run.Status == "completed" || run.Status == "cancelled" || run.Status == "failed" {
		http.Error(w, "terminal run cannot change desired state", http.StatusConflict)
		return
	}
	run.DesiredState = desired
	run.UpdatedAt = time.Now()
	if desired == "running" {
		run.Status = "starting"
	}
	if err := h.stateManager.SaveRLRun(run); err != nil {
		http.Error(w, "failed to update run", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"run_id": id, "desired_state": desired})
}

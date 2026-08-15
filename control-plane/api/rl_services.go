package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
	"github.com/bluequbit/faas/control-plane/dataplane"
	"github.com/bluequbit/faas/control-plane/observability"
	"github.com/bluequbit/faas/control-plane/rlservice"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/bluequbit/faas/control-plane/weights"
	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newGroupedSampleStore(logger *logrus.Logger) *dataplane.Store {
	options := dataplane.Options{
		MaxTokens: 131072, HighWatermarkGroups: envInt("SKYSCALE_RL_SAMPLE_HIGH_WATERMARK", 1000),
		LowWatermarkGroups: envInt("SKYSCALE_RL_SAMPLE_LOW_WATERMARK", 500), Retention: 7 * 24 * time.Hour,
	}
	if os.Getenv("SKYSCALE_RL_LOCAL_DATA_PLANE") == "1" {
		store, err := dataplane.New(dataplane.NewMemoryMetadataStore(), dataplane.NewMemoryBlobStore(), options)
		if err != nil {
			logger.Errorf("initialize local grouped sample store: %v", err)
			return nil
		}
		logger.Warn("grouped sample data plane uses in-memory local-development storage")
		return store
	}
	databaseURL, endpoint, bucket := os.Getenv("DATABASE_URL"), os.Getenv("S3_ENDPOINT"), os.Getenv("S3_BUCKET")
	if databaseURL == "" || endpoint == "" || bucket == "" {
		logger.Info("production grouped sample data plane disabled: DATABASE_URL, S3_ENDPOINT, and S3_BUCKET are required")
		return nil
	}
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		logger.Errorf("connect grouped sample metadata database: %v", err)
		return nil
	}
	metadata, err := dataplane.NewSQLMetadataStore(db)
	if err != nil {
		logger.Errorf("initialize grouped sample metadata: %v", err)
		return nil
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), ""),
		Secure: os.Getenv("S3_USE_SSL") != "false",
	})
	if err != nil {
		logger.Errorf("initialize grouped sample object store: %v", err)
		return nil
	}
	blobs, err := dataplane.NewObjectBlobStore(client, bucket)
	if err != nil {
		logger.Errorf("initialize grouped sample object adapter: %v", err)
		return nil
	}
	store, err := dataplane.New(metadata, blobs, options)
	if err != nil {
		logger.Errorf("initialize production grouped sample store: %v", err)
		return nil
	}
	return store
}

func newWeightService(manager *state.StateManager, logger *logrus.Logger) *weights.Service {
	var artifacts weights.ArtifactStore
	endpoint, bucket := os.Getenv("S3_ENDPOINT"), os.Getenv("S3_BUCKET")
	if os.Getenv("SKYSCALE_RL_KUBERNETES") == "1" && os.Getenv("DATABASE_URL") == "" &&
		os.Getenv("SKYSCALE_RL_LOCAL_WEIGHT_SERVICE") != "1" {
		logger.Error("production weight service disabled: DATABASE_URL is required")
		return nil
	}
	if endpoint != "" && bucket != "" {
		client, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), ""),
			Secure: os.Getenv("S3_USE_SSL") != "false",
		})
		if err == nil {
			artifacts, err = weights.NewMinioArtifactStore(client, bucket)
		}
		if err != nil {
			logger.Errorf("initialize weight artifact store: %v", err)
			return nil
		}
	} else {
		if os.Getenv("SKYSCALE_RL_KUBERNETES") == "1" && os.Getenv("SKYSCALE_RL_LOCAL_WEIGHT_SERVICE") != "1" {
			logger.Error("production weight service disabled: S3_ENDPOINT and S3_BUCKET are required")
			return nil
		}
		logger.Warn("weight artifacts use process-local storage; configure S3 for production")
		artifacts = weights.NewMemoryArtifactStore()
	}
	service, err := weights.NewService(manager, artifacts)
	if err != nil {
		logger.Errorf("initialize durable weight service: %v", err)
		return nil
	}
	return service
}

func (h *APIHandler) withRLRuntimeAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := os.Getenv("SKYSCALE_RUNTIME_TOKEN")
		if expected == "" {
			if os.Getenv("SKYSCALE_RL_KUBERNETES") == "1" {
				http.Error(w, "runtime authentication is not configured", http.StatusServiceUnavailable)
				return
			}
			next(w, r)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (h *APIHandler) rlGroupedSamplePutHandler(w http.ResponseWriter, r *http.Request) {
	if h.groupedSamples == nil {
		http.Error(w, "grouped sample data plane is not configured", http.StatusServiceUnavailable)
		return
	}
	var sample contracts.SampleEnvelope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&sample); err != nil {
		http.Error(w, "invalid grouped sample", http.StatusBadRequest)
		return
	}
	run, err := h.stateManager.GetRLRun(sample.RunID)
	if err != nil || run.TenantID != sample.TenantID || run.ProjectID != sample.ProjectID {
		http.Error(w, "sample lineage does not match its run owner", http.StatusConflict)
		return
	}
	record, inserted, err := h.groupedSamples.Put(r.Context(), sample)
	if errors.Is(err, dataplane.ErrBackpressure) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if inserted {
		w.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sample_id": record.SampleID, "inserted": inserted, "checksum": record.Checksum})
}

func (h *APIHandler) rlGroupedSampleClaimHandler(w http.ResponseWriter, r *http.Request) {
	if h.groupedSamples == nil {
		http.Error(w, "grouped sample data plane is not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		RunID             string             `json:"run_id"`
		ExpectedGroupSize int                `json:"expected_group_size"`
		Owner             string             `json:"owner"`
		LeaseSeconds      int                `json:"lease_seconds"`
		ImportanceRatios  map[string]float64 `json:"importance_ratios"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid claim", http.StatusBadRequest)
		return
	}
	run, err := h.stateManager.GetRLRun(body.RunID)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	granted, err := h.stateManager.AcquireFairClaimTurn(run.TenantID, run.ID, body.Owner)
	if err != nil {
		http.Error(w, "failed to schedule fair sample claim", http.StatusInternalServerError)
		return
	}
	if !granted {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "another tenant has the next fair scheduling turn", http.StatusTooManyRequests)
		return
	}
	lease, samples, err := h.groupedSamples.Claim(r.Context(), run.TenantID, body.RunID, body.ExpectedGroupSize, body.Owner, time.Duration(body.LeaseSeconds)*time.Second)
	if errors.Is(err, dataplane.ErrNoCompleteGroup) {
		http.Error(w, err.Error(), http.StatusNoContent)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var snapshot contracts.RunSnapshot
	if err := json.Unmarshal([]byte(run.SnapshotJSON), &snapshot); err != nil {
		_ = h.groupedSamples.Nack(r.Context(), lease.ID, body.Owner, "run snapshot failed")
		http.Error(w, "invalid run snapshot", http.StatusInternalServerError)
		return
	}
	decisions := make(map[string]rlservice.SampleDecision, len(samples))
	groupAction := "accept"
	for _, sample := range samples {
		behaviorStep := int64(0)
		if version, lookupErr := h.stateManager.GetPolicyVersion(run.ID, sample.PolicyVersion); lookupErr == nil {
			behaviorStep = version.OptimizerStep
		}
		decision := rlservice.EvaluateSample(snapshot.Spec.Algorithm, run.OptimizerStep, behaviorStep, sample.GeneratedAt, time.Now(), body.ImportanceRatios[sample.SampleID])
		decisions[sample.SampleID] = decision
		observability.RLSampleAgeSeconds.WithLabelValues(run.TenantID, run.ID, decision.Action).Observe(decision.Age.Seconds())
		observability.RLPolicyLagSteps.WithLabelValues(run.TenantID, run.ID, decision.Action).Observe(float64(decision.PolicyLag))
		if decision.Action == "discard" {
			groupAction = "discard"
		} else if decision.Action == "quarantine" && groupAction != "discard" {
			groupAction = "quarantine"
		}
	}
	if groupAction == "discard" {
		_ = h.groupedSamples.Ack(r.Context(), lease.ID, body.Owner)
		run.RolloutDraining, run.UpdatedAt = true, time.Now()
		_ = h.stateManager.SaveRLRun(run)
		w.Header().Set("X-Skyscale-Group-Action", "discard")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if groupAction == "quarantine" {
		_ = h.groupedSamples.Nack(r.Context(), lease.ID, body.Owner, "quarantine: bounded staleness exceeded")
		w.Header().Set("X-Skyscale-Group-Action", "quarantine")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lease": lease, "samples": samples, "admission": decisions})
}

func (h *APIHandler) rlGroupedSampleLeaseHandler(w http.ResponseWriter, r *http.Request) {
	if h.groupedSamples == nil {
		http.Error(w, "grouped sample data plane is not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Owner  string `json:"owner"`
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid lease update", http.StatusBadRequest)
		return
	}
	leaseID := mux.Vars(r)["lease_id"]
	var err error
	if body.Action == "ack" {
		err = h.groupedSamples.Ack(r.Context(), leaseID, body.Owner)
	} else if body.Action == "nack" {
		err = h.groupedSamples.Nack(r.Context(), leaseID, body.Owner, body.Reason)
	} else {
		http.Error(w, "action must be ack or nack", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) rlGroupedSampleBackpressureHandler(w http.ResponseWriter, r *http.Request) {
	if h.groupedSamples == nil {
		http.Error(w, "grouped sample data plane is not configured", http.StatusServiceUnavailable)
		return
	}
	run, err := h.stateManager.GetRLRun(r.URL.Query().Get("run_id"))
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	paused, resumeBelow, err := h.groupedSamples.Backpressure(r.Context(), run.TenantID, run.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run.Backpressured != paused {
		run.Backpressured, run.UpdatedAt = paused, time.Now()
		_ = h.stateManager.SaveRLRun(run)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"paused": paused, "resume_below_groups": resumeBelow})
}

func (h *APIHandler) rlRolloutMetricsHandler(w http.ResponseWriter, r *http.Request) {
	run, err := h.stateManager.GetRLRun(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	var body struct {
		QueueDepth       int     `json:"queue_depth"`
		GenerationP95Sec float64 `json:"generation_p95_seconds"`
		TokensPerSecond  float64 `json:"tokens_per_second"`
		TrainerDemand    int     `json:"trainer_demand"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QueueDepth < 0 ||
		body.GenerationP95Sec < 0 || body.TokensPerSecond < 0 || body.TrainerDemand < 0 {
		http.Error(w, "rollout metrics must be non-negative", http.StatusBadRequest)
		return
	}
	run.RolloutQueueDepth = body.QueueDepth
	run.RolloutGenerationP95 = body.GenerationP95Sec
	run.RolloutTokensPerSecond = body.TokensPerSecond
	run.RolloutTrainerDemand = body.TrainerDemand
	run.UpdatedAt = time.Now()
	if err := h.stateManager.SaveRLRun(run); err != nil {
		http.Error(w, "failed to persist rollout metrics", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) rlRegisterEngineHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID          string `json:"run_id"`
		EngineID       string `json:"engine_id"`
		CurrentVersion string `json:"current_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" || body.EngineID == "" {
		http.Error(w, "run_id and engine_id are required", http.StatusBadRequest)
		return
	}
	if _, err := h.stateManager.GetRLRun(body.RunID); err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if h.weightService == nil {
		http.Error(w, "weight service is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := h.weightService.RegisterEngine(r.Context(), body.RunID, body.EngineID, body.CurrentVersion); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	instruction, err := h.weightService.EngineInstruction(r.Context(), body.RunID, body.EngineID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(instruction)
}

func (h *APIHandler) rlPublishWeightsHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Manifest        weights.Manifest `json:"manifest"`
		RequiredEngines []string         `json:"required_engines"`
		TimeoutSeconds  int              `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid publication", http.StatusBadRequest)
		return
	}
	if h.weightService == nil {
		http.Error(w, "weight service is unavailable", http.StatusServiceUnavailable)
		return
	}
	run, err := h.stateManager.GetRLRun(body.Manifest.RunID)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if err := h.weightService.Publish(r.Context(), body.Manifest, body.RequiredEngines, time.Duration(body.TimeoutSeconds)*time.Second); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	uri, checksum, err := h.weightService.Materialize(r.Context(), body.Manifest.RunID, body.Manifest.Version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	run.RolloutDraining, run.UpdatedAt = true, time.Now()
	_ = h.stateManager.SaveRLRun(run)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"materialized_uri": uri, "checksum": checksum})
}

func (h *APIHandler) rlAcknowledgeWeightsHandler(w http.ResponseWriter, r *http.Request) {
	if h.weightService == nil {
		http.Error(w, "weight service is unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		RunID    string `json:"run_id"`
		EngineID string `json:"engine_id"`
		Checksum string `json:"checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" || body.EngineID == "" {
		http.Error(w, "run_id and engine_id are required", http.StatusBadRequest)
		return
	}
	version := mux.Vars(r)["version"]
	active, err := h.weightService.Acknowledge(r.Context(), body.RunID, body.EngineID, version, body.Checksum)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if active {
		if run, getErr := h.stateManager.GetRLRun(body.RunID); getErr == nil {
			var snapshot contracts.RunSnapshot
			_ = json.Unmarshal([]byte(run.SnapshotJSON), &snapshot)
			requiresEvaluation := snapshot.Spec.Evaluation.SuiteURI != "" || len(snapshot.Spec.Evaluation.Gates) > 0
			run.PolicyVersion, run.RolloutDraining, run.UpdatedAt = version, requiresEvaluation, time.Now()
			_ = h.stateManager.SaveRLRun(run)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"version": version, "active": active})
}

func (h *APIHandler) rlWeightArtifactHandler(w http.ResponseWriter, r *http.Request) {
	if h.weightService == nil {
		http.Error(w, "weight service is unavailable", http.StatusServiceUnavailable)
		return
	}
	runID, version := r.URL.Query().Get("run_id"), mux.Vars(r)["version"]
	payload, checksum, err := h.weightService.MaterializedArtifact(r.Context(), runID, version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Skyscale-SHA256", checksum)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	_, _ = w.Write(payload)
}

func (h *APIHandler) rlGarbageCollectWeightsHandler(w http.ResponseWriter, r *http.Request) {
	if h.weightService == nil {
		http.Error(w, "weight service is unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		RunID        string `json:"run_id"`
		RetainNewest int    `json:"retain_newest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" || body.RetainNewest < 0 {
		http.Error(w, "run_id is required and retain_newest must be non-negative", http.StatusBadRequest)
		return
	}
	expired, err := h.weightService.Expire(r.Context(), body.RunID, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	removed, err := h.weightService.GarbageCollect(r.Context(), body.RunID, body.RetainNewest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"expired": expired, "safe_to_delete": removed})
}

func (h *APIHandler) rlRollbackWeightsHandler(w http.ResponseWriter, r *http.Request) {
	if h.weightService == nil {
		http.Error(w, "weight service is unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		RunID  string `json:"run_id"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" || body.Reason == "" {
		http.Error(w, "run_id and rollback reason are required", http.StatusBadRequest)
		return
	}
	version, err := h.weightService.Rollback(r.Context(), body.RunID, body.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if run, getErr := h.stateManager.GetRLRun(body.RunID); getErr == nil {
		run.PolicyVersion, run.RolloutDraining, run.UpdatedAt = version, true, time.Now()
		_ = h.stateManager.SaveRLRun(run)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"desired_version": version})
}

func (h *APIHandler) rlEvaluateWeightsHandler(w http.ResponseWriter, r *http.Request) {
	run, err := h.stateManager.GetRLRun(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	var snapshot contracts.RunSnapshot
	if err := json.Unmarshal([]byte(run.SnapshotJSON), &snapshot); err != nil {
		http.Error(w, "invalid run snapshot", http.StatusInternalServerError)
		return
	}
	var body struct {
		Metrics map[string]float64 `json:"metrics"`
		Phase   string             `json:"phase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid evaluation", http.StatusBadRequest)
		return
	}
	version := mux.Vars(r)["version"]
	evaluation, err := h.stateManager.GetRLEvaluation(run.ID, version)
	if err != nil {
		http.Error(w, "evaluation not found", http.StatusNotFound)
		return
	}
	rawMetrics, _ := json.Marshal(body.Metrics)
	evaluation.MetricsJSON = string(rawMetrics)
	gateErr := rlservice.EvaluateGates(snapshot.Spec.Evaluation.Gates, body.Metrics)
	if body.Phase == "canary" {
		if gateErr != nil {
			evaluation.CanaryState, evaluation.FailureReason = "failed", gateErr.Error()
		} else {
			evaluation.CanaryState = "passed"
		}
	} else {
		if gateErr != nil {
			evaluation.Status, evaluation.GatesPassed, evaluation.FailureReason = "failed", false, gateErr.Error()
		} else {
			evaluation.Status, evaluation.GatesPassed = "passed", true
		}
	}
	evaluation.UpdatedAt = time.Now()
	if err := h.stateManager.SaveRLEvaluation(evaluation); err != nil {
		http.Error(w, "failed to persist evaluation", http.StatusInternalServerError)
		return
	}
	if gateErr != nil {
		http.Error(w, gateErr.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) rlCommitCheckpointHandler(w http.ResponseWriter, r *http.Request) {
	runID := mux.Vars(r)["id"]
	run, err := h.stateManager.GetRLRun(runID)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	var body struct {
		AttemptID      string `json:"attempt_id"`
		OptimizerStep  int64  `json:"optimizer_step"`
		ResumeURI      string `json:"resume_uri"`
		ServingURI     string `json:"serving_uri"`
		PolicyVersion  string `json:"policy_version"`
		ManifestSHA256 string `json:"manifest_sha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AttemptID == "" || body.OptimizerStep < 0 ||
		body.ResumeURI == "" || body.PolicyVersion == "" || len(body.ManifestSHA256) != 64 {
		http.Error(w, "attempt_id, optimizer_step, resume_uri, policy_version, and manifest_sha256 are required", http.StatusBadRequest)
		return
	}
	if body.AttemptID != run.CurrentAttemptID {
		http.Error(w, "checkpoint attempt does not match current attempt", http.StatusConflict)
		return
	}
	checkpoint := &state.RLCheckpointRecord{
		ID:    "ckpt-" + runID + "-" + strconv.FormatInt(body.OptimizerStep, 10),
		RunID: runID, AttemptID: body.AttemptID, OptimizerStep: body.OptimizerStep,
		ResumeURI: body.ResumeURI, ServingURI: body.ServingURI, PolicyVersion: body.PolicyVersion,
		ManifestSHA256: body.ManifestSHA256, Status: "committed", CreatedAt: time.Now(),
	}
	if err := h.stateManager.SaveRLCheckpoint(checkpoint); err != nil {
		http.Error(w, "failed to commit checkpoint", http.StatusInternalServerError)
		return
	}
	if err := h.stateManager.SavePolicyVersion(&state.PolicyVersionRecord{
		ID: body.PolicyVersion, RunID: runID, OptimizerStep: body.OptimizerStep,
		ParentVersion: run.PolicyVersion, CheckpointID: checkpoint.ID, Status: "published", CreatedAt: time.Now(),
	}); err != nil {
		http.Error(w, "failed to persist policy version", http.StatusInternalServerError)
		return
	}
	run.CheckpointID, run.PolicyVersion, run.OptimizerStep, run.UpdatedAt = checkpoint.ID, body.PolicyVersion, body.OptimizerStep, time.Now()
	var snapshot contracts.RunSnapshot
	if json.Unmarshal([]byte(run.SnapshotJSON), &snapshot) == nil &&
		snapshot.Spec.Evaluation.EverySteps > 0 &&
		body.OptimizerStep > 0 &&
		body.OptimizerStep%int64(snapshot.Spec.Evaluation.EverySteps) == 0 {
		evaluation := &state.RLEvaluationRecord{
			ID: "eval-" + run.ID + "-" + body.PolicyVersion, RunID: run.ID,
			PolicyVersion: body.PolicyVersion, CheckpointID: checkpoint.ID,
			Status: "pending", CanaryState: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := h.stateManager.SaveRLEvaluation(evaluation); err != nil {
			http.Error(w, "failed to schedule periodic evaluation", http.StatusInternalServerError)
			return
		}
		run.Status, run.ObservedState, run.RolloutDraining = "evaluating", "evaluation-pending", true
	}
	if err := h.stateManager.SaveRLRun(run); err != nil {
		http.Error(w, "failed to update run checkpoint", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(checkpoint)
}

func (h *APIHandler) rlTrainerProgressHandler(w http.ResponseWriter, r *http.Request) {
	run, err := h.stateManager.GetRLRun(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	var body struct {
		AttemptID     string `json:"attempt_id"`
		OptimizerStep int64  `json:"optimizer_step"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AttemptID == "" || body.OptimizerStep < 0 {
		http.Error(w, "attempt_id and non-negative optimizer_step are required", http.StatusBadRequest)
		return
	}
	if body.AttemptID != run.CurrentAttemptID {
		http.Error(w, "trainer attempt is no longer authoritative", http.StatusConflict)
		return
	}
	if body.OptimizerStep < run.OptimizerStep {
		http.Error(w, "optimizer step cannot regress", http.StatusConflict)
		return
	}
	run.OptimizerStep, run.UpdatedAt = body.OptimizerStep, time.Now()
	if err := h.stateManager.SaveRLRun(run); err != nil {
		http.Error(w, "failed to persist trainer progress", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) rlReportUsageHandler(w http.ResponseWriter, r *http.Request) {
	run, err := h.stateManager.GetRLRun(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	var usage state.RLUsageRecord
	if err := json.NewDecoder(r.Body).Decode(&usage); err != nil {
		http.Error(w, "invalid usage record", http.StatusBadRequest)
		return
	}
	if usage.GPUSeconds < 0 || usage.CPUSeconds < 0 || usage.StorageByteHours < 0 || usage.NetworkBytes < 0 ||
		usage.GeneratedTokens < 0 || usage.DiscardedTokens < 0 || usage.SandboxSeconds < 0 || usage.EstimatedCostUSD < 0 {
		http.Error(w, "usage values cannot be negative", http.StatusBadRequest)
		return
	}
	usage.TenantID, usage.ProjectID, usage.RunID = run.TenantID, run.ProjectID, run.ID
	usage.RecordedAt = time.Now()
	if err := h.stateManager.SaveRLUsage(&usage); err != nil {
		http.Error(w, "failed to persist usage", http.StatusInternalServerError)
		return
	}
	for resource, value := range map[string]float64{
		"gpu": usage.GPUSeconds, "cpu": usage.CPUSeconds, "sandbox": usage.SandboxSeconds,
	} {
		observability.RLResourceSecondsTotal.WithLabelValues(run.TenantID, run.ProjectID, run.ID, resource).Add(value)
	}
	observability.RLEstimatedCostUSDTotal.WithLabelValues(run.TenantID, run.ProjectID, run.ID).Add(usage.EstimatedCostUSD)
	observability.RLDiscardedTokensTotal.WithLabelValues(run.TenantID, run.ID, "reported").Add(float64(usage.DiscardedTokens))
	w.WriteHeader(http.StatusNoContent)
}

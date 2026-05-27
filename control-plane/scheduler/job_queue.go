package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/bluequbit/faas/control-plane/observability"
	"github.com/bluequbit/faas/control-plane/providers"
	"github.com/bluequbit/faas/control-plane/rlevents"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/sirupsen/logrus"
)

const (
	defaultWorkers = 3
	maxRetries     = 3
	retryBaseDelay = 5 * time.Second
	pollInterval   = 2 * time.Second
)

// JobQueueDispatcher processes queued GPU jobs with fair FIFO scheduling.
type JobQueueDispatcher struct {
	sm       *state.StateManager
	registry *providers.Registry
	pool     *providers.BufferPool
	workers  int
	logger   *logrus.Logger
}

func NewJobQueueDispatcher(
	sm *state.StateManager,
	registry *providers.Registry,
	pool *providers.BufferPool,
	logger *logrus.Logger,
) *JobQueueDispatcher {
	return &JobQueueDispatcher{
		sm:       sm,
		registry: registry,
		pool:     pool,
		workers:  defaultWorkers,
		logger:   logger,
	}
}

// Start launches worker goroutines that poll for queued jobs.
func (d *JobQueueDispatcher) Start(ctx context.Context) {
	for i := 0; i < d.workers; i++ {
		go d.worker(ctx, i)
	}
}

func (d *JobQueueDispatcher) worker(ctx context.Context, id int) {
	d.logger.Infof("job_queue: worker %d started", id)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.processOne(ctx)
		}
	}
}

func (d *JobQueueDispatcher) processOne(ctx context.Context) {
	item, err := d.sm.ClaimNextJob()
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			d.logger.Warnf("job_queue: claim: %v", err)
		}
		return
	}
	d.logger.Infof("job_queue: dispatching job %s exec=%s gpu=%s image=%s priority=%d",
		item.ID, item.ExecutionID, item.GPUModel, item.DockerImage, item.Priority)

	env := map[string]string{}
	if item.EnvVarsJSON != "" {
		if err := json.Unmarshal([]byte(item.EnvVarsJSON), &env); err != nil {
			d.logger.Warnf("job_queue: decode env for %s: %v", item.ID, err)
			env = map[string]string{}
		}
	}

	runID := env["RUN_ID"]
	role := env["SKYSCALE_ROLE"]
	if role == "" {
		if exec, err := d.sm.GetExecution(item.ExecutionID); err == nil {
			role = exec.JobType
		}
	}
	// Workers and trainer need the policy inference URL before they can make progress.
	if runID != "" && role != "policy_server" {
		run, err := d.sm.GetRLRun(runID)
		if err == nil && run.PolicyServerURL == "" {
			observability.RecordJobDeferred(role, item.GPUModel)
			_ = d.sm.UpdateJobStatus(item.ID, "queued", "")
			d.logger.Infof("job_queue: deferred %s — policy server URL not ready for run %s", item.ID, runID)
			rlevents.Default.Record(runID, "queue", "info",
				fmt.Sprintf("deferred %s until policy server URL is registered", role))
			return
		}
	}
	if runID != "" {
		rlevents.Default.Record(runID, "queue", "info",
			fmt.Sprintf("dispatching %s job=%s gpu=%s", role, item.ID, item.GPUModel))
	}

	started := time.Now()

	spec := providers.DeploySpec{
		DockerImage:     item.DockerImage,
		GPUModel:        item.GPUModel,
		ProviderName:    item.ProviderName,
		ExecutionID:     item.ExecutionID,
		JobID:           item.ID,
		ControlPlaneURL: item.ControlPlaneURL,
		EnvVars:         env,
	}

	var result providers.DeployResult
	// Try warm pool first.
	if item.ProviderName == "" {
		if slot, ok := d.pool.Claim(item.GPUModel); ok {
			d.logger.Infof("job_queue: using warm slot %s for %s", slot.DeploymentID, item.GPUModel)
			result = providers.DeployResult{
				DeploymentID: slot.DeploymentID,
				ProviderName: slot.ProviderName,
				ProviderAddr: slot.ProviderAddr,
			}
		}
	}
	if result.DeploymentID == "" {
		// Cold path: provision via registry with retries.
		result, err = d.dispatchWithRetry(ctx, spec)
		if err != nil {
			observability.RecordJobDispatch(role, item.GPUModel, item.ProviderName, "failed", time.Since(started).Seconds())
			d.logger.Errorf("job_queue: dispatch failed for %s after %s: %v", item.ID, time.Since(started).Round(time.Second), err)
			if runID != "" {
				rlevents.Default.Record(runID, "queue", "error",
					fmt.Sprintf("%s dispatch failed: %v", role, err))
			}
			_ = d.sm.UpdateJobStatus(item.ID, "failed", err.Error())
			d.markExecFailed(item.ExecutionID, err.Error())
			return
		}
	}

	// Update execution with deployment ID so UI shows dseq.
	if exec, err := d.sm.GetExecution(item.ExecutionID); err == nil {
		exec.VMID = result.DeploymentID
		exec.Status = "running"
		_ = d.sm.SaveExecution(exec)
	}

	// Register as GPU VM.
	vm := &state.VM{
		ID:                result.DeploymentID,
		Status:            "busy",
		HardwareType:      "gpu",
		GPUModel:          item.GPUModel,
		ProviderAddr:      result.ProviderAddr,
		AkashDeploymentID: result.DeploymentID,
		CreatedAt:         time.Now(),
	}
	_ = d.sm.SaveVM(vm)

	if strings.HasPrefix(result.ProviderAddr, "http") {
		if exec, err := d.sm.GetExecution(item.ExecutionID); err == nil && exec.JobType == "policy_server" {
			if run, err := d.sm.GetRLRun(exec.FunctionID); err == nil {
				run.PolicyServerURL = result.ProviderAddr
				run.UpdatedAt = time.Now()
				_ = d.sm.SaveRLRun(run)
				d.logger.Infof("job_queue: policy server URL set for run %s", exec.FunctionID)
				rlevents.Default.Record(exec.FunctionID, "policy", "info",
					"policy URL set via dispatch: "+result.ProviderAddr)
			}
		}
	}

	elapsed := time.Since(started).Round(time.Second)
	observability.RecordJobDispatch(role, item.GPUModel, result.ProviderName, "success", elapsed.Seconds())
	_ = d.sm.UpdateJobStatus(item.ID, "done", "")
	d.logger.Infof("job_queue: job %s dispatched provider=%s deployment=%s addr=%s elapsed=%s",
		item.ID, result.ProviderName, result.DeploymentID, result.ProviderAddr, elapsed)
	if runID != "" {
		rlevents.Default.Record(runID, "queue", "info",
			fmt.Sprintf("%s dispatched via %s in %s (deployment=%s)", role, result.ProviderName, elapsed, result.DeploymentID))
	}
}

func (d *JobQueueDispatcher) dispatchWithRetry(ctx context.Context, spec providers.DeploySpec) (providers.DeployResult, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<uint(attempt-1))
			d.logger.Infof("job_queue: retry %d for %s after %s", attempt, spec.ExecutionID, delay)
			select {
			case <-ctx.Done():
				return providers.DeployResult{}, ctx.Err()
			case <-time.After(delay):
			}
		}
		result, err := d.registry.Dispatch(ctx, spec)
		if err == nil {
			return result, nil
		}
		d.logger.Warnf("job_queue: provider attempt %d for %s failed: %v", attempt+1, spec.ExecutionID, err)
		lastErr = err
	}
	return providers.DeployResult{}, fmt.Errorf("all retries exhausted: %w", lastErr)
}

func (d *JobQueueDispatcher) markExecFailed(executionID, errMsg string) {
	exec, err := d.sm.GetExecution(executionID)
	if err != nil {
		return
	}
	exec.Status = "error"
	exec.Error = errMsg
	exec.EndTime = time.Now()
	_ = d.sm.SaveExecution(exec)
}

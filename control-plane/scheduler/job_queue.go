package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/bluequbit/faas/control-plane/providers"
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
	d.logger.Infof("job_queue: dispatching job %s exec=%s gpu=%s", item.ID, item.ExecutionID, item.GPUModel)

	env := map[string]string{}
	if item.EnvVarsJSON != "" {
		if err := json.Unmarshal([]byte(item.EnvVarsJSON), &env); err != nil {
			d.logger.Warnf("job_queue: decode env for %s: %v", item.ID, err)
			env = map[string]string{}
		}
	}

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
			d.logger.Errorf("job_queue: dispatch failed for %s: %v", item.ID, err)
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

	_ = d.sm.UpdateJobStatus(item.ID, "done", "")
	d.logger.Infof("job_queue: job %s dispatched deployment=%s", item.ID, result.DeploymentID)
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

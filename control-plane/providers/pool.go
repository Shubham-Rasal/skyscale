package providers

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bluequbit/faas/control-plane/state"
	"github.com/sirupsen/logrus"
)

const (
	standbyTTL     = 2 * time.Hour
	refillInterval = 10 * time.Minute
)

// BufferPool maintains pre-warmed GPU slots across all providers.
type BufferPool struct {
	registry *Registry
	state    *state.StateManager
	targets  map[string]int // gpuModel → desired standby count
	logger   *logrus.Logger
}

// NewBufferPool reads POOL_SIZE_<GPUMODEL> env vars to determine targets.
// Example: POOL_SIZE_A100=1  POOL_SIZE_H100=1
func NewBufferPool(registry *Registry, sm *state.StateManager, logger *logrus.Logger) *BufferPool {
	targets := map[string]int{}
	knownModels := []string{"a100", "h100", "h200", "rtx4090", "rtx3090", "rtx3060", "t4"}
	for _, model := range knownModels {
		key := "POOL_SIZE_" + strings.ToUpper(strings.ReplaceAll(model, "-", ""))
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				targets[model] = n
			}
		}
	}
	return &BufferPool{registry: registry, state: sm, targets: targets, logger: logger}
}

// Start launches the background refill goroutine.
func (p *BufferPool) Start() {
	if len(p.targets) == 0 {
		p.logger.Info("pool: no POOL_SIZE_* env vars set, warm pool disabled")
		return
	}
	go func() {
		p.refill()
		t := time.NewTicker(refillInterval)
		defer t.Stop()
		for range t.C {
			p.refill()
		}
	}()
}

// Claim atomically grabs a pre-warmed slot for the given GPU model.
// Returns (slot, true) on success or (nil, false) if pool is empty.
func (p *BufferPool) Claim(gpuModel string) (*state.StandbyVM, bool) {
	vm, err := p.state.ClaimStandbyVM(gpuModel)
	if err != nil {
		return nil, false
	}
	return vm, true
}

func (p *BufferPool) refill() {
	for model, target := range p.targets {
		slots, err := p.state.ListStandbyVMs("standby")
		if err != nil {
			p.logger.Warnf("pool: list standby: %v", err)
			continue
		}
		count := 0
		for _, s := range slots {
			if s.GPUModel == model {
				count++
			}
		}
		for i := count; i < target; i++ {
			p.provision(model)
		}
	}
}

func (p *BufferPool) provision(gpuModel string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	execID := fmt.Sprintf("standby-%s-%d", gpuModel, time.Now().UnixMilli())
	spec := DeploySpec{
		DockerImage:     "alpine:latest", // minimal placeholder; real jobs replace this
		GPUModel:        gpuModel,
		ExecutionID:     execID,
		JobID:           execID,
		ControlPlaneURL: "",
		EnvVars:         map[string]string{"STANDBY": "1"},
	}
	result, err := p.registry.Dispatch(ctx, spec)
	if err != nil {
		p.logger.Warnf("pool: provision %s: %v", gpuModel, err)
		return
	}
	vm := &state.StandbyVM{
		ID:           execID,
		ProviderName: result.ProviderName,
		ProviderAddr: result.ProviderAddr,
		GPUModel:     gpuModel,
		DeploymentID: result.DeploymentID,
		Status:       "standby",
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(standbyTTL),
	}
	if err := p.state.SaveStandbyVM(vm); err != nil {
		p.logger.Warnf("pool: save standby vm: %v", err)
	} else {
		p.logger.Infof("pool: provisioned standby slot %s for %s", result.DeploymentID, gpuModel)
	}
}

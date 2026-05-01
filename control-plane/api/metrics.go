package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// GPUMetric describes the current state of a GPU used by an active job.
type GPUMetric struct {
	JobID       string  `json:"job_id"`
	GPUModel    string  `json:"gpu_model"`
	Provider    string  `json:"provider"`
	Utilization int     `json:"utilization_pct"`
	MemoryTotal int     `json:"memory_total_mb"`
	MemoryUsed  int     `json:"memory_used_mb"`
	StepPerSec  float64 `json:"steps_per_sec"`
	UpdatedAt   int64   `json:"updated_at"`
}

// GPUMetricsHandler is the exported HTTP handler for GET /api/metrics/gpu.
func (h *APIHandler) GPUMetricsHandler(w http.ResponseWriter, r *http.Request) {
	h.gpuMetricsHandler(w, r)
}

// gpuMetricsHandler returns GPU metrics for all active Akash GPU deployments.
// It reads the latest training metrics from the in-memory store to derive
// utilization info, and falls back to plausible simulated data when no
// active GPU jobs exist (useful for local dev demos).
func (h *APIHandler) gpuMetricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics := h.collectGPUMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *APIHandler) collectGPUMetrics() []GPUMetric {
	// Pull active GPU VMs from state
	vms, err := h.vmManager.ListVMs()
	if err != nil {
		h.logger.Errorf("gpuMetrics: list VMs: %v", err)
	}

	snapshot := h.trainingMetrics.Snapshot()
	now := time.Now().UnixMilli()

	var metrics []GPUMetric
	for _, vm := range vms {
		if vm.HardwareType != "gpu" || vm.Status != "busy" {
			continue
		}

		m := GPUMetric{
			JobID:       vm.AkashDeploymentID,
			GPUModel:    vm.GPUModel,
			Provider:    vm.ProviderAddr,
			MemoryTotal: vm.VRAMmb,
			UpdatedAt:   now,
		}

		// Derive live stats from the latest training metric for this job
		if ring, ok := snapshot[vm.AkashDeploymentID]; ok && len(ring) > 0 {
			last := ring[len(ring)-1]
			m.Utilization = last.GPUUtil
			if len(ring) >= 2 {
				prev := ring[len(ring)-2]
				stepDelta := float64(last.Step - prev.Step)
				timeDeltaSec := float64(last.Timestamp-prev.Timestamp) / 1000.0
				if timeDeltaSec > 0 {
					m.StepPerSec = stepDelta / timeDeltaSec
				}
			}
		}

		metrics = append(metrics, m)
	}

	// Fallback: return a single simulated GPU so the dashboard isn't empty
	if len(metrics) == 0 {
		metrics = []GPUMetric{
			{
				JobID:       "demo",
				GPUModel:    "nvidia-rtx3090",
				Provider:    "akash1simulatedprovider000000000000000000001",
				Utilization: 0,
				MemoryTotal: 24576,
				MemoryUsed:  0,
				StepPerSec:  0,
				UpdatedAt:   now,
			},
		}
	}

	return metrics
}

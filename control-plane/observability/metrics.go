package observability

import (
	"net/url"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	TrainingLoss = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyscale_training_loss",
			Help: "Latest training loss for a job/run.",
		},
		[]string{"run_id"},
	)
	TrainingEpisodeReward = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyscale_training_episode_reward",
			Help: "Latest episode reward for a job/run.",
		},
		[]string{"run_id"},
	)
	TrainingGPUUtil = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyscale_training_gpu_util",
			Help: "Latest GPU utilization percent for a job/run.",
		},
		[]string{"run_id"},
	)
	TrainingStep = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyscale_training_step",
			Help: "Latest training step for a job/run.",
		},
		[]string{"run_id"},
	)

	RLBufferSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyscale_rl_buffer_size",
			Help: "Unconsumed trajectories in the RL experience buffer.",
		},
		[]string{"run_id"},
	)
	RLTrajectoriesPushedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skyscale_rl_trajectories_pushed_total",
			Help: "Total trajectories pushed to the RL buffer.",
		},
		[]string{"run_id"},
	)
	RLTrajectoriesSampledTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skyscale_rl_trajectories_sampled_total",
			Help: "Total trajectories sampled from the RL buffer.",
		},
		[]string{"run_id"},
	)

	JobDispatchTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skyscale_job_dispatch_total",
			Help: "Total job dispatch attempts by outcome.",
		},
		[]string{"role", "gpu_model", "provider", "status"},
	)
	JobDispatchFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skyscale_job_dispatch_failures_total",
			Help: "Total failed job dispatches.",
		},
		[]string{"role", "gpu_model", "provider"},
	)
	JobDispatchDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "skyscale_job_dispatch_duration_seconds",
			Help:    "Job dispatch duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"role", "gpu_model", "provider"},
	)
	JobDeferredTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skyscale_job_deferred_total",
			Help: "Total jobs deferred waiting for dependencies.",
		},
		[]string{"role", "gpu_model"},
	)
)

func init() {
	prometheus.MustRegister(
		TrainingLoss,
		TrainingEpisodeReward,
		TrainingGPUUtil,
		TrainingStep,
		RLBufferSize,
		RLTrajectoriesPushedTotal,
		RLTrajectoriesSampledTotal,
		JobDispatchTotal,
		JobDispatchFailuresTotal,
		JobDispatchDurationSeconds,
		JobDeferredTotal,
	)
}

// RecordTrainingMetric updates Prometheus gauges for a training step.
func RecordTrainingMetric(runID string, step int, loss, reward float64, gpuUtil int) {
	if runID == "" {
		return
	}
	TrainingStep.WithLabelValues(runID).Set(float64(step))
	TrainingLoss.WithLabelValues(runID).Set(loss)
	TrainingEpisodeReward.WithLabelValues(runID).Set(reward)
	TrainingGPUUtil.WithLabelValues(runID).Set(float64(gpuUtil))
}

// RecordBufferPush updates buffer metrics after a trajectory push.
func RecordBufferPush(runID string, bufferSize int) {
	if runID == "" {
		return
	}
	RLTrajectoriesPushedTotal.WithLabelValues(runID).Inc()
	RLBufferSize.WithLabelValues(runID).Set(float64(bufferSize))
}

// RecordBufferSample updates buffer metrics after a trainer sample.
func RecordBufferSample(runID string, count int) {
	if runID == "" {
		return
	}
	RLTrajectoriesSampledTotal.WithLabelValues(runID).Add(float64(count))
}

// RecordBufferSize sets the current buffer size gauge.
func RecordBufferSize(runID string, bufferSize int) {
	if runID == "" {
		return
	}
	RLBufferSize.WithLabelValues(runID).Set(float64(bufferSize))
}

// RecordJobDeferred increments the deferred job counter.
func RecordJobDeferred(role, gpuModel string) {
	JobDeferredTotal.WithLabelValues(safeLabel(role), safeLabel(gpuModel)).Inc()
}

// RecordJobDispatch records dispatch outcome and duration.
func RecordJobDispatch(role, gpuModel, provider, status string, durationSeconds float64) {
	role = safeLabel(role)
	gpuModel = safeLabel(gpuModel)
	provider = safeLabel(provider)
	status = safeLabel(status)
	JobDispatchTotal.WithLabelValues(role, gpuModel, provider, status).Inc()
	if status == "failed" {
		JobDispatchFailuresTotal.WithLabelValues(role, gpuModel, provider).Inc()
	}
	if durationSeconds >= 0 {
		JobDispatchDurationSeconds.WithLabelValues(role, gpuModel, provider).Observe(durationSeconds)
	}
}

func safeLabel(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// GrafanaRunURL builds a deep link to the RL dashboard filtered by run_id.
func GrafanaRunURL(runID string) string {
	base := strings.TrimRight(os.Getenv("GRAFANA_BASE_URL"), "/")
	dashboardUID := os.Getenv("GRAFANA_RL_DASHBOARD_UID")
	if base == "" || dashboardUID == "" || runID == "" {
		return ""
	}
	u, err := url.Parse(base + "/d/" + dashboardUID + "/skyscale-rl-training")
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("var-run_id", runID)
	if org := os.Getenv("GRAFANA_ORG_ID"); org != "" {
		q.Set("orgId", org)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

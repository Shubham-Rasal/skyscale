# Observability (Prometheus + Grafana)

Skyscale exports RL and training metrics from the control plane at `GET /metrics`. Use Prometheus for time-series storage and Grafana for historical analysis, cross-run comparison, and alerting.

## Quick start

1. Start the control plane on port 8080 (see main README).
2. Launch the observability stack:

```bash
docker compose -f docker-compose.observability.yml up -d
```

3. Open Grafana at [http://localhost:3001](http://localhost:3001) (default login: `admin` / `admin`).
4. Open the **Skyscale RL Training** dashboard (UID: `skyscale-rl-training`).

Prometheus scrapes the control plane at `host.docker.internal:8080/metrics` every 15 seconds.

## Control plane configuration

Set these environment variables on the control plane to enable per-run Grafana links in the dashboard:

| Variable | Example | Purpose |
|----------|---------|---------|
| `GRAFANA_BASE_URL` | `http://localhost:3001` | Base URL for deep links |
| `GRAFANA_RL_DASHBOARD_UID` | `skyscale-rl-training` | Dashboard UID to open |
| `GRAFANA_ORG_ID` | `1` | Optional Grafana org ID |

When configured, `GET /api/rl/runs/{id}` includes a `grafana_url` field. The RL run detail panel shows an **Open in Grafana** button filtered to that run.

## Exported metrics

| Metric | Labels | Description |
|--------|--------|-------------|
| `skyscale_training_loss` | `run_id` | Latest training loss |
| `skyscale_training_episode_reward` | `run_id` | Latest episode reward |
| `skyscale_training_gpu_util` | `run_id` | Latest GPU utilization (%) |
| `skyscale_training_step` | `run_id` | Latest training step |
| `skyscale_rl_buffer_size` | `run_id` | Unconsumed trajectories in buffer |
| `skyscale_rl_trajectories_pushed_total` | `run_id` | Total trajectories pushed |
| `skyscale_rl_trajectories_sampled_total` | `run_id` | Total trajectories sampled |
| `skyscale_job_dispatch_total` | `role`, `gpu_model`, `provider`, `status` | Dispatch attempts |
| `skyscale_job_dispatch_failures_total` | `role`, `gpu_model`, `provider` | Failed dispatches |
| `skyscale_job_dispatch_duration_seconds` | `role`, `gpu_model`, `provider` | Dispatch latency histogram |
| `skyscale_job_deferred_total` | `role`, `gpu_model` | Jobs deferred waiting on dependencies |

Metrics are updated at ingest time (training POST, buffer push/sample, job dispatch) — no separate polling loop.

## Verify metrics

After starting a run or posting a test metric:

```bash
curl -s http://localhost:8080/metrics | grep skyscale_
```

Example test POST:

```bash
curl -X POST http://localhost:8080/api/training/metrics \
  -H 'Content-Type: application/json' \
  -d '{"job_id":"rl-test","step":1,"loss":0.42,"episode_reward":0.8,"gpu_util":75}'
```

## Production notes

- Expose `/metrics` on an internal network only; it is unauthenticated by design.
- `run_id` labels enable per-run filtering but increase cardinality — use Prometheus retention limits or recording rules at scale.
- The in-app Recharts panels remain for quick live views; Grafana is for historical analysis and SLOs.
- On Linux without Docker Desktop, update `observability/prometheus/prometheus.yml` to use your host IP instead of `host.docker.internal`.

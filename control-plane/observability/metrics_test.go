package observability

import (
	"os"
	"strings"
	"testing"
)

func TestGrafanaRunURL(t *testing.T) {
	t.Setenv("GRAFANA_BASE_URL", "http://localhost:3001")
	t.Setenv("GRAFANA_RL_DASHBOARD_UID", "skyscale-rl-training")
	t.Setenv("GRAFANA_ORG_ID", "1")

	got := GrafanaRunURL("rl-abc123")
	if got == "" {
		t.Fatal("expected non-empty URL")
	}
	if !strings.Contains(got, "var-run_id=rl-abc123") {
		t.Fatalf("URL missing run_id var: %s", got)
	}
	if !strings.Contains(got, "/d/skyscale-rl-training/") {
		t.Fatalf("URL missing dashboard path: %s", got)
	}
}

func TestGrafanaRunURLEmptyWhenUnset(t *testing.T) {
	os.Unsetenv("GRAFANA_BASE_URL")
	os.Unsetenv("GRAFANA_RL_DASHBOARD_UID")
	if got := GrafanaRunURL("rl-abc123"); got != "" {
		t.Fatalf("expected empty URL, got %q", got)
	}
}

func TestRecordTrainingMetricNoPanic(t *testing.T) {
	RecordTrainingMetric("", 1, 0.5, 0.8, 50)
	RecordTrainingMetric("rl-test", 1, 0.5, 0.8, 50)
}

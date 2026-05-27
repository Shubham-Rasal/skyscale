package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSpendAuthMiddlewareAllowsWhenUnset(t *testing.T) {
	os.Unsetenv("SKYSCALE_DASHBOARD_TOKEN")
	am, _ := NewAuthManager(logrus.New())
	called := false
	handler := am.SpendAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/training/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected handler to run when token unset")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSpendAuthMiddlewareRejectsMissingBearer(t *testing.T) {
	t.Setenv("SKYSCALE_DASHBOARD_TOKEN", "secret-token")
	am, _ := NewAuthManager(logrus.New())
	handler := am.SpendAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/training/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSpendAuthMiddlewareAcceptsDashboardToken(t *testing.T) {
	t.Setenv("SKYSCALE_DASHBOARD_TOKEN", "secret-token")
	am, _ := NewAuthManager(logrus.New())
	called := false
	handler := am.SpendAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/training/jobs", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected handler to run with dashboard token")
	}
}

func TestGpuInvokeBody(t *testing.T) {
	if !gpuInvokeBody([]byte(`{"hardware_type":"gpu"}`)) {
		t.Fatal("expected gpu hardware to require spend auth")
	}
	if !gpuInvokeBody([]byte(`{"job_type":"training_run","hardware_type":"cpu"}`)) {
		t.Fatal("expected training_run to require spend auth")
	}
	if gpuInvokeBody([]byte(`{"hardware_type":"cpu","job_type":"faas_function"}`)) {
		t.Fatal("expected cpu faas invoke to skip spend auth")
	}
}

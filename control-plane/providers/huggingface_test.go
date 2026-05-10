package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestHuggingFaceProviderDeployStartsJob(t *testing.T) {
	var got hfStartJobRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/jobs/test-ns" {
			t.Fatalf("path = %s, want /api/jobs/test-ns", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization header not set")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"job-123","url":"https://huggingface.co/jobs/test-ns/job-123"}`))
	}))
	defer server.Close()

	p := &HuggingFaceProvider{
		token:     "test-token",
		namespace: "test-ns",
		baseURL:   server.URL,
		http:      server.Client(),
		logger:    logrus.New(),
	}

	result, err := p.Deploy(context.Background(), DeploySpec{
		DockerImage:     "ghcr.io/example/trainer:latest",
		GPUModel:        "a100",
		EnvVars:         map[string]string{"EPOCHS": "1"},
		ExecutionID:     "exec-mnist",
		JobID:           "mnist",
		ControlPlaneURL: "https://cp.example.com",
	})
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}

	if result.DeploymentID != "job-123" {
		t.Fatalf("DeploymentID = %q, want job-123", result.DeploymentID)
	}
	if result.ProviderName != "huggingface" {
		t.Fatalf("ProviderName = %q, want huggingface", result.ProviderName)
	}
	if got.DockerImage != "ghcr.io/example/trainer:latest" {
		t.Fatalf("DockerImage = %q", got.DockerImage)
	}
	if got.Flavor != "a100-large" {
		t.Fatalf("Flavor = %q, want a100-large", got.Flavor)
	}
	if got.Environment["JOB_ID"] != "mnist" || got.Environment["EXECUTION_ID"] != "exec-mnist" {
		t.Fatalf("job env not injected: %#v", got.Environment)
	}
	if got.Environment["SKYSCALE_JOB_ID"] != "mnist" {
		t.Fatalf("SKYSCALE_JOB_ID = %q", got.Environment["SKYSCALE_JOB_ID"])
	}
	if got.Environment["CONTROL_PLANE_URL"] != "https://cp.example.com" {
		t.Fatalf("CONTROL_PLANE_URL = %q", got.Environment["CONTROL_PLANE_URL"])
	}
	if got.Environment["EPOCHS"] != "1" {
		t.Fatalf("EPOCHS = %q", got.Environment["EPOCHS"])
	}
}

func TestHuggingFaceProviderTerminateCancelsJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/jobs/test-ns/job-123/cancel" {
			t.Fatalf("path = %s, want /api/jobs/test-ns/job-123/cancel", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := &HuggingFaceProvider{
		token:     "test-token",
		namespace: "test-ns",
		baseURL:   server.URL,
		http:      server.Client(),
		logger:    logrus.New(),
	}

	if err := p.Terminate(context.Background(), "job-123"); err != nil {
		t.Fatalf("Terminate returned error: %v", err)
	}
}

func TestHuggingFaceProviderRejectsLocalCallbackURL(t *testing.T) {
	t.Setenv("SKYSCALE_PUBLIC_BASE", "")
	t.Setenv("HF_CALLBACK_BASE", "")

	p := &HuggingFaceProvider{
		token:     "test-token",
		namespace: "test-ns",
		baseURL:   "https://huggingface.co",
		http:      http.DefaultClient,
		logger:    logrus.New(),
	}

	_, err := p.Deploy(context.Background(), DeploySpec{
		DockerImage:     "ghcr.io/example/trainer:latest",
		GPUModel:        "a10g",
		ExecutionID:     "exec-job",
		JobID:           "job",
		ControlPlaneURL: "http://127.0.0.1:8080",
	})
	if err == nil {
		t.Fatal("expected local callback URL error")
	}
}

func TestHuggingFaceProviderUsesPublicCallbackEnvFallback(t *testing.T) {
	t.Setenv("SKYSCALE_PUBLIC_BASE", "https://cp.example.com/")

	var got hfStartJobRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"job-123"}`))
	}))
	defer server.Close()

	p := &HuggingFaceProvider{
		token:     "test-token",
		namespace: "test-ns",
		baseURL:   server.URL,
		http:      server.Client(),
		logger:    logrus.New(),
	}

	_, err := p.Deploy(context.Background(), DeploySpec{
		DockerImage:     "ghcr.io/example/trainer:latest",
		GPUModel:        "a10g",
		ExecutionID:     "exec-job",
		JobID:           "job",
		ControlPlaneURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}
	if got.Environment["CONTROL_PLANE_URL"] != "https://cp.example.com" {
		t.Fatalf("CONTROL_PLANE_URL = %q", got.Environment["CONTROL_PLANE_URL"])
	}
}

func TestHuggingFaceProviderSimulateWithoutToken(t *testing.T) {
	t.Setenv("SKYSCALE_PUBLIC_BASE", "")
	t.Setenv("HF_CALLBACK_BASE", "")

	p := &HuggingFaceProvider{
		simulate: true,
		logger:   logrus.New(),
	}

	result, err := p.Deploy(context.Background(), DeploySpec{
		DockerImage: "example/trainer:latest",
		GPUModel:    "a10g",
		JobID:       "job",
		ExecutionID: "exec-job",
	})
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}
	if result.ProviderName != "huggingface" {
		t.Fatalf("ProviderName = %q, want huggingface", result.ProviderName)
	}
	if result.DeploymentID == "" {
		t.Fatal("DeploymentID is empty")
	}
}

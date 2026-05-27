package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
)

// SpendAuthMiddleware requires a dashboard bearer token or valid API key when
// SKYSCALE_DASHBOARD_TOKEN is set. When unset, requests pass through (local dev).
func (a *AuthManager) SpendAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !spendAuthRequired() {
			next.ServeHTTP(w, r)
			return
		}
		if !a.authorizeSpend(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SpendAuthIfGPUMiddleware applies SpendAuth only for GPU/training invoke bodies.
func (a *AuthManager) SpendAuthIfGPUMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !spendAuthRequired() {
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		if gpuInvokeBody(body) && !a.authorizeSpend(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

func spendAuthRequired() bool {
	return os.Getenv("SKYSCALE_DASHBOARD_TOKEN") != ""
}

func (a *AuthManager) authorizeSpend(r *http.Request) bool {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return false
	}
	if token == os.Getenv("SKYSCALE_DASHBOARD_TOKEN") {
		return true
	}
	_, err := a.ValidateAPIKey(token)
	return err == nil
}

func bearerToken(authHeader string) string {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return parts[1]
}

func gpuInvokeBody(body []byte) bool {
	var req struct {
		HardwareType string `json:"hardware_type"`
		JobType      string `json:"job_type"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.HardwareType == "gpu" || req.JobType == "training_run"
}

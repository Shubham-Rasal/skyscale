package api

import (
	"encoding/json"
	"net/http"

	"github.com/bluequbit/faas/control-plane/deployment"
	"github.com/sirupsen/logrus"
)

// DeploymentHandler serves POST /api/deployments and GET /api/deployments.
type DeploymentHandler struct {
	manager *deployment.Manager
	logger  *logrus.Logger
}

// NewDeploymentHandler constructs a deployment API handler.
func NewDeploymentHandler(manager *deployment.Manager, logger *logrus.Logger) *DeploymentHandler {
	return &DeploymentHandler{manager: manager, logger: logger}
}

// CreateDeployment handles POST /api/deployments with a single DeploymentSpec JSON object.
func (h *DeploymentHandler) CreateDeployment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var spec deployment.DeploymentSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	dep, err := h.manager.Deploy(spec)
	if err != nil {
		h.logger.Errorf("deploy: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":  dep.ID,
		"url": dep.URL,
		"slug": dep.Slug,
	})
}

// ListDeployments handles GET /api/deployments.
func (h *DeploymentHandler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := h.manager.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

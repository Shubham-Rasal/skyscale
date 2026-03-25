package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bluequbit/faas/control-plane/sandbox"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// SandboxAPIHandler handles sandbox API requests
type SandboxAPIHandler struct {
	sandboxManager *sandbox.SandboxManager
	logger         *logrus.Logger
}

// NewSandboxAPIHandler creates a new SandboxAPIHandler
func NewSandboxAPIHandler(sandboxManager *sandbox.SandboxManager, logger *logrus.Logger) *SandboxAPIHandler {
	return &SandboxAPIHandler{
		sandboxManager: sandboxManager,
		logger:         logger,
	}
}

// RegisterSandboxRoutes registers sandbox routes on the given router
func (h *SandboxAPIHandler) RegisterSandboxRoutes(router *mux.Router) {
	sandboxes := router.PathPrefix("/api/sandboxes").Subrouter()
	sandboxes.HandleFunc("", h.createSandboxHandler).Methods(http.MethodPost)
	sandboxes.HandleFunc("", h.listSandboxesHandler).Methods(http.MethodGet)
	sandboxes.HandleFunc("/{id}", h.getSandboxHandler).Methods(http.MethodGet)
	sandboxes.HandleFunc("/{id}", h.destroySandboxHandler).Methods(http.MethodDelete)
	sandboxes.HandleFunc("/{id}/exec", h.execHandler).Methods(http.MethodPost)
	// File routes: match everything after /files/
	sandboxes.PathPrefix("/{id}/files/").HandlerFunc(h.fileHandler)
}

// ─── Request / Response types ────────────────────────────────────────────────

type createSandboxRequest struct {
	Runtime    string            `json:"runtime"`
	TTLSeconds int               `json:"ttl_seconds"`
	Metadata   map[string]string `json:"metadata"`
}

type execRequest struct {
	Code           string `json:"code"`
	Language       string `json:"language"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (h *SandboxAPIHandler) createSandboxHandler(w http.ResponseWriter, r *http.Request) {
	var req createSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	sb, err := h.sandboxManager.CreateSandbox(req.Runtime, req.TTLSeconds)
	if err != nil {
		h.logger.Errorf("Failed to create sandbox: %v", err)
		http.Error(w, "Failed to create sandbox: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sb)
}

func (h *SandboxAPIHandler) listSandboxesHandler(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := h.sandboxManager.ListSandboxes()
	if err != nil {
		http.Error(w, "Failed to list sandboxes", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sandboxes)
}

func (h *SandboxAPIHandler) getSandboxHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	sb, err := h.sandboxManager.GetSandbox(id)
	if err != nil {
		http.Error(w, "Sandbox not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sb)
}

func (h *SandboxAPIHandler) destroySandboxHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.sandboxManager.DestroySandbox(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Sandbox not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to destroy sandbox: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SandboxAPIHandler) execHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// Verify sandbox exists and is not destroyed before parsing body
	sb, err := h.sandboxManager.GetSandbox(id)
	if err != nil {
		http.Error(w, "Sandbox not found", http.StatusNotFound)
		return
	}
	if sb.Status == "destroyed" {
		http.Error(w, "Sandbox has been destroyed", http.StatusNotFound)
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.sandboxManager.ExecInSandbox(id, req.Code, req.Language, req.TimeoutSeconds)
	if err != nil {
		if strings.Contains(err.Error(), "busy") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		h.logger.Errorf("Sandbox exec error (sandbox %s): %v", id, err)
		http.Error(w, "Exec failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// fileHandler handles both upload (POST) and download (GET) of files in the sandbox workspace.
// URL pattern: /api/sandboxes/{id}/files/{path...}
func (h *SandboxAPIHandler) fileHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// Strip the prefix to get the relative file path
	prefix := "/api/sandboxes/" + id + "/files/"
	filePath := strings.TrimPrefix(r.URL.Path, prefix)
	if filePath == "" {
		http.Error(w, "Missing file path", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPost:
		if err := h.sandboxManager.UploadFile(id, filePath, r.Body); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	case http.MethodGet:
		if err := h.sandboxManager.DownloadFile(id, filePath, w); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, "Download failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}


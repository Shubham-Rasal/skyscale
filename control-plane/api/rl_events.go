package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bluequbit/faas/control-plane/rlevents"
	"github.com/gorilla/mux"
)

func recordRLEvent(runID, component, level, message string) {
	rlevents.Default.Record(runID, component, level, message)
}

// rlGetEventsHandler: GET /api/rl/runs/{id}/events?limit=100
func (h *APIHandler) rlGetEventsHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	events := rlevents.Default.List(id, limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"run_id": id,
		"events": events,
	})
}

// rlPostEventHandler: POST /api/rl/runs/{id}/events
func (h *APIHandler) rlPostEventHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Component string `json:"component"`
		Level     string `json:"level"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}
	component := body.Component
	if component == "" {
		component = "remote"
	}
	level := body.Level
	if level == "" {
		level = "info"
	}
	recordRLEvent(id, component, level, body.Message)
	h.logger.Infof("rl[%s] %s: %s", id, component, body.Message)
	w.WriteHeader(http.StatusAccepted)
}

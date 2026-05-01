package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/bluequbit/faas/control-plane/scheduler"
	"github.com/bluequbit/faas/control-plane/state"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSPayload is broadcast to every connected dashboard client every second.
type WSPayload struct {
	Active          []*scheduler.ExecutionContext        `json:"active"`
	Recent          []state.Execution                   `json:"recent"`
	VMPool          []state.VM                          `json:"vm_pool"`
	GPU             []GPUMetric                         `json:"gpu"`
	TrainingMetrics map[string][]TrainingMetric         `json:"training_metrics"`
}

// WSHandler manages the WebSocket broadcast loop.
type WSHandler struct {
	apiHandler *APIHandler
	logger     *logrus.Logger
}

func NewWSHandler(apiHandler *APIHandler, logger *logrus.Logger) *WSHandler {
	return &WSHandler{apiHandler: apiHandler, logger: logger}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Errorf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// close loop: detect client disconnect
	closeCh := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(closeCh)
				return
			}
		}
	}()

	for {
		select {
		case <-closeCh:
			return
		case <-ticker.C:
			payload, err := h.buildPayload()
			if err != nil {
				h.logger.Errorf("ws build payload: %v", err)
				continue
			}
			data, _ := json.Marshal(payload)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

func (h *WSHandler) buildPayload() (*WSPayload, error) {
	ah := h.apiHandler

	active := ah.scheduler.GetActiveExecutionSnapshot()

	recent, err := ah.stateManager.ListRecentExecutions(20)
	if err != nil {
		recent = []state.Execution{}
	}

	vms, err := ah.vmManager.ListVMs()
	if err != nil {
		vms = []state.VM{}
	}

	gpu := ah.collectGPUMetrics()
	training := ah.trainingMetrics.Snapshot()

	return &WSPayload{
		Active:          active,
		Recent:          recent,
		VMPool:          vms,
		GPU:             gpu,
		TrainingMetrics: training,
	}, nil
}

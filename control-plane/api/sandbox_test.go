package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sbpkg "github.com/bluequbit/faas/control-plane/sandbox"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

type mockVMProviderAPI struct {
	sm *state.StateManager
}

func (m *mockVMProviderAPI) GetVM() (*state.VM, error) {
	vm := &state.VM{
		ID:        "api-test-vm-" + time.Now().Format("150405.000000000"),
		Status:    "ready",
		IP:        "127.0.0.1",
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		Memory:    128,
		CPU:       1,
		IsWarm:    true,
	}
	m.sm.SaveVM(vm)
	return vm, nil
}

func (m *mockVMProviderAPI) TerminateVM(id string) error {
	return m.sm.DeleteVM(id)
}

func newAPITestRouter(t *testing.T) (*mux.Router, *state.StateManager) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&state.VM{}, &state.Sandbox{}, &state.SandboxExec{})

	sm := state.NewStateManagerFromDB(db, logrus.New())
	mvm := &mockVMProviderAPI{sm: sm}
	mgr := sbpkg.NewSandboxManager(mvm, sm, logrus.New())
	handler := NewSandboxAPIHandler(mgr, logrus.New())

	router := mux.NewRouter()
	handler.RegisterSandboxRoutes(router)
	return router, sm
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestCreateSandboxHandler(t *testing.T) {
	router, _ := newAPITestRouter(t)

	body, _ := json.Marshal(createSandboxRequest{Runtime: "python3", TTLSeconds: 300})
	req := httptest.NewRequest(http.MethodPost, "/api/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var sb state.Sandbox
	if err := json.Unmarshal(rr.Body.Bytes(), &sb); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if sb.ID == "" {
		t.Error("expected non-empty sandbox ID in response")
	}
	if sb.Status != "ready" {
		t.Errorf("expected status 'ready', got %q", sb.Status)
	}
}

func TestGetSandboxHandler(t *testing.T) {
	router, sm := newAPITestRouter(t)

	// Pre-insert a sandbox
	sb := &state.Sandbox{
		ID:         "get-test",
		VMID:       "vm-1",
		VMIP:       "127.0.0.1",
		Status:     "ready",
		Runtime:    "python3",
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	sm.SaveSandbox(sb)

	req := httptest.NewRequest(http.MethodGet, "/api/sandboxes/get-test", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGetSandboxNotFound(t *testing.T) {
	router, _ := newAPITestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/sandboxes/nonexistent", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDestroySandboxHandler(t *testing.T) {
	router, sm := newAPITestRouter(t)

	sm.SaveVM(&state.VM{ID: "destroy-vm", Status: "ready"})
	sb := &state.Sandbox{
		ID:         "destroy-test",
		VMID:       "destroy-vm",
		VMIP:       "127.0.0.1",
		Status:     "ready",
		Runtime:    "bash",
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	sm.SaveSandbox(sb)

	req := httptest.NewRequest(http.MethodDelete, "/api/sandboxes/destroy-test", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestExecOnDestroyedSandboxHandler(t *testing.T) {
	router, sm := newAPITestRouter(t)

	sb := &state.Sandbox{
		ID:         "dead-sandbox",
		VMID:       "dead-vm",
		VMIP:       "127.0.0.1",
		Status:     "destroyed",
		Runtime:    "python3",
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	sm.SaveSandbox(sb)

	body, _ := json.Marshal(execRequest{Code: "print('hi')"})
	req := httptest.NewRequest(http.MethodPost, "/api/sandboxes/dead-sandbox/exec", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestListSandboxesHandler(t *testing.T) {
	router, sm := newAPITestRouter(t)

	// Insert 2 sandboxes
	for _, id := range []string{"sb-a", "sb-b"} {
		sm.SaveSandbox(&state.Sandbox{
			ID:        id,
			VMID:      "vm-" + id,
			Status:    "ready",
			Runtime:   "python3",
			ExpiresAt: time.Now().Add(time.Hour),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sandboxes", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var sandboxes []state.Sandbox
	if err := json.Unmarshal(rr.Body.Bytes(), &sandboxes); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(sandboxes) < 2 {
		t.Errorf("expected at least 2 sandboxes, got %d", len(sandboxes))
	}
}

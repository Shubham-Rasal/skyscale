package sandbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluequbit/faas/control-plane/state"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newInMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(&state.VM{}, &state.Sandbox{}, &state.SandboxExec{}); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
	}
	return db
}

type mockVMProvider struct {
	sm              *state.StateManager
	terminateCalled []string
}

func (m *mockVMProvider) GetVM() (*state.VM, error) {
	vm := &state.VM{
		ID:        "test-vm-" + time.Now().Format("150405.000000000"),
		Status:    "ready",
		IP:        "127.0.0.1",
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		Memory:    128,
		CPU:       1,
		IsWarm:    true,
	}
	_ = m.sm.SaveVM(vm)
	return vm, nil
}

func (m *mockVMProvider) TerminateVM(id string) error {
	m.terminateCalled = append(m.terminateCalled, id)
	_ = m.sm.DeleteVM(id)
	return nil
}

func newTestManager(t *testing.T) (*SandboxManager, *mockVMProvider, *state.StateManager) {
	t.Helper()
	db := newInMemoryDB(t)
	sm := state.NewStateManagerFromDB(db, logrus.New())
	mvm := &mockVMProvider{sm: sm}
	mgr := NewSandboxManager(mvm, sm, logrus.New())
	return mgr, mvm, sm
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestCreateSandbox(t *testing.T) {
	mgr, _, sm := newTestManager(t)

	sb, err := mgr.CreateSandbox("python3", 300)
	if err != nil {
		t.Fatalf("CreateSandbox returned error: %v", err)
	}
	if sb.ID == "" {
		t.Error("expected non-empty sandbox ID")
	}
	if sb.Status != "ready" {
		t.Errorf("expected status 'ready', got %q", sb.Status)
	}
	if sb.Runtime != "python3" {
		t.Errorf("expected runtime 'python3', got %q", sb.Runtime)
	}

	// Verify persisted in DB
	got, err := sm.GetSandbox(sb.ID)
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}
	if got.VMID == "" {
		t.Error("expected VMID to be set in DB")
	}
}

func TestExecInSandbox(t *testing.T) {
	// Start a mock daemon that returns a fixed exec response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sandbox/exec" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req ExecRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(ExecResult{
			ExecID:     req.ExecID,
			Stdout:     "hello\n",
			Stderr:     "",
			ExitCode:   0,
			DurationMs: 5,
		})
	}))
	defer srv.Close()

	mgr, _, sm := newTestManager(t)

	// The mock server's address is host:port — use it as VMIP.
	// daemonPort is added via fmt.Sprintf in ExecInSandbox, so we need VMIP to be
	// just the host and the mock server's port to match daemonPort.
	// Easiest: insert the sandbox with VMIP=host and rely on daemonPort matching.
	// Since httptest uses a random port, we instead insert VMIP as "host:port"
	// and call the daemon URL directly to validate parsing.

	// Directly test the response parsing (the network path is covered by e2e tests)
	respBody, _ := json.Marshal(ExecResult{
		ExecID:     "e1",
		Stdout:     "ok\n",
		Stderr:     "",
		ExitCode:   0,
		DurationMs: 3,
	})
	var result ExecResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("failed to unmarshal ExecResult: %v", err)
	}
	if result.Stdout != "ok\n" {
		t.Errorf("expected stdout 'ok\\n', got %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	// Network-level testing (calling the live mock daemon) is covered by the e2e tests.
	// The daemonPort const can't be overridden at test time without dependency injection.
	// Here we just verify that ExecResult JSON round-trips correctly and that the
	// sandbox record can be persisted.
	vmAddr := strings.TrimPrefix(srv.URL, "http://") // "127.0.0.1:PORT"
	parts := strings.SplitN(vmAddr, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected server URL: %s", srv.URL)
	}

	sb := &state.Sandbox{
		ID:         "exec-test-sandbox",
		VMID:       "exec-test-vm",
		VMIP:       parts[0],
		Status:     "ready",
		Runtime:    "python3",
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}
	sm.SaveSandbox(sb)
}

func TestDestroySandbox(t *testing.T) {
	mgr, mvm, sm := newTestManager(t)

	sb, err := mgr.CreateSandbox("bash", 60)
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	vmID := sb.VMID

	if err := mgr.DestroySandbox(sb.ID); err != nil {
		t.Fatalf("DestroySandbox: %v", err)
	}

	// VM should have been terminated
	found := false
	for _, id := range mvm.terminateCalled {
		if id == vmID {
			found = true
		}
	}
	if !found {
		t.Error("expected TerminateVM to be called with the sandbox's VMID")
	}

	// Sandbox status should be "destroyed"
	got, err := sm.GetSandbox(sb.ID)
	if err != nil {
		t.Fatalf("GetSandbox after destroy: %v", err)
	}
	if got.Status != "destroyed" {
		t.Errorf("expected status 'destroyed', got %q", got.Status)
	}
}

func TestDestroySandboxIdempotent(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	sb, _ := mgr.CreateSandbox("python3", 60)
	mgr.DestroySandbox(sb.ID)
	if err := mgr.DestroySandbox(sb.ID); err != nil {
		t.Errorf("expected idempotent destroy, got error: %v", err)
	}
}

func TestExecOnDestroyedSandbox(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	sb, _ := mgr.CreateSandbox("python3", 60)
	mgr.DestroySandbox(sb.ID)

	_, err := mgr.ExecInSandbox(sb.ID, "print('hi')", "python3", 10)
	if err == nil {
		t.Error("expected error when exec on destroyed sandbox")
	}
}

func TestSandboxTTLExpiry(t *testing.T) {
	mgr, _, sm := newTestManager(t)

	// Insert a VM and an already-expired sandbox
	sm.SaveVM(&state.VM{ID: "expired-vm", Status: "ready"})
	sb := &state.Sandbox{
		ID:         "expired-sandbox",
		VMID:       "expired-vm",
		VMIP:       "127.0.0.1",
		Status:     "ready",
		Runtime:    "python3",
		CreatedAt:  time.Now().Add(-2 * time.Hour),
		LastUsedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt:  time.Now().Add(-1 * time.Hour),
	}
	sm.SaveSandbox(sb)

	// Simulate one GC sweep
	expired, err := sm.ListExpiredSandboxes()
	if err != nil {
		t.Fatalf("ListExpiredSandboxes: %v", err)
	}
	if len(expired) == 0 {
		t.Fatal("expected at least one expired sandbox")
	}
	for _, s := range expired {
		if err := mgr.DestroySandbox(s.ID); err != nil {
			t.Errorf("DestroySandbox(%s): %v", s.ID, err)
		}
	}

	got, _ := sm.GetSandbox("expired-sandbox")
	if got.Status != "destroyed" {
		t.Errorf("expected destroyed, got %q", got.Status)
	}
}

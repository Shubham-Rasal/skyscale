package state

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *StateManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&Function{}, &Execution{}, &VM{}, &Sandbox{}, &SandboxExec{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStateManagerFromDB(db, logrus.New())
}

func TestSandboxCRUD(t *testing.T) {
	sm := newTestDB(t)

	sb := &Sandbox{
		ID:         "test-sb-1",
		VMID:       "vm-1",
		VMIP:       "127.0.0.1",
		Status:     "ready",
		Runtime:    "python3",
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}

	// Save
	if err := sm.SaveSandbox(sb); err != nil {
		t.Fatalf("SaveSandbox: %v", err)
	}

	// Get
	got, err := sm.GetSandbox("test-sb-1")
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}
	if got.VMID != "vm-1" {
		t.Errorf("expected VMID 'vm-1', got %q", got.VMID)
	}

	// Update
	got.Status = "destroyed"
	sm.SaveSandbox(got)
	updated, _ := sm.GetSandbox("test-sb-1")
	if updated.Status != "destroyed" {
		t.Errorf("expected status 'destroyed', got %q", updated.Status)
	}

	// Delete
	if err := sm.DeleteSandbox("test-sb-1"); err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}
	_, err = sm.GetSandbox("test-sb-1")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestListSandboxes(t *testing.T) {
	sm := newTestDB(t)

	for _, id := range []string{"s1", "s2", "s3"} {
		sm.SaveSandbox(&Sandbox{
			ID:        id,
			VMID:      "vm",
			Status:    "ready",
			ExpiresAt: time.Now().Add(time.Hour),
		})
	}

	sandboxes, err := sm.ListSandboxes()
	if err != nil {
		t.Fatalf("ListSandboxes: %v", err)
	}
	if len(sandboxes) != 3 {
		t.Errorf("expected 3 sandboxes, got %d", len(sandboxes))
	}
}

func TestListExpiredSandboxes(t *testing.T) {
	sm := newTestDB(t)

	sm.SaveSandbox(&Sandbox{
		ID:        "expired",
		VMID:      "vm",
		Status:    "ready",
		ExpiresAt: time.Now().Add(-time.Hour), // past
	})
	sm.SaveSandbox(&Sandbox{
		ID:        "active",
		VMID:      "vm",
		Status:    "ready",
		ExpiresAt: time.Now().Add(time.Hour), // future
	})
	sm.SaveSandbox(&Sandbox{
		ID:        "already-destroyed",
		VMID:      "vm",
		Status:    "destroyed",
		ExpiresAt: time.Now().Add(-time.Hour), // past but already destroyed
	})

	expired, err := sm.ListExpiredSandboxes()
	if err != nil {
		t.Fatalf("ListExpiredSandboxes: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != "expired" {
		t.Errorf("expected 1 expired sandbox with ID 'expired', got %v", expired)
	}
}

func TestSandboxExecCRUD(t *testing.T) {
	sm := newTestDB(t)

	exec := &SandboxExec{
		ID:         "exec-1",
		SandboxID:  "sb-1",
		Language:   "python3",
		Status:     "done",
		Stdout:     "hello\n",
		ExitCode:   0,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
		DurationMs: 42,
	}

	if err := sm.SaveSandboxExec(exec); err != nil {
		t.Fatalf("SaveSandboxExec: %v", err)
	}

	got, err := sm.GetSandboxExec("exec-1")
	if err != nil {
		t.Fatalf("GetSandboxExec: %v", err)
	}
	if got.Stdout != "hello\n" {
		t.Errorf("expected stdout 'hello\\n', got %q", got.Stdout)
	}
}

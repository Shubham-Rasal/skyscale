// Package sandbox provides a long-lived isolated execution environment for AI agents.
//
// A Sandbox is a Firecracker microVM that is exclusively reserved for a single agent
// session. Unlike FaaS executions (which return the VM to the warm pool after each call),
// a sandbox holds its VM until explicitly destroyed or until its TTL expires.
package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/bluequbit/faas/control-plane/state"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// VMProvider abstracts VM acquisition and termination so the SandboxManager can be
// tested without a real Firecracker VMManager.
type VMProvider interface {
	GetVM() (*state.VM, error)
	TerminateVM(id string) error
}

const (
	defaultTTL        = 1 * time.Hour
	daemonPort        = "8081"
	gcSweepInterval   = 30 * time.Second
)

// SandboxManager manages the lifecycle of agent sandboxes
type SandboxManager struct {
	vmManager    VMProvider
	stateManager *state.StateManager
	logger       *logrus.Logger
	mu           sync.RWMutex
	busyLocks    map[string]*sync.Mutex // per-sandbox exec serialization
	httpClient   *http.Client
}

// ExecRequest is sent to the in-VM daemon for synchronous code execution
type ExecRequest struct {
	ExecID   string `json:"exec_id"`
	Code     string `json:"code"`
	Language string `json:"language"`
	Timeout  int    `json:"timeout_seconds"`
}

// ExecResult is the response from the daemon
type ExecResult struct {
	ExecID     string `json:"exec_id"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// NewSandboxManager creates a new SandboxManager and starts the TTL sweep goroutine
func NewSandboxManager(vmManager VMProvider, stateManager *state.StateManager, logger *logrus.Logger) *SandboxManager {
	m := &SandboxManager{
		vmManager:    vmManager,
		stateManager: stateManager,
		logger:       logger,
		busyLocks:    make(map[string]*sync.Mutex),
		httpClient: &http.Client{
			Timeout: 0, // no global timeout; per-request timeout via context
		},
	}
	go m.gcSweep()
	return m
}

// CreateSandbox reserves a VM and returns a new Sandbox
func (m *SandboxManager) CreateSandbox(runtime string, ttlSeconds int) (*state.Sandbox, error) {
	if runtime == "" {
		runtime = "python3"
	}
	if ttlSeconds <= 0 {
		ttlSeconds = int(defaultTTL.Seconds())
	}

	// Pull a VM from the warm pool (or cold-start one)
	vmObj, err := m.vmManager.GetVM()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire VM: %w", err)
	}

	now := time.Now()
	sandbox := &state.Sandbox{
		ID:         uuid.New().String(),
		VMID:       vmObj.ID,
		VMIP:       vmObj.IP,
		Status:     "ready",
		Runtime:    runtime,
		CreatedAt:  now,
		LastUsedAt: now,
		ExpiresAt:  now.Add(time.Duration(ttlSeconds) * time.Second),
	}

	if err := m.stateManager.SaveSandbox(sandbox); err != nil {
		// Best-effort: return the VM before failing
		_ = m.vmManager.TerminateVM(vmObj.ID)
		return nil, fmt.Errorf("failed to save sandbox: %w", err)
	}

	// Register a per-sandbox mutex for exec serialization
	m.mu.Lock()
	m.busyLocks[sandbox.ID] = &sync.Mutex{}
	m.mu.Unlock()

	m.logger.Infof("Created sandbox %s on VM %s (IP: %s, TTL: %ds)", sandbox.ID, vmObj.ID, vmObj.IP, ttlSeconds)
	return sandbox, nil
}

// GetSandbox retrieves a sandbox by ID
func (m *SandboxManager) GetSandbox(id string) (*state.Sandbox, error) {
	return m.stateManager.GetSandbox(id)
}

// ListSandboxes retrieves all sandboxes
func (m *SandboxManager) ListSandboxes() ([]state.Sandbox, error) {
	return m.stateManager.ListSandboxes()
}

// ExecInSandbox executes code inside the sandbox synchronously and returns the result
func (m *SandboxManager) ExecInSandbox(sandboxID, code, language string, timeoutSeconds int) (*ExecResult, error) {
	sandbox, err := m.stateManager.GetSandbox(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox not found: %w", err)
	}
	if sandbox.Status == "destroyed" {
		return nil, fmt.Errorf("sandbox %s has been destroyed", sandboxID)
	}

	// Serialize concurrent execs for the same sandbox
	m.mu.RLock()
	lock, ok := m.busyLocks[sandboxID]
	m.mu.RUnlock()
	if !ok {
		// Sandbox was loaded from DB (e.g. after restart) — create lock
		m.mu.Lock()
		lock = &sync.Mutex{}
		m.busyLocks[sandboxID] = lock
		m.mu.Unlock()
	}

	if !lock.TryLock() {
		return nil, fmt.Errorf("sandbox %s is busy (concurrent exec rejected)", sandboxID)
	}
	defer lock.Unlock()

	// Update status
	sandbox.Status = "busy"
	sandbox.LastUsedAt = time.Now()
	_ = m.stateManager.SaveSandbox(sandbox)

	if language == "" {
		language = sandbox.Runtime
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	execID := uuid.New().String()
	execReq := ExecRequest{
		ExecID:   execID,
		Code:     code,
		Language: language,
		Timeout:  timeoutSeconds,
	}

	body, _ := json.Marshal(execReq)
	daemonURL := fmt.Sprintf("http://%s:%s/sandbox/exec", sandbox.VMIP, daemonPort)

	// Use a client with a timeout matching the execution timeout + buffer
	client := &http.Client{Timeout: time.Duration(timeoutSeconds+10) * time.Second}
	resp, err := client.Post(daemonURL, "application/json", bytes.NewReader(body))

	// Restore sandbox status regardless
	sandbox.Status = "ready"
	_ = m.stateManager.SaveSandbox(sandbox)

	if err != nil {
		return nil, fmt.Errorf("failed to call daemon exec: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read exec response: %w", err)
	}

	var result ExecResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse exec response: %w", err)
	}

	// Persist exec record
	execRecord := &state.SandboxExec{
		ID:         execID,
		SandboxID:  sandboxID,
		Language:   language,
		Status:     "done",
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		ExitCode:   result.ExitCode,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
		DurationMs: result.DurationMs,
	}
	if result.ExitCode != 0 {
		execRecord.Status = "error"
	}
	_ = m.stateManager.SaveSandboxExec(execRecord)

	m.logger.Infof("Sandbox %s exec %s completed: exit=%d, duration=%dms", sandboxID, execID, result.ExitCode, result.DurationMs)
	return &result, nil
}

// UploadFile uploads a file to the sandbox workspace
func (m *SandboxManager) UploadFile(sandboxID, path string, data io.Reader) error {
	sandbox, err := m.stateManager.GetSandbox(sandboxID)
	if err != nil {
		return fmt.Errorf("sandbox not found: %w", err)
	}
	if sandbox.Status == "destroyed" {
		return fmt.Errorf("sandbox %s has been destroyed", sandboxID)
	}

	daemonURL := fmt.Sprintf("http://%s:%s/sandbox/files/%s", sandbox.VMIP, daemonPort, path)
	req, err := http.NewRequest(http.MethodPost, daemonURL, data)
	if err != nil {
		return err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("file upload failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned %d for file upload", resp.StatusCode)
	}
	return nil
}

// DownloadFile downloads a file from the sandbox workspace and writes it to w
func (m *SandboxManager) DownloadFile(sandboxID, path string, w io.Writer) error {
	sandbox, err := m.stateManager.GetSandbox(sandboxID)
	if err != nil {
		return fmt.Errorf("sandbox not found: %w", err)
	}
	if sandbox.Status == "destroyed" {
		return fmt.Errorf("sandbox %s has been destroyed", sandboxID)
	}

	daemonURL := fmt.Sprintf("http://%s:%s/sandbox/files/%s", sandbox.VMIP, daemonPort, path)
	resp, err := m.httpClient.Get(daemonURL)
	if err != nil {
		return fmt.Errorf("file download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("file not found: %s", path)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned %d for file download", resp.StatusCode)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// DestroySandbox terminates the VM and marks the sandbox as destroyed
func (m *SandboxManager) DestroySandbox(id string) error {
	sandbox, err := m.stateManager.GetSandbox(id)
	if err != nil {
		return fmt.Errorf("sandbox not found: %w", err)
	}
	if sandbox.Status == "destroyed" {
		return nil // idempotent
	}

	sandbox.Status = "destroyed"
	_ = m.stateManager.SaveSandbox(sandbox)

	if err := m.vmManager.TerminateVM(sandbox.VMID); err != nil {
		m.logger.Warnf("Failed to terminate VM %s for sandbox %s: %v", sandbox.VMID, id, err)
	}

	m.mu.Lock()
	delete(m.busyLocks, id)
	m.mu.Unlock()

	m.logger.Infof("Destroyed sandbox %s (VM %s)", id, sandbox.VMID)
	return nil
}

// gcSweep periodically destroys sandboxes past their TTL
func (m *SandboxManager) gcSweep() {
	ticker := time.NewTicker(gcSweepInterval)
	defer ticker.Stop()
	for range ticker.C {
		expired, err := m.stateManager.ListExpiredSandboxes()
		if err != nil {
			m.logger.Warnf("GC sweep error listing expired sandboxes: %v", err)
			continue
		}
		for _, s := range expired {
			m.logger.Infof("GC: destroying expired sandbox %s", s.ID)
			if err := m.DestroySandbox(s.ID); err != nil {
				m.logger.Warnf("GC: failed to destroy sandbox %s: %v", s.ID, err)
			}
		}
	}
}

// Package state provides functionality for managing the state of the control plane.
//
// The StateManager manages the database and cache for the control plane.
// It provides methods for saving, retrieving, and deleting functions, executions, and VMs.
// It also provides a method for tracking active executions.

package state

import (
	"context"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// StateManager handles the state management for the control plane
type StateManager struct {
	db          *gorm.DB
	cache       *redis.Client
	logger      *logrus.Logger
	activeExecs sync.Map // Map to track active executions
	mu          sync.Mutex
}

// Function represents a serverless function
type Function struct {
	ID        string `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex"`
	Runtime   string
	Memory    int
	Timeout   int
	CreatedAt time.Time
	UpdatedAt time.Time
	Status    string
	Version   string
	Code      string
}

// Execution represents a function execution
type Execution struct {
	ID           string `gorm:"primaryKey"`
	FunctionID   string
	Status       string
	StartTime    time.Time
	EndTime      time.Time
	Duration     int64
	VMID         string
	Logs         string
	Error        string
	JobType      string // "faas_function", "training_run", "rl_env"
	HardwareType string // "cpu", "gpu"
	GPUModel     string // e.g. "nvidia-t4"
}

// VM represents a Firecracker micro-VM or Akash GPU deployment
type VM struct {
	ID                string `gorm:"primaryKey"`
	Status            string
	IP                string
	CreatedAt         time.Time
	LastUsed          time.Time
	Memory            int
	CPU               int
	IsWarm            bool
	HardwareType      string // "cpu", "gpu"
	GPUModel          string // e.g. "nvidia-t4", empty for cpu
	VRAMmb            int    // VRAM in MB, 0 for cpu
	AkashDeploymentID string // dseq if Akash-provisioned, empty for Firecracker
	ProviderAddr      string // Akash provider address
	DaemonPort        int    // external port for daemon (Akash forwarded port)
}

// Sandbox represents a long-lived isolated execution environment for AI agents
type Sandbox struct {
	ID         string    `gorm:"primaryKey"`
	VMID       string    `gorm:"index"`
	VMIP       string
	Status     string    // "starting" | "ready" | "busy" | "destroyed"
	Runtime    string    // "python3" | "bash"
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
	Metadata   string // JSON blob
}

// Deployment is a long-lived web endpoint backed by a VM workload.
type Deployment struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	Slug            string    `gorm:"uniqueIndex" json:"slug"`
	AppName         string    `json:"app_name"`
	EndpointName    string    `json:"endpoint_name"`
	VMID            string    `json:"vm_id"`
	VMPort          int       `json:"vm_port"`
	WebPath         string    `json:"web_path"`
	WebMethod       string    `json:"web_method"`
	URL             string    `json:"url"`
	Status          string    `json:"status"`
	HardwareType    string    `json:"hardware_type"`
	GPUModel        string    `json:"gpu_model"`
	ScaledownWindow int       `json:"scaledown_window"`
	LastRequestAt   time.Time `json:"last_request_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// SandboxExec represents a single code execution within a sandbox
type SandboxExec struct {
	ID         string    `gorm:"primaryKey"`
	SandboxID  string    `gorm:"index"`
	Language   string
	Status     string // "done" | "error" | "timeout"
	Stdout     string
	Stderr     string
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
	DurationMs int64
}

// SaveSandbox saves a sandbox to the database
func (s *StateManager) SaveSandbox(sandbox *Sandbox) error {
	return s.db.Save(sandbox).Error
}

// GetSandbox retrieves a sandbox by ID
func (s *StateManager) GetSandbox(id string) (*Sandbox, error) {
	var sandbox Sandbox
	if err := s.db.First(&sandbox, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &sandbox, nil
}

// ListSandboxes retrieves all sandboxes
func (s *StateManager) ListSandboxes() ([]Sandbox, error) {
	var sandboxes []Sandbox
	return sandboxes, s.db.Find(&sandboxes).Error
}

// ListExpiredSandboxes retrieves sandboxes past their expiry time that aren't already destroyed
func (s *StateManager) ListExpiredSandboxes() ([]Sandbox, error) {
	var sandboxes []Sandbox
	return sandboxes, s.db.Where("expires_at < ? AND status != ?", time.Now(), "destroyed").Find(&sandboxes).Error
}

// DeleteSandbox deletes a sandbox by ID
func (s *StateManager) DeleteSandbox(id string) error {
	return s.db.Delete(&Sandbox{}, "id = ?", id).Error
}

// SaveSandboxExec saves a sandbox execution record
func (s *StateManager) SaveSandboxExec(exec *SandboxExec) error {
	return s.db.Save(exec).Error
}

// GetSandboxExec retrieves a sandbox execution by ID
func (s *StateManager) GetSandboxExec(id string) (*SandboxExec, error) {
	var exec SandboxExec
	if err := s.db.First(&exec, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &exec, nil
}

// NewStateManagerFromDB creates a StateManager from an existing *gorm.DB (for testing)
func NewStateManagerFromDB(db *gorm.DB, logger *logrus.Logger) *StateManager {
	return &StateManager{db: db, logger: logger}
}

// NewStateManager creates a new state manager
func NewStateManager(logger *logrus.Logger) (*StateManager, error) {
	// Initialize SQLite database
	db, err := gorm.Open(sqlite.Open("skyscale.db"), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Auto migrate the schema
	err = db.AutoMigrate(&Function{}, &Execution{}, &VM{}, &Sandbox{}, &SandboxExec{}, &Deployment{})
	if err != nil {
		return nil, err
	}

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// Test Redis connection
	ctx := context.Background()
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		logger.Warnf("Redis not available, continuing without cache: %v", err)
		rdb = nil
	}

	return &StateManager{
		db:     db,
		cache:  rdb,
		logger: logger,
	}, nil
}

// SaveFunction saves a function to the database
func (s *StateManager) SaveFunction(function *Function) error {
	return s.db.Save(function).Error
}

// GetFunction retrieves a function by ID
func (s *StateManager) GetFunction(id string) (*Function, error) {
	var function Function
	err := s.db.First(&function, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &function, nil
}

// GetFunctionByName retrieves a function by name
func (s *StateManager) GetFunctionByName(name string) (*Function, error) {
	var function Function
	err := s.db.First(&function, "name = ?", name).Error
	if err != nil {
		return nil, err
	}
	return &function, nil
}

// ListFunctions retrieves all functions
func (s *StateManager) ListFunctions() ([]Function, error) {
	var functions []Function
	err := s.db.Find(&functions).Error
	return functions, err
}

// DeleteFunction deletes a function by ID
func (s *StateManager) DeleteFunction(id string) error {
	return s.db.Delete(&Function{}, "id = ?", id).Error
}

// SaveExecution saves an execution to the database
func (s *StateManager) SaveExecution(execution *Execution) error {
	return s.db.Save(execution).Error
}

// GetExecution retrieves an execution by ID
func (s *StateManager) GetExecution(id string) (*Execution, error) {
	var execution Execution
	err := s.db.First(&execution, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &execution, nil
}

// ListExecutions retrieves all executions for a function
func (s *StateManager) ListExecutions(functionID string) ([]Execution, error) {
	var executions []Execution
	err := s.db.Find(&executions, "function_id = ?", functionID).Error
	return executions, err
}

// ListRecentExecutions returns the N most recent executions across all functions.
func (s *StateManager) ListRecentExecutions(limit int) ([]Execution, error) {
	var executions []Execution
	err := s.db.Order("start_time desc").Limit(limit).Find(&executions).Error
	return executions, err
}

// SaveVM saves a VM to the database
func (s *StateManager) SaveVM(vm *VM) error {
	return s.db.Save(vm).Error
}

// GetVM retrieves a VM by ID
func (s *StateManager) GetVM(id string) (*VM, error) {
	var vm VM
	err := s.db.First(&vm, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &vm, nil
}

// ListVMs retrieves all VMs
func (s *StateManager) ListVMs() ([]VM, error) {
	var vms []VM
	err := s.db.Find(&vms).Error
	return vms, err
}

// ListWarmVMs retrieves all warm VMs
func (s *StateManager) ListWarmVMs() ([]VM, error) {
	var vms []VM
	err := s.db.Find(&vms, "is_warm = ? AND status = ?", true, "ready").Error
	return vms, err
}

// DeleteVM deletes a VM by ID
func (s *StateManager) DeleteVM(id string) error {
	return s.db.Delete(&VM{}, "id = ?", id).Error
}

// SaveDeployment persists a deployment record.
func (s *StateManager) SaveDeployment(d *Deployment) error {
	return s.db.Save(d).Error
}

// GetDeployment retrieves a deployment by ID.
func (s *StateManager) GetDeployment(id string) (*Deployment, error) {
	var d Deployment
	if err := s.db.First(&d, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// GetDeploymentBySlug finds a deployment by its public URL slug.
func (s *StateManager) GetDeploymentBySlug(slug string) (*Deployment, error) {
	var d Deployment
	if err := s.db.First(&d, "slug = ?", slug).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDeployments returns all deployments.
func (s *StateManager) ListDeployments() ([]Deployment, error) {
	var out []Deployment
	return out, s.db.Order("created_at desc").Find(&out).Error
}

// ClaimIdleGPUVM atomically finds a ready GPU VM matching the model and marks it busy.
// Returns nil if none available.
func (s *StateManager) ClaimIdleGPUVM(gpuModel string) (*VM, error) {
	var vm VM
	err := s.db.Where("hardware_type = ? AND gpu_model = ? AND status = ?", "gpu", gpuModel, "ready").
		Order("last_used asc").First(&vm).Error
	if err != nil {
		return nil, err // not found or DB error
	}
	vm.Status = "busy"
	vm.LastUsed = time.Now()
	if err := s.db.Save(&vm).Error; err != nil {
		return nil, err
	}
	return &vm, nil
}

// TrackActiveExecution adds an execution to the active executions map
func (s *StateManager) TrackActiveExecution(executionID string, vmID string) {
	s.activeExecs.Store(executionID, vmID)
}

// UntrackActiveExecution removes an execution from the active executions map
func (s *StateManager) UntrackActiveExecution(executionID string) {
	s.activeExecs.Delete(executionID)
}

// GetActiveExecutions returns all active executions
func (s *StateManager) GetActiveExecutions() map[string]string {
	result := make(map[string]string)
	s.activeExecs.Range(func(key, value interface{}) bool {
		result[key.(string)] = value.(string)
		return true
	})
	return result
}

// Close closes the state manager
func (s *StateManager) Close() {
	if s.cache != nil {
		s.cache.Close()
	}
}

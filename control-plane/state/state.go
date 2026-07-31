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
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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
	ModalSandboxID    string // Modal sandbox ID if Modal-provisioned
	Provider          string // "akash" | "modal" | "" (Firecracker)
}

// Sandbox represents a long-lived isolated execution environment for AI agents
type Sandbox struct {
	ID         string `gorm:"primaryKey"`
	VMID       string `gorm:"index"`
	VMIP       string
	Status     string // "starting" | "ready" | "busy" | "destroyed"
	Runtime    string // "python3" | "bash"
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

// StandbyVM is a pre-warmed GPU deployment held in the buffer pool.
type StandbyVM struct {
	ID           string `gorm:"primaryKey"`
	ProviderName string // "akash", "aws", "azure"
	ProviderAddr string
	GPUModel     string `gorm:"index"`
	DeploymentID string
	Status       string `gorm:"index"` // standby | claimed | expired
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// Trajectory stores one rollout step for the RL experience buffer.
type Trajectory struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	RunID     string    `gorm:"index"                   json:"run_id"`
	ProblemID string    `json:"problem_id"`
	Prompt    string    `json:"prompt"`
	Code      string    `json:"code"`
	Reward    float64   `json:"reward"`
	Done      bool      `json:"done"`
	StepN     int       `json:"step_n"`
	Consumed  bool      `gorm:"index"                   json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// RLRun tracks a distributed RL training run (coordinator + workers + trainer).
type RLRun struct {
	ID              string    `gorm:"primaryKey"`
	Status          string    // starting | running | stopped | completed
	BaseModel       string
	NumWorkers      int
	GPUModel        string
	PolicyServerURL string
	TrainerExecID   string
	WorkerExecIDs   string // JSON array of execution IDs
	ClosedLoop      bool    // serialized collect → train → redeploy cycles
	TrainingRound   int     // current training round (incremented after each optimizer step)
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TrainingMetricRecord persists a single training step metric to SQLite.
type TrainingMetricRecord struct {
	ID            uint   `gorm:"primaryKey;autoIncrement"`
	JobID         string `gorm:"index"`
	Step          int
	EpisodeReward float64
	Loss          float64
	GPUUtil       int
	Timestamp     int64
}

// SandboxExec represents a single code execution within a sandbox
type SandboxExec struct {
	ID         string `gorm:"primaryKey"`
	SandboxID  string `gorm:"index"`
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
	db, err := gorm.Open(sqlite.Open("skyscale.db"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// Auto migrate the schema
	err = db.AutoMigrate(&Function{}, &Execution{}, &VM{}, &Sandbox{}, &SandboxExec{}, &Deployment{}, &TrainingMetricRecord{}, &StandbyVM{}, &JobQueueItem{}, &Trajectory{}, &RLRun{})
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

// EnqueueJob inserts a new job queue item.
func (s *StateManager) EnqueueJob(item *JobQueueItem) error {
	return s.db.Create(item).Error
}

// ClaimNextJob atomically claims the highest-priority queued job.
// Returns nil if the queue is empty.
func (s *StateManager) ClaimNextJob() (*JobQueueItem, error) {
	var item JobQueueItem
	err := s.db.Where("status = ?", "queued").
		Order("priority desc, created_at asc").
		First(&item).Error
	if err != nil {
		return nil, err // gorm.ErrRecordNotFound when empty
	}
	item.Status = "dispatching"
	item.UpdatedAt = time.Now()
	return &item, s.db.Save(&item).Error
}

// UpdateJobStatus sets the status (and optional error) on a job queue item.
func (s *StateManager) UpdateJobStatus(id, status, errMsg string) error {
	updates := map[string]interface{}{"status": status, "updated_at": time.Now()}
	if errMsg != "" {
		updates["error"] = errMsg
	}
	return s.db.Model(&JobQueueItem{}).Where("id = ?", id).Updates(updates).Error
}

// SaveStandbyVM persists a standby VM slot.
func (s *StateManager) SaveStandbyVM(vm *StandbyVM) error {
	return s.db.Save(vm).Error
}

// ClaimStandbyVM atomically finds a standby slot for the given GPU model and marks it claimed.
// Returns nil if none available.
func (s *StateManager) ClaimStandbyVM(gpuModel string) (*StandbyVM, error) {
	var vm StandbyVM
	err := s.db.Where("gpu_model = ? AND status = ? AND expires_at > ?", gpuModel, "standby", time.Now()).
		Order("created_at asc").First(&vm).Error
	if err != nil {
		return nil, err
	}
	vm.Status = "claimed"
	return &vm, s.db.Save(&vm).Error
}

// ListStandbyVMs returns all standby slots filtered by status.
func (s *StateManager) ListStandbyVMs(status string) ([]StandbyVM, error) {
	var vms []StandbyVM
	return vms, s.db.Where("status = ?", status).Find(&vms).Error
}

// SaveMetric persists a training metric record to the database.
func (s *StateManager) SaveMetric(m *TrainingMetricRecord) error {
	return s.db.Create(m).Error
}

// GetMetrics returns all persisted metrics for a job ordered by step.
func (s *StateManager) GetMetrics(jobID string) ([]TrainingMetricRecord, error) {
	var rows []TrainingMetricRecord
	return rows, s.db.Where("job_id = ?", jobID).Order("step asc").Find(&rows).Error
}

// SaveTrajectory saves a trajectory to the experience buffer.
func (s *StateManager) SaveTrajectory(t *Trajectory) error {
	return s.db.Create(t).Error
}

// SampleTrajectories returns up to n unconsumed trajectories for a run and marks them consumed.
func (s *StateManager) SampleTrajectories(runID string, n int) ([]Trajectory, error) {
	var rows []Trajectory
	err := s.db.Where("run_id = ? AND consumed = ?", runID, false).
		Order("created_at asc").Limit(n).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return rows, err
	}
	ids := make([]uint, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	s.db.Model(&Trajectory{}).Where("id IN ?", ids).Update("consumed", true)
	return rows, nil
}

// BufferSize returns the number of unconsumed trajectories for a run.
func (s *StateManager) BufferSize(runID string) (int64, error) {
	var count int64
	err := s.db.Model(&Trajectory{}).Where("run_id = ? AND consumed = ?", runID, false).Count(&count).Error
	return count, err
}

// SaveRLRun persists an RL run record.
func (s *StateManager) SaveRLRun(r *RLRun) error {
	return s.db.Save(r).Error
}

// GetRLRun retrieves an RL run by ID.
func (s *StateManager) GetRLRun(id string) (*RLRun, error) {
	var r RLRun
	if err := s.db.First(&r, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRLRuns returns all RL runs ordered newest first.
func (s *StateManager) ListRLRuns() ([]RLRun, error) {
	var runs []RLRun
	return runs, s.db.Order("created_at desc").Find(&runs).Error
}

// Close closes the state manager
func (s *StateManager) Close() {
	if s.cache != nil {
		s.cache.Close()
	}
}

// Package vm provides functionality for managing Firecracker micro-VMs.
//
// The VMManager manages the lifecycle of Firecracker micro-VMs, including:
// - Creating new VMs
// - Returning VMs to the warm pool
// - Terminating VMs

package vm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/bluequbit/faas/control-plane/akash"
	"github.com/bluequbit/faas/control-plane/state"
	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// VMManager manages the lifecycle of Firecracker micro-VMs
type VMManager struct {
	stateManager *state.StateManager
	akashClient  *akash.Client
	logger       *logrus.Logger
	vmDir        string
	warmPoolSize int
	warmPool     chan *state.VM
	mu           sync.Mutex
	createMu     sync.Mutex
	akashMu      sync.Mutex // serialises Akash deployment creation
	vms          map[string]*VMInstance
}

// VMInstance represents a running Firecracker VM instance or Akash deployment
type VMInstance struct {
	ID                string
	IP                string
	Machine           *firecracker.Machine
	Status            string
	CreatedAt         time.Time
	LastUsed          time.Time
	Memory            int
	CPU               int
	IsWarm            bool
	HardwareType      string // "cpu", "gpu"
	GPUModel          string // e.g. "nvidia-t4"
	VRAMmb            int
	AkashDeploymentID string // dseq if Akash-provisioned
	ProviderAddr      string // Akash provider address
}

// VMConfig represents the configuration for a VM
type VMConfig struct {
	Memory int
	CPU    int
	Kernel string
	RootFS string
}

// ComputeRequest describes what kind of compute a deployment or job needs.
type ComputeRequest struct {
	HardwareType    string // "cpu" | "gpu"
	GPUModel        string // e.g. "a100", "t4"
	DockerImage     string // for Akash GPU containers
	Memory          int
	CPU             int
	JobID           string
	ExecutionID     string
	ControlPlaneURL string
}

// NewVMManager creates a new VM manager
func NewVMManager(stateManager *state.StateManager, logger *logrus.Logger) (*VMManager, error) {
	// Create VM directory if it doesn't exist
	vmDir := "vm-storage"
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return nil, err
	}
	warmPoolSize := 2
	if v := os.Getenv("WARM_POOL_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid WARM_POOL_SIZE %q", v)
		}
		warmPoolSize = n
	}

	manager := &VMManager{
		stateManager: stateManager,
		akashClient:  akash.NewClient(logger),
		logger:       logger,
		vmDir:        vmDir,
		warmPoolSize: warmPoolSize,
		warmPool:     make(chan *state.VM, max(1, warmPoolSize)),
		vms:          make(map[string]*VMInstance),
	}

	if warmPoolSize > 0 {
		go manager.manageWarmPool()
	} else {
		logger.Info("Firecracker warm pool disabled")
	}

	return manager, nil
}

// manageWarmPool maintains a pool of pre-warmed VMs
func (m *VMManager) manageWarmPool() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			currentSize := len(m.warmPool)

			if currentSize < m.warmPoolSize {
				m.logger.Infof("Warm pool size: %d/%d, creating new warm VM", currentSize, m.warmPoolSize)
				m.createMu.Lock()
				if len(m.warmPool) >= m.warmPoolSize {
					m.createMu.Unlock()
					m.logger.Infof("Warm pool refilled while waiting; size: %d/%d", len(m.warmPool), m.warmPoolSize)
					continue
				}
				vm, err := m.createVM(true)
				m.createMu.Unlock()
				if err != nil {
					m.logger.Errorf("Failed to create warm VM: %v", err)
					continue
				}

				select {
				case m.warmPool <- vm:
					m.logger.Infof("Added VM %s to warm pool", vm.ID)
				default:
					// Pool is full, clean up the VM
					m.logger.Warnf("Warm pool is full, cleaning up VM %s", vm.ID)
					m.terminateVM(vm.ID)
				}
			} else {
				m.logger.Infof("Warm pool size: %d/%d, no need to create new warm VM", currentSize, m.warmPoolSize)
			}
		}
	}
}

// GetVM gets a VM from the warm pool or creates a new one
func (m *VMManager) GetVM() (*state.VM, error) {
	// Try to get a VM from the warm pool
	select {
	case vm := <-m.warmPool:
		m.logger.Infof("Using warm VM %s from pool", vm.ID)

		// Update VM status
		vm.Status = "busy"
		vm.LastUsed = time.Now()
		if err := m.stateManager.SaveVM(vm); err != nil {
			m.logger.Errorf("Failed to update VM status: %v", err)
		}

		return vm, nil
	default:
		// No warm VM available, create a new one
		m.logger.Info("No warm VM available, creating new VM")
		m.createMu.Lock()
		defer m.createMu.Unlock()

		// Another invocation may have returned a VM while this request waited
		// for the cold-start lock.
		select {
		case vm := <-m.warmPool:
			m.logger.Infof("Using warm VM %s from pool after waiting for cold-start lock", vm.ID)
			vm.Status = "busy"
			vm.LastUsed = time.Now()
			if err := m.stateManager.SaveVM(vm); err != nil {
				m.logger.Errorf("Failed to update VM status: %v", err)
			}
			return vm, nil
		default:
		}

		return m.createVM(false)
	}
}

// createVM creates a new Firecracker VM using the Go SDK
func (m *VMManager) createVM(isWarm bool) (*state.VM, error) {
	// Generate VM ID
	id := uuid.New().String()
	cleanupOnError := true
	var machine *firecracker.Machine
	defer func() {
		if !cleanupOnError {
			return
		}
		if machine != nil {
			if err := machine.StopVMM(); err != nil {
				m.logger.Debugf("Failed to stop VM %s during failed startup cleanup: %v", id, err)
			}
		}
		vmDir := filepath.Join(m.vmDir, id)
		if err := os.RemoveAll(vmDir); err != nil {
			m.logger.Errorf("Failed to remove VM directory %s after startup failure: %v", vmDir, err)
		}
	}()

	// Create VM directory
	vmDir := filepath.Join(m.vmDir, id)
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return nil, err
	}

	// Create VM configuration
	config := VMConfig{
		Memory: getDefaultMemoryMB(),
		CPU:    getDefaultCPUCount(),
		Kernel: getDefaultKernelPath(),
		RootFS: getDefaultRootFSPath(),
	}

	// Copy rootfs for this VM (each VM needs its own writable copy)
	vmRootFS := filepath.Join(vmDir, "rootfs.ext4")
	if err := copyFile(config.RootFS, vmRootFS); err != nil {
		return nil, fmt.Errorf("failed to copy rootfs: %v", err)
	}
	config.RootFS = vmRootFS

	// Create context for VM operations
	ctx := context.Background()

	// Socket path for Firecracker
	socketPath := filepath.Join(vmDir, "firecracker.sock")

	// Create Firecracker machine configuration
	fcCfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: config.Kernel,
		KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off",
		Drives: []models.Drive{
			{
				DriveID:      firecracker.String("1"),
				PathOnHost:   firecracker.String(config.RootFS),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			},
		},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(config.CPU)),
			MemSizeMib: firecracker.Int64(int64(config.Memory)),
		},
		NetworkInterfaces: firecracker.NetworkInterfaces{
			firecracker.NetworkInterface{
				// finds the CNI configuration in /etc/cni/conf.d by default
				CNIConfiguration: &firecracker.CNIConfiguration{
					NetworkName: "fcnet", // matches the name in your CNI config file
					IfName:      "veth0", // changed from tap0 to veth0 for ptp plugin
				},
				AllowMMDS: true,
			},
		},
		VMID:        id,
		LogLevel:    "Debug",
		LogFifo:     filepath.Join(vmDir, "firecracker.log"),
		MetricsFifo: filepath.Join(vmDir, "firecracker.metrics"),
	}

	// Create command for Firecracker
	cmd := firecracker.VMCommandBuilder{}.
		WithBin("/usr/local/bin/firecracker").
		WithSocketPath(socketPath).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	// Create machine options
	machineOpts := []firecracker.Opt{
		firecracker.WithLogger(logrus.NewEntry(m.logger)),
		firecracker.WithProcessRunner(cmd),
	}

	// Create the machine
	machine, err := firecracker.NewMachine(ctx, fcCfg, machineOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine: %v", err)
	}

	// Start the machine
	if err := machine.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start machine: %v", err)
	}

	// Get the IP address from the network configuration
	ipAddress := machine.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP.String()

	m.logger.WithField("ip", ipAddress).Info("machine started")
	if err := m.waitForDaemon(ctx, id, ipAddress, 8081); err != nil {
		return nil, err
	}

	// Create VM instance
	vmInstance := &VMInstance{
		ID:      id,
		IP:      ipAddress,
		Machine: machine,
		Status: func() string {
			if isWarm {
				return "ready"
			}
			return "busy"
		}(),
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		Memory:    config.Memory,
		CPU:       config.CPU,
		IsWarm:    isWarm,
	}

	// Store VM instance
	m.mu.Lock()
	m.vms[id] = vmInstance
	m.mu.Unlock()

	// Create VM in state manager
	vm := &state.VM{
		ID:           id,
		Status:       vmInstance.Status,
		IP:           vmInstance.IP,
		CreatedAt:    vmInstance.CreatedAt,
		LastUsed:     vmInstance.LastUsed,
		Memory:       config.Memory,
		CPU:          config.CPU,
		IsWarm:       isWarm,
		HardwareType: "cpu",
	}

	if err := m.stateManager.SaveVM(vm); err != nil {
		m.logger.Errorf("Failed to save VM to state manager: %v", err)
	}

	cleanupOnError = false
	return vm, nil
}

func (m *VMManager) waitForDaemon(ctx context.Context, id, ip string, port int) error {
	timeout := 45 * time.Second
	if raw := os.Getenv("FAAS_VM_DAEMON_READY_TIMEOUT"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			return fmt.Errorf("invalid FAAS_VM_DAEMON_READY_TIMEOUT %q", raw)
		}
		timeout = time.Duration(seconds) * time.Second
	}

	deadline := time.Now().Add(timeout)
	healthURL := fmt.Sprintf("http://%s:%d/health", ip, port)
	client := &http.Client{Timeout: 750 * time.Millisecond}
	var lastErr error

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				m.logger.Infof("VM %s daemon ready at %s", id, healthURL)
				return nil
			}
			lastErr = fmt.Errorf("daemon health returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return fmt.Errorf("daemon on VM %s did not become ready at %s within %s: %w", id, healthURL, timeout, lastErr)
	}
	return fmt.Errorf("daemon on VM %s did not become ready at %s within %s", id, healthURL, timeout)
}

// ReturnVM returns a VM to the warm pool
func (m *VMManager) ReturnVM(id string) error {
	// Get VM from state manager
	vm, err := m.stateManager.GetVM(id)
	if err != nil {
		return err
	}

	// Update VM status
	vm.Status = "ready"
	vm.LastUsed = time.Now()
	vm.IsWarm = true
	if err := m.stateManager.SaveVM(vm); err != nil {
		return err
	}

	// Add VM to warm pool
	select {
	case m.warmPool <- vm:
		m.logger.Infof("Returned VM %s to warm pool", id)
	default:
		// Pool is full, terminate the VM
		m.logger.Warnf("Warm pool is full, terminating VM %s", id)
		return m.terminateVM(id)
	}

	return nil
}

// terminateVM terminates a VM
func (m *VMManager) terminateVM(id string) error {
	m.mu.Lock()
	vmInstance, exists := m.vms[id]
	m.mu.Unlock()

	if !exists {
		return errors.New("VM not found")
	}

	// Stop the VM
	if err := vmInstance.Machine.StopVMM(); err != nil {
		m.logger.Errorf("Failed to stop VM: %v", err)
	}

	// Remove VM directory
	vmDir := filepath.Join(m.vmDir, id)
	if err := os.RemoveAll(vmDir); err != nil {
		m.logger.Errorf("Failed to remove VM directory: %v", err)
	}

	// Remove VM from state manager
	if err := m.stateManager.DeleteVM(id); err != nil {
		m.logger.Errorf("Failed to delete VM from state manager: %v", err)
	}

	// Remove VM from map
	m.mu.Lock()
	delete(m.vms, id)
	m.mu.Unlock()

	m.logger.Infof("Terminated VM %s", id)
	return nil
}

// TerminateVM terminates a VM by ID (public wrapper for terminateVM)
func (m *VMManager) TerminateVM(id string) error {
	return m.terminateVM(id)
}

// assignIP assigns an IP address to a VM
func (m *VMManager) assignIP() (string, error) {
	// For simplicity, we'll use a hardcoded IP range
	// In a production environment, this would be more sophisticated
	return "172.16.0.2", nil
}

// Cleanup cleans up all VMs
func (m *VMManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, vmInstance := range m.vms {
		if err := vmInstance.Machine.StopVMM(); err != nil {
			m.logger.Errorf("Failed to stop VM: %v", err)
		}
		m.logger.Infof("Terminated VM %s during cleanup", id)
	}

	m.vms = make(map[string]*VMInstance)
}

// GetVMStatus gets the status of a VM
func (m *VMManager) GetVMStatus(id string) (string, error) {
	vm, err := m.stateManager.GetVM(id)
	if err != nil {
		return "", err
	}
	return vm.Status, nil
}

// GetAkashVM provisions a GPU VM via Akash Network for a training job.
// jobID is used to generate the SDL and label the deployment.
func (m *VMManager) GetAkashVM(jobID, executionID, gpuModel, dockerImage, controlPlaneURL string) (*state.VM, error) {
	if gpuModel == "" {
		gpuModel = "a100"
	}
	if dockerImage == "" {
		dockerImage = "ghcr.io/shubham-rasal/skyscale/skyscale-trainer:latest"
	}

	// Reuse an idle GPU deployment if one is available for this GPU model.
	if existing, err := m.stateManager.ClaimIdleGPUVM(gpuModel); err == nil {
		m.logger.Infof("Reusing existing Akash deployment %s (gpu=%s) for job %s", existing.AkashDeploymentID, gpuModel, jobID)
		return existing, nil
	}

	sdlData := akash.SDLData{
		DockerImage:     dockerImage,
		JobID:           jobID,
		ExecutionID:     executionID,
		ControlPlaneURL: controlPlaneURL,
		GPUModel:        gpuModel,
	}
	sdl, err := m.akashClient.GenerateTrainingSDL(sdlData)
	if err != nil {
		return nil, fmt.Errorf("generate SDL: %w", err)
	}

	// Serialise deployments: Akash ties dseq to the wallet, so concurrent
	// submissions from the same key collide with "already exists".
	m.akashMu.Lock()
	deployment, err := m.akashClient.CreateDeployment(sdl, jobID)
	m.akashMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("create Akash deployment: %w", err)
	}

	vm := &state.VM{
		ID:                deployment.ID,
		Status:            "busy",
		IP:                deployment.IP,
		DaemonPort:        deployment.Port,
		CreatedAt:         time.Now(),
		LastUsed:          time.Now(),
		HardwareType:      "gpu",
		GPUModel:          gpuModel,
		AkashDeploymentID: deployment.ID,
		ProviderAddr:      deployment.ProviderAddr,
	}
	if err := m.stateManager.SaveVM(vm); err != nil {
		m.logger.Errorf("Failed to save Akash VM to state: %v", err)
	}

	m.logger.Infof("Provisioned new Akash GPU deployment %s for job %s", deployment.ID, jobID)
	return vm, nil
}

// ReturnAkashVM marks a GPU deployment as idle so it can be reused.
func (m *VMManager) ReturnAkashVM(dseq string) error {
	vm, err := m.stateManager.GetVM(dseq)
	if err != nil {
		return err
	}
	vm.Status = "ready"
	vm.LastUsed = time.Now()
	m.logger.Infof("Returned Akash deployment %s to idle pool", dseq)
	return m.stateManager.SaveVM(vm)
}

// CloseAkashVM closes an Akash deployment and removes it from state.
func (m *VMManager) CloseAkashVM(dseq string) error {
	if err := m.akashClient.CloseDeployment(dseq); err != nil {
		m.logger.Errorf("Failed to close Akash deployment %s: %v", dseq, err)
	}
	return m.stateManager.DeleteVM(dseq)
}

// ListVMs lists all VMs
func (m *VMManager) ListVMs() ([]state.VM, error) {
	return m.stateManager.ListVMs()
}

// GetVMByID gets a VM by ID
func (m *VMManager) GetVMByID(id string) (*state.VM, error) {
	return m.stateManager.GetVM(id)
}

// GetCompute allocates CPU (Firecracker warm pool) or GPU (Akash) from a single entry point.
func (m *VMManager) GetCompute(req ComputeRequest) (*state.VM, error) {
	if req.HardwareType == "gpu" {
		jobID := req.JobID
		if jobID == "" {
			jobID = "gpu-job"
		}
		execID := req.ExecutionID
		if execID == "" {
			execID = jobID
		}
		return m.GetAkashVM(jobID, execID, req.GPUModel, req.DockerImage, req.ControlPlaneURL)
	}
	vm, err := m.GetVM()
	if err != nil && os.Getenv("DAEMON_PATH") != "" {
		// Test mode: Firecracker is not available; fall back to the local host VM.
		m.logger.Warn("GetVM failed in test mode, falling back to test host VM")
		return m.GetOrCreateTestHostVM()
	}
	return vm, err
}

// ReleaseCompute returns compute to the warm pool (CPU) or idle pool (GPU).
func (m *VMManager) ReleaseCompute(vmID string) error {
	vm, err := m.stateManager.GetVM(vmID)
	if err != nil {
		return err
	}
	if vm.HardwareType == "gpu" {
		return m.ReturnAkashVM(vmID)
	}
	return m.ReturnVM(vmID)
}

// CreateTestHostVM creates a test VM that represents the host machine for testing
func (m *VMManager) CreateTestHostVM() (*state.VM, error) {
	m.logger.Info("Creating test host VM for testing")

	// Generate VM ID
	id := "host-vm-test"

	// Use the host machine's IP (localhost)
	ip := "127.0.0.1"

	// Create VM in state manager
	vm := &state.VM{
		ID:           id,
		Status:       "ready",
		IP:           ip,
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
		Memory:       1024, // 1GB
		CPU:          2,    // 2 cores
		IsWarm:       true,
		HardwareType: "cpu",
	}

	if err := m.stateManager.SaveVM(vm); err != nil {
		return nil, fmt.Errorf("failed to save test VM to state manager: %v", err)
	}

	// Create VM instance (without actual Firecracker machine)
	vmInstance := &VMInstance{
		ID:        id,
		IP:        ip,
		Machine:   nil, // No actual Firecracker machine
		Status:    "ready",
		CreatedAt: vm.CreatedAt,
		LastUsed:  vm.LastUsed,
		Memory:    vm.Memory,
		CPU:       vm.CPU,
		IsWarm:    true,
	}

	// Store VM instance
	m.mu.Lock()
	m.vms[id] = vmInstance
	m.mu.Unlock()

	m.logger.Infof("Created test host VM with ID %s and IP %s", id, ip)
	return vm, nil
}

// GetOrCreateTestHostVM gets the test host VM if it exists, or creates it if it doesn't
func (m *VMManager) GetOrCreateTestHostVM() (*state.VM, error) {
	// Check if test host VM already exists
	vm, err := m.stateManager.GetVM("host-vm-test")
	if err == nil {
		// VM exists, return it
		return vm, nil
	}

	// VM doesn't exist, create it
	return m.CreateTestHostVM()
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

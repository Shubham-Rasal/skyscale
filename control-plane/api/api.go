package api

import (
	"encoding/json"
	"net/http"
	"time"
	_ "net/http/pprof"
	"github.com/bluequbit/faas/control-plane/akash"
	"github.com/bluequbit/faas/control-plane/auth"
	"github.com/bluequbit/faas/control-plane/registry"
	"github.com/bluequbit/faas/control-plane/scheduler"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/bluequbit/faas/control-plane/vm"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// APIHandler handles API requests
type APIHandler struct {
	functionRegistry *registry.FunctionRegistry
	vmManager        *vm.VMManager
	scheduler        *scheduler.Scheduler
	authManager      *auth.AuthManager
	stateManager     *state.StateManager
	trainingMetrics  *TrainingMetricsStore
	akashClient      *akash.Client
	logger           *logrus.Logger
}

// FunctionRequest represents a request to register a function
type FunctionRequest struct {
	Name         string `json:"name"`
	Runtime      string `json:"runtime"`
	Memory       int    `json:"memory"`
	Timeout      int    `json:"timeout"`
	Code         string `json:"code"`
	Requirements string `json:"requirements"`
	Config       string `json:"config"`
}

// InvokeRequest represents a request to invoke a function
type InvokeRequest struct {
	Input           map[string]any `json:"input"`
	Sync            bool           `json:"sync"`
	JobType         string         `json:"job_type"`      // "faas_function", "training_run", "rl_env"
	HardwareType    string         `json:"hardware_type"` // "cpu", "gpu"
	GPUModel        string         `json:"gpu_model"`     // e.g. "nvidia-t4"
	DockerImage     string         `json:"docker_image"`  // for GPU training jobs
	ControlPlaneURL string         `json:"control_plane_url"`
}

// APIKeyRequest represents a request to generate an API key
type APIKeyRequest struct {
	UserID    string   `json:"user_id"`
	Roles     []string `json:"roles"`
	ExpiresIn int64    `json:"expires_in"` // in seconds
}

// VMInfo represents information about a VM
type VMInfo struct {
	VMID        string `json:"vm_id"`
	IPAddress   string `json:"ip_address"`
	MachineName string `json:"machine_name"`
	Status      string `json:"status"`
}

// ExecutionResult represents the result of a function execution
type ExecutionResult struct {
	RequestID    string `json:"request_id"`
	FunctionID   string `json:"function_id"`
	StatusCode   int    `json:"status_code"`
	Output       string `json:"output"`
	ErrorMessage string `json:"error_message,omitempty"`
	Duration     int64  `json:"duration_ms"`
	MemoryUsage  int64  `json:"memory_usage_kb,omitempty"`
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(functionRegistry *registry.FunctionRegistry, vmManager *vm.VMManager, sched *scheduler.Scheduler, authManager *auth.AuthManager, stateManager *state.StateManager, logger *logrus.Logger) *APIHandler {
	return &APIHandler{
		functionRegistry: functionRegistry,
		vmManager:        vmManager,
		scheduler:        sched,
		authManager:      authManager,
		stateManager:     stateManager,
		trainingMetrics:  NewTrainingMetricsStore(),
		akashClient:      akash.NewClient(logger),
		logger:           logger,
	}
}

// RegisterRoutes registers API routes
func (h *APIHandler) RegisterRoutes(router *mux.Router) {
	// API routes
	api := router.PathPrefix("/api").Subrouter()

	// Public routes
	api.HandleFunc("/health", h.healthHandler).Methods("GET")

	// Auth routes
	auth := api.PathPrefix("/auth").Subrouter()
	auth.HandleFunc("/api-key", h.generateAPIKeyHandler).Methods("POST")

	// Protected routes
	protected := api.PathPrefix("").Subrouter()
	protected.Use(h.authManager.Middleware)

	// Function routes
	functions := api.PathPrefix("/functions").Subrouter()
	functions.HandleFunc("", h.listFunctionsHandler).Methods("GET")
	functions.HandleFunc("", h.registerFunctionHandler).Methods("POST")
	functions.HandleFunc("/{id}", h.getFunctionHandler).Methods("GET")
	functions.HandleFunc("/{id}", h.updateFunctionHandler).Methods("PUT")
	functions.HandleFunc("/{id}", h.deleteFunctionHandler).Methods("DELETE")
	functions.HandleFunc("/{id}/invoke", h.invokeFunctionHandler).Methods("POST")
	functions.HandleFunc("/name/{name}", h.getFunctionByNameHandler).Methods("GET")
	functions.HandleFunc("/name/{name}/invoke", h.invokeFunctionByNameHandler).Methods("POST")
	// functions.HandleFunc("/test/invoke", h.invokeTestFunctionHandler).Methods("POST")

	// Execution routes
	executions := api.PathPrefix("/executions").Subrouter()
	executions.HandleFunc("/{id}", h.getExecutionHandler).Methods("GET")
	executions.HandleFunc("/{id}/complete", h.completeExecutionHandler).Methods("POST")
	executions.HandleFunc("/function/{id}", h.listExecutionsHandler).Methods("GET")

	// Training metrics routes (no auth — called by containers on Akash)
	api.HandleFunc("/training/metrics", h.TrainingMetricsHandler).Methods("POST")
	api.HandleFunc("/training/metrics/{job_id}", h.trainingMetricsGetHandler).Methods("GET")

	// Submit a training job directly to Akash from the dashboard
	api.HandleFunc("/training/jobs", h.submitTrainingJobHandler).Methods("POST")

	// Training job seeding (used by akash-test and scheduler to register jobs)
	api.HandleFunc("/training/executions", h.seedExecutionHandler).Methods("POST")
	api.HandleFunc("/training/vms", h.seedVMHandler).Methods("POST")

	// GPU metrics route
	api.HandleFunc("/metrics/gpu", h.GPUMetricsHandler).Methods("GET")

	// VM routes
	vms := api.PathPrefix("/vms").Subrouter()
	vms.HandleFunc("", h.listVMsHandler).Methods("GET")
	vms.HandleFunc("/{id}", h.getVMHandler).Methods("GET")
	vms.HandleFunc("/register", h.registerVMHandler).Methods("POST")

	// Result routes - no auth required for VM to report results
	api.HandleFunc("/results", h.handleResultHandler).Methods("POST")
}

// RegisterSandboxAPIRoutes wires the sandbox API handler into the provided router
func RegisterSandboxAPIRoutes(router *mux.Router, sandboxHandler *SandboxAPIHandler) {
	sandboxHandler.RegisterSandboxRoutes(router)
}

// healthHandler handles health check requests
func (h *APIHandler) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// generateAPIKeyHandler handles API key generation requests
func (h *APIHandler) generateAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	var req APIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate API key
	key, err := h.authManager.GenerateAPIKey(req.UserID, req.Roles, time.Duration(req.ExpiresIn)*time.Second)
	if err != nil {
		http.Error(w, "Failed to generate API key", http.StatusInternalServerError)
		return
	}

	// Return API key
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"api_key": key,
	})
}

// registerFunctionHandler handles function registration requests
func (h *APIHandler) registerFunctionHandler(w http.ResponseWriter, r *http.Request) {
	var req FunctionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Register function
	function, err := h.functionRegistry.RegisterFunction(req.Name, req.Runtime, req.Memory, req.Timeout, req.Code, req.Requirements, req.Config)
	if err != nil {
		http.Error(w, "Failed to register function: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return function metadata
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(function)
}

// updateFunctionHandler handles function update requests
func (h *APIHandler) updateFunctionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req FunctionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update function
	function, err := h.functionRegistry.UpdateFunction(id, req.Code, req.Requirements, req.Config)
	if err != nil {
		http.Error(w, "Failed to update function: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return function metadata
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(function)
}

// getFunctionHandler handles function retrieval requests
func (h *APIHandler) getFunctionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get function
	function, err := h.functionRegistry.GetFunction(id)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Return function metadata
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(function)
}

// getFunctionByNameHandler handles function retrieval by name requests
func (h *APIHandler) getFunctionByNameHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	// Get function
	function, err := h.functionRegistry.GetFunctionByName(name)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Return function metadata
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(function)
}

// listFunctionsHandler handles function listing requests
func (h *APIHandler) listFunctionsHandler(w http.ResponseWriter, r *http.Request) {
	// List functions
	functions, err := h.functionRegistry.ListFunctions()
	if err != nil {
		http.Error(w, "Failed to list functions", http.StatusInternalServerError)
		return
	}

	// Return function list
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(functions)
}

// deleteFunctionHandler handles function deletion requests
func (h *APIHandler) deleteFunctionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Delete function
	err := h.functionRegistry.DeleteFunction(id)
	if err != nil {
		http.Error(w, "Failed to delete function: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Function deleted"))
}

// invokeFunctionHandler handles function invocation requests
func (h *APIHandler) invokeFunctionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Invoke function
	response, err := h.scheduler.ScheduleExecution(id, req.Input, req.Sync, scheduler.JobOptions{
		JobType:         req.JobType,
		HardwareType:    req.HardwareType,
		GPUModel:        req.GPUModel,
		DockerImage:     req.DockerImage,
		ControlPlaneURL: req.ControlPlaneURL,
	})
	if err != nil {
		http.Error(w, "Failed to invoke function: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// invokeTestFunctionHandler handles function invocation requests for test mode

// invokeFunctionByNameHandler handles function invocation by name requests
func (h *APIHandler) invokeFunctionByNameHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	var req InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Invoke function
	response, err := h.scheduler.ScheduleExecutionByName(name, req.Input, req.Sync, scheduler.JobOptions{
		JobType:         req.JobType,
		HardwareType:    req.HardwareType,
		GPUModel:        req.GPUModel,
		DockerImage:     req.DockerImage,
		ControlPlaneURL: req.ControlPlaneURL,
	})
	if err != nil {
		http.Error(w, "Failed to invoke function: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getExecutionHandler handles execution retrieval requests
func (h *APIHandler) getExecutionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get execution
	execution, err := h.stateManager.GetExecution(id)
	if err != nil {
		http.Error(w, "Execution not found", http.StatusNotFound)
		return
	}

	// Return execution
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(execution)
}

// completeExecutionHandler is called by the training container when training finishes.
func (h *APIHandler) completeExecutionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var body struct {
		Status string `json:"status"`
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	execution, err := h.stateManager.GetExecution(id)
	if err != nil {
		http.Error(w, "Execution not found", http.StatusNotFound)
		return
	}

	status := body.Status
	if status == "" {
		status = "completed"
	}
	execution.Status = status
	execution.Logs = body.Output
	execution.Error = body.Error
	execution.EndTime = time.Now()
	execution.Duration = time.Since(execution.StartTime).Milliseconds()
	h.stateManager.SaveExecution(execution)

	h.logger.Infof("Execution %s completed via callback: status=%s", id, status)
	w.WriteHeader(http.StatusOK)
}

// listExecutionsHandler handles execution listing requests
func (h *APIHandler) listExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// List executions
	executions, err := h.stateManager.ListExecutions(id)
	if err != nil {
		http.Error(w, "Failed to list executions", http.StatusInternalServerError)
		return
	}

	// Return execution list
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(executions)
}

// listVMsHandler handles VM listing requests
func (h *APIHandler) listVMsHandler(w http.ResponseWriter, r *http.Request) {
	// List VMs
	vms, err := h.vmManager.ListVMs()
	if err != nil {
		http.Error(w, "Failed to list VMs", http.StatusInternalServerError)
		return
	}

	// Return VM list
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vms)
}

// getVMHandler handles VM retrieval requests
func (h *APIHandler) getVMHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get VM
	vm, err := h.vmManager.GetVMByID(id)
	if err != nil {
		http.Error(w, "VM not found", http.StatusNotFound)
		return
	}

	// Return VM
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vm)
}

// registerVMHandler handles VM registration requests
func (h *APIHandler) registerVMHandler(w http.ResponseWriter, r *http.Request) {
	var vmInfo VMInfo
	if err := json.NewDecoder(r.Body).Decode(&vmInfo); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	h.logger.Infof("Registering VM: %s (%s) at %s", vmInfo.VMID, vmInfo.MachineName, vmInfo.IPAddress)

	// Get VM from state manager
	vm, err := h.vmManager.GetVMByID(vmInfo.VMID)
	if err != nil {
		// VM not found, create a new one
		h.logger.Warnf("VM not found in state manager: %s", vmInfo.VMID)
		http.Error(w, "VM not found", http.StatusNotFound)
		return
	}

	// Update VM status
	vm.Status = vmInfo.Status
	vm.IP = vmInfo.IPAddress
	if err := h.stateManager.SaveVM(vm); err != nil {
		h.logger.Errorf("Failed to update VM status: %v", err)
		http.Error(w, "Failed to update VM status", http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("VM registered"))
}

// handleResultHandler handles function execution result reports from VMs
func (h *APIHandler) handleResultHandler(w http.ResponseWriter, r *http.Request) {
	var result ExecutionResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	h.logger.Infof("Received execution result for request ID: %s, function ID: %s", result.RequestID, result.FunctionID)

	// Get execution from state manager
	execution, err := h.stateManager.GetExecution(result.RequestID)
	if err != nil {
		h.logger.Warnf("Execution not found: %s", result.RequestID)
		http.Error(w, "Execution not found", http.StatusNotFound)
		return
	}

	// Update execution status
	execution.Status = "completed"
	execution.EndTime = time.Now()
	execution.Duration = result.Duration

	if result.StatusCode == 200 {
		// Store the output in the logs field since there's no Result field
		execution.Logs = result.Output
	} else {
		execution.Status = "error"
		execution.Error = result.ErrorMessage
	}

	// Save execution
	if err := h.stateManager.SaveExecution(execution); err != nil {
		h.logger.Errorf("Failed to save execution: %v", err)
		http.Error(w, "Failed to save execution", http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Result received"))
}

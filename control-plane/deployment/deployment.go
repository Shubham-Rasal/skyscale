package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/bluequbit/faas/control-plane/state"
	"github.com/bluequbit/faas/control-plane/vm"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Manager provisions long-lived web endpoints and proxies traffic to workloads.
type Manager struct {
	vmManager    *vm.VMManager
	stateManager *state.StateManager
	httpClient   *http.Client
	publicBase   string
	logger       *logrus.Logger
	nextPort     atomic.Int32 // allocates a unique daemon-side port per deployment
}

// NewManager constructs a deployment manager. publicBase is the control plane
// origin used in returned URLs (e.g. https://api.example.com or http://127.0.0.1:8080).
func NewManager(vmManager *vm.VMManager, stateManager *state.StateManager, publicBase string, logger *logrus.Logger) *Manager {
	if publicBase == "" {
		publicBase = "http://127.0.0.1:8080"
	}
	publicBase = strings.TrimRight(publicBase, "/")
	m := &Manager{
		vmManager:    vmManager,
		stateManager: stateManager,
		httpClient: &http.Client{
			Timeout: 45 * time.Minute,
		},
		publicBase: publicBase,
		logger:     logger,
	}
	// Seed the port counter from the highest vm_port already persisted so that
	// a control-plane restart never reuses a port that uvicorn is still holding.
	if existing, err := stateManager.ListDeployments(); err == nil {
		maxPort := int32(9000)
		for _, d := range existing {
			if int32(d.VMPort) > maxPort {
				maxPort = int32(d.VMPort)
			}
		}
		m.nextPort.Store(maxPort)
	} else {
		m.nextPort.Store(9000)
	}
	return m
}

// allocatePort returns the next available port, incrementing the counter.
func (m *Manager) allocatePort() int {
	// Add(1) first so two concurrent deploys never race to the same port.
	return int(m.nextPort.Add(1))
}

func slugify(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

// Deploy provisions compute, starts the daemon /serve process, and persists state.
func (m *Manager) Deploy(spec DeploymentSpec) (*state.Deployment, error) {
	slug := slugify(spec.AppName) + "--" + slugify(spec.EndpointName)
	existing, errSlug := m.stateManager.GetDeploymentBySlug(slug)
	if errSlug == nil && existing != nil {
		return nil, fmt.Errorf("deployment already exists for slug %q", slug)
	}
	if errSlug != nil && !errors.Is(errSlug, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup deployment: %w", errSlug)
	}

	hw := spec.Hardware.Type
	if hw == "" {
		hw = "cpu"
	}
	gpuModel := spec.Hardware.GPUModel

	cpURL := m.publicBase
	vmReq := vm.ComputeRequest{
		HardwareType:    hw,
		GPUModel:        gpuModel,
		DockerImage:     "",
		Memory:          spec.Hardware.Memory,
		JobID:           spec.AppName + "-" + spec.EndpointName,
		ExecutionID:     uuid.NewString(),
		ControlPlaneURL: cpURL,
	}

	computeVM, err := m.vmManager.GetCompute(vmReq)
	if err != nil {
		return nil, fmt.Errorf("provision compute: %w", err)
	}

	webPath := spec.Web.Path
	if webPath == "" {
		webPath = "/"
	}
	webMethod := spec.Web.Method
	if webMethod == "" {
		webMethod = "POST"
	}

	port := m.allocatePort()
	serveReq := ServeRequest{
		Code:         spec.Code,
		Requirements: spec.Requirements,
		SetupScript:  spec.SetupScript,
		EntryType:    spec.EntryType,
		EntryClass:   spec.EntryClass,
		EntryMethod:  spec.EntryMethod,
		EnterMethod:  spec.EnterMethod,
		Port:         port,
		WebMethod:    webMethod,
		WebPath:      webPath,
	}

	if err := m.startServer(computeVM, serveReq); err != nil {
		_ = m.vmManager.ReleaseCompute(computeVM.ID)
		return nil, fmt.Errorf("start server: %w", err)
	}

	publicURL := fmt.Sprintf("%s/proxy/%s%s", m.publicBase, slug, webPath)
	dep := &state.Deployment{
		ID:              uuid.NewString(),
		Slug:            slug,
		AppName:         spec.AppName,
		EndpointName:    spec.EndpointName,
		VMID:            computeVM.ID,
		VMPort:          port,
		WebPath:         webPath,
		WebMethod:       webMethod,
		URL:             publicURL,
		Status:          "ready",
		HardwareType:    hw,
		GPUModel:        gpuModel,
		ScaledownWindow: spec.ScaledownWindow,
		LastRequestAt:   time.Now(),
		CreatedAt:       time.Now(),
	}
	if err := m.stateManager.SaveDeployment(dep); err != nil {
		_ = m.vmManager.ReleaseCompute(computeVM.ID)
		return nil, fmt.Errorf("save deployment: %w", err)
	}
	return dep, nil
}

func (m *Manager) startServer(vm *state.VM, req ServeRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	daemonPort := vm.DaemonPort
	if daemonPort == 0 {
		daemonPort = 8081
	}
	daemonURL := fmt.Sprintf("http://%s:%d/serve", vm.IP, daemonPort)
	m.logger.Infof("POST /serve on workload at %s", daemonURL)

	resp, err := m.httpClient.Post(daemonURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		return fmt.Errorf("daemon /serve: status %d: %s", resp.StatusCode, buf.String())
	}
	var r struct {
		Port   int    `json:"port"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("decode daemon response: %w", err)
	}
	if r.Status != "ready" {
		return fmt.Errorf("daemon status %q", r.Status)
	}
	return nil
}

// ProxyRequest forwards /proxy/{slug}/... to the deployment VM.
func (m *Manager) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	prefix := "/proxy/"
	if !strings.HasPrefix(path, prefix) {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(path, prefix)
	slug, suffix := rest, ""
	if idx := strings.Index(rest, "/"); idx >= 0 {
		slug = rest[:idx]
		suffix = rest[idx:]
	}
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	dep, err := m.stateManager.GetDeploymentBySlug(slug)
	if err != nil || dep == nil {
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}

	if !strings.EqualFold(r.Method, dep.WebMethod) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	want := dep.WebPath
	if want == "" {
		want = "/"
	}
	if suffix == "" {
		suffix = "/"
	}
	if suffix != want {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	computeVM, err := m.stateManager.GetVM(dep.VMID)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}

	dep.LastRequestAt = time.Now()
	_ = m.stateManager.SaveDeployment(dep)

	target := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", computeVM.IP, dep.VMPort),
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = dep.WebPath
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.Host = target.Host
	}
	proxy.ServeHTTP(w, r)
}

// List returns all deployments (newest first).
func (m *Manager) List() ([]state.Deployment, error) {
	return m.stateManager.ListDeployments()
}

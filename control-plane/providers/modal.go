package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/sirupsen/logrus"
)

// ModalProvider dispatches GPU jobs to Modal sandboxes via the Python sidecar.
type ModalProvider struct {
	logger     *logrus.Logger
	pythonPath string
	scriptPath string
}

func NewModalProvider(logger *logrus.Logger) *ModalProvider {
	_, thisFile, _, _ := runtime.Caller(0)
	// dispatch.py lives at control-plane/modal/dispatch.py
	scriptPath := filepath.Join(filepath.Dir(thisFile), "..", "modal", "dispatch.py")

	python := "python3"
	if p := os.Getenv("MODAL_PYTHON"); p != "" {
		python = p
	}

	return &ModalProvider{
		logger:     logger,
		pythonPath: python,
		scriptPath: scriptPath,
	}
}

func (p *ModalProvider) Name() string { return "modal" }

func (p *ModalProvider) Deploy(ctx context.Context, spec DeploySpec) (DeployResult, error) {
	params := map[string]interface{}{
		"job_id":            spec.JobID,
		"execution_id":      spec.ExecutionID,
		"docker_image":      spec.DockerImage,
		"gpu_model":         spec.GPUModel,
		"env_vars":          spec.EnvVars,
		"control_plane_url": spec.ControlPlaneURL,
		"cmd":               spec.Cmd,
	}
	paramsJSON, _ := json.Marshal(params)

	cmd := exec.CommandContext(ctx, p.pythonPath, p.scriptPath, string(paramsJSON))
	cmd.Env = append(os.Environ())

	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return DeployResult{}, fmt.Errorf("modal dispatch: %w\n%s", err, stderr)
	}

	var result struct {
		SandboxID string `json:"sandbox_id"`
		Status    string `json:"status"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return DeployResult{}, fmt.Errorf("modal: parse output %q: %w", string(out), err)
	}
	if result.Error != "" {
		return DeployResult{}, fmt.Errorf("modal: %s", result.Error)
	}
	if result.SandboxID == "" {
		return DeployResult{}, fmt.Errorf("modal: no sandbox_id in output: %s", string(out))
	}

	p.logger.Infof("modal: sandbox %s started (job=%s gpu=%s)", result.SandboxID, spec.JobID, spec.GPUModel)
	return DeployResult{
		DeploymentID: result.SandboxID,
		ProviderName: "modal",
		ProviderAddr: "modal.com",
	}, nil
}

func (p *ModalProvider) Terminate(ctx context.Context, deploymentID string) error {
	script := fmt.Sprintf(`
import modal, asyncio
async def stop():
    sb = modal.Sandbox.from_id(%q)
    await sb.aio.terminate()
asyncio.run(stop())
`, deploymentID)

	cmd := exec.CommandContext(ctx, p.pythonPath, "-c", script)
	cmd.Env = append(os.Environ())
	if out, err := cmd.Output(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("modal terminate %s: %s", deploymentID, string(ee.Stderr))
		}
		_ = out
		return fmt.Errorf("modal terminate %s: %w", deploymentID, err)
	}
	return nil
}

func (p *ModalProvider) Available(_ context.Context, _ string) (int, error) {
	// Modal scales on demand — always report capacity available.
	return 999, nil
}

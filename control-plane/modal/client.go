// Package modal provides a client for dispatching GPU jobs to Modal.
// It shells out to dispatch.py (a Modal Python sidecar) to submit sandboxes.
package modal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

// JobParams describes a GPU job to run on Modal.
type JobParams struct {
	JobID       string            `json:"job_id"`
	ExecutionID string            `json:"execution_id"`
	DockerImage string            `json:"docker_image"`
	GPUModel    string            `json:"gpu_model"` // "a100", "h100", "a10g", "t4", "l4"
	EnvVars     map[string]string `json:"env_vars"`
	Timeout     int               `json:"timeout"` // seconds, 0 → 86400
}

// JobResult is returned by Submit after the sandbox starts.
type JobResult struct {
	SandboxID   string `json:"sandbox_id"`
	Status      string `json:"status"`
	JobID       string `json:"job_id"`
	ExecutionID string `json:"execution_id"`
	GPU         string `json:"gpu"`
}

// Client submits jobs to Modal via the Python sidecar.
type Client struct {
	logger     *logrus.Logger
	pythonPath string // path to python3 binary
	scriptPath string // path to dispatch.py
}

// NewClient creates a Modal client. scriptDir is the directory containing
// dispatch.py; if empty it defaults to the modal/ directory next to this file.
func NewClient(logger *logrus.Logger) *Client {
	_, thisFile, _, _ := runtime.Caller(0)
	scriptDir := filepath.Dir(thisFile)

	python := "python3"
	if p := os.Getenv("MODAL_PYTHON"); p != "" {
		python = p
	}

	return &Client{
		logger:     logger,
		pythonPath: python,
		scriptPath: filepath.Join(scriptDir, "dispatch.py"),
	}
}

// Submit launches a Modal sandbox for the given job and returns immediately.
// The sandbox runs asynchronously; the container is expected to POST back to
// CONTROL_PLANE_URL when it finishes.
func (c *Client) Submit(p JobParams) (*JobResult, error) {
	if p.Timeout == 0 {
		p.Timeout = 86400
	}
	if p.GPUModel == "" {
		p.GPUModel = "a100"
	}

	paramsJSON, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	c.logger.Infof("modal: submitting job %s (image=%s gpu=%s)", p.JobID, p.DockerImage, p.GPUModel)

	cmd := exec.Command(c.pythonPath, c.scriptPath, string(paramsJSON))

	// Forward Modal credentials from environment.
	cmd.Env = append(os.Environ())

	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return nil, fmt.Errorf("dispatch.py failed: %w\nstderr: %s", err, stderr)
	}

	var result JobResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse dispatch output %q: %w", string(out), err)
	}
	if result.SandboxID == "" {
		return nil, fmt.Errorf("dispatch returned no sandbox_id: %s", string(out))
	}

	c.logger.Infof("modal: sandbox %s started for job %s", result.SandboxID, p.JobID)
	return &result, nil
}

// StopSandbox terminates a running Modal sandbox by ID.
func (c *Client) StopSandbox(sandboxID string) error {
	script := fmt.Sprintf(`
import modal, asyncio
async def stop():
    sb = modal.Sandbox.from_id(%q)
    await sb.aio.terminate()
asyncio.run(stop())
`, sandboxID)

	cmd := exec.Command(c.pythonPath, "-c", script)
	cmd.Env = append(os.Environ())
	if out, err := cmd.Output(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("stop sandbox %s: %s", sandboxID, string(ee.Stderr))
		}
		_ = out
		return fmt.Errorf("stop sandbox %s: %w", sandboxID, err)
	}
	c.logger.Infof("modal: sandbox %s terminated", sandboxID)
	return nil
}

// WaitForSandbox polls until the sandbox exits or timeout elapses.
// Returns exit code and any error.
func (c *Client) WaitForSandbox(sandboxID string, timeout time.Duration) (int, error) {
	script := fmt.Sprintf(`
import modal, asyncio, json, sys
async def wait():
    sb = modal.Sandbox.from_id(%q)
    await sb.wait.aio()
    print(json.dumps({"returncode": sb.returncode}))
asyncio.run(wait())
`, sandboxID)

	cmd := exec.Command(c.pythonPath, "-c", script)
	cmd.Env = append(os.Environ())

	done := make(chan error, 1)
	var out []byte
	go func() {
		var err error
		out, err = cmd.Output()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return -1, fmt.Errorf("wait sandbox %s: %s", sandboxID, string(ee.Stderr))
			}
			return -1, err
		}
		var result struct {
			Returncode int `json:"returncode"`
		}
		if err := json.Unmarshal(out, &result); err != nil {
			return -1, fmt.Errorf("parse wait output: %w", err)
		}
		return result.Returncode, nil
	case <-time.After(timeout):
		cmd.Process.Kill()
		return -1, fmt.Errorf("wait sandbox %s: timed out after %s", sandboxID, timeout)
	}
}

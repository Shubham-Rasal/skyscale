package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/bluequbit/faas/control-plane/akash"
	"github.com/sirupsen/logrus"
)

const (
	jobID       = "mnist-test-002"
	executionID = "exec-mnist-002"
	dockerImage = "ghcr.io/shubham-rasal/skyscale-mnist:v1"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	client := akash.NewClient(logger)

	controlPlaneURL := os.Getenv("CONTROL_PLANE_URL")
	if controlPlaneURL == "" {
		controlPlaneURL = "http://n8n.maximalstudio.in:8080"
	}

	sdlData := akash.SDLData{
		DockerImage:     dockerImage,
		JobID:           jobID,
		ExecutionID:     executionID,
		ControlPlaneURL: controlPlaneURL,
		GPUModel:        "a100",
		EnvVars:         map[string]string{"EPOCHS": "3", "BATCH_SIZE": "512"},
	}

	sdl, err := client.GenerateTrainingSDL(sdlData)
	if err != nil {
		logger.Fatalf("Failed to generate SDL: %v", err)
	}

	fmt.Println("=== Generated SDL ===")
	fmt.Println(sdl)
	fmt.Println("====================")

	// Seed a placeholder execution record (no dseq yet)
	if err := seedExecution(controlPlaneURL, "", logger); err != nil {
		logger.Warnf("Could not seed execution record: %v (dashboard may not show job)", err)
	}

	logger.Info("Creating Akash deployment...")
	deployment, err := client.CreateDeployment(sdl, sdlData.JobID)
	if err != nil {
		logger.Fatalf("Failed to create deployment: %v", err)
	}

	fmt.Printf("\n=== Deployment Created ===\n")
	fmt.Printf("ID:       %s\n", deployment.ID)
	fmt.Printf("Provider: %s\n", deployment.ProviderAddr)
	fmt.Printf("Status:   %s\n", deployment.Status)

	// Update execution record with the real dseq so the UI checks go green
	if err := seedExecution(controlPlaneURL, deployment.ID, logger); err != nil {
		logger.Warnf("Could not update execution with dseq: %v", err)
	}

	// Register the Akash deployment as a VM so GPU metrics work
	if err := seedVM(controlPlaneURL, deployment, logger); err != nil {
		logger.Warnf("Could not seed VM record: %v", err)
	}

	// Poll execution status
	logger.Infof("Job submitted. Watching for completion at %s/api/executions/%s", controlPlaneURL, executionID)
	for i := 0; i < 60; i++ {
		time.Sleep(30 * time.Second)
		exec, err := getExecution(controlPlaneURL, executionID)
		if err != nil {
			logger.Warnf("poll: %v", err)
			continue
		}
		logger.Infof("execution status=%s", exec["status"])
		if exec["status"] == "completed" || exec["status"] == "error" {
			fmt.Printf("\n=== Training Result ===\n")
			fmt.Printf("Status: %s\n", exec["status"])
			fmt.Printf("Output: %s\n", exec["logs"])
			return
		}
	}
	logger.Warn("Timed out waiting for completion — check dashboard for results")
}

// seedExecution creates/updates an execution record. Pass dseq="" before deployment,
// then call again with the real dseq to set VMID so the dashboard checks go green.
func seedExecution(baseURL, dseq string, logger *logrus.Logger) error {
	body, _ := json.Marshal(map[string]any{
		"id":            executionID,
		"job_id":        jobID,
		"status":        "running",
		"job_type":      "training_run",
		"hardware_type": "gpu",
		"gpu_model":     "a100",
		"vm_id":         dseq,
	})
	resp, err := http.Post(baseURL+"/api/training/executions", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("POST /api/training/executions → %d", resp.StatusCode)
	}
	logger.Infof("Seeded execution record: %s", executionID)
	return nil
}

// seedVM registers the Akash deployment as a GPU VM in control plane state.
func seedVM(baseURL string, d *akash.Deployment, logger *logrus.Logger) error {
	body, _ := json.Marshal(map[string]any{
		"id":                   d.ID,
		"status":               "busy",
		"hardware_type":        "gpu",
		"gpu_model":            "a100",
		"provider":             d.ProviderAddr,
		"akash_deployment_id":  d.ID,
	})
	resp, err := http.Post(baseURL+"/api/training/vms", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("POST /api/training/vms → %d", resp.StatusCode)
	}
	logger.Infof("Seeded VM record: %s", d.ID)
	return nil
}

func getExecution(baseURL, id string) (map[string]any, error) {
	resp, err := http.Get(baseURL + "/api/executions/" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}

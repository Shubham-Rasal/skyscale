package main

import (
	"fmt"
	"os"

	"github.com/bluequbit/faas/control-plane/akash"
	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	client := akash.NewClient(logger)

	controlPlaneURL := os.Getenv("CONTROL_PLANE_URL")
	if controlPlaneURL == "" {
		controlPlaneURL = "http://localhost:8080"
	}

	sdlData := akash.SDLData{
		DockerImage:     "shubhamrasal007/skyscale-cartpole:v2",
		JobID:           "cartpole-test-001",
		ExecutionID:     "exec-cartpole-001",
		ControlPlaneURL: controlPlaneURL,
		GPUModel:        "rtx3090",
		EnvVars:         map[string]string{"TOTAL_STEPS": "300", "REPORT_EVERY": "100"},
	}

	sdl, err := client.GenerateTrainingSDL(sdlData)
	if err != nil {
		logger.Fatalf("Failed to generate SDL: %v", err)
	}

	fmt.Println("=== Generated SDL ===")
	fmt.Println(sdl)
	fmt.Println("====================")

	logger.Info("Creating test deployment with 1 GPU...")
	deployment, err := client.CreateDeployment(sdl, sdlData.JobID)
	if err != nil {
		logger.Fatalf("Failed to create deployment: %v", err)
	}

	fmt.Printf("\n=== Deployment Created ===\n")
	fmt.Printf("ID:       %s\n", deployment.ID)
	fmt.Printf("Provider: %s\n", deployment.ProviderAddr)
	fmt.Printf("Status:   %s\n", deployment.Status)
	fmt.Printf("IP:       %s\n", deployment.IP)
	fmt.Printf("Port:     %d\n", deployment.Port)

	// Fetch status
	logger.Infof("Fetching deployment status for dseq=%s", deployment.ID)
	status, err := client.GetDeploymentStatus(deployment.ID)
	if err != nil {
		logger.Errorf("Failed to get deployment status: %v", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Deployment Status ===\n")
	fmt.Printf("ID:     %s\n", status.ID)
	fmt.Printf("Status: %s\n", status.Status)
	fmt.Printf("Host:   %s\n", status.IP)
}

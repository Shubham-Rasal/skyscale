package providers

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by provider stubs that are not yet wired up.
var ErrNotImplemented = errors.New("provider not implemented")

// DeploySpec describes a GPU workload to launch.
type DeploySpec struct {
	DockerImage     string
	GPUModel        string
	ProviderName    string
	EnvVars         map[string]string
	ExecutionID     string
	JobID           string
	ControlPlaneURL string
}

// DeployResult holds the opaque identifiers returned after a successful deployment.
type DeployResult struct {
	DeploymentID string // numeric dseq for Akash, task ARN for AWS, etc.
	ProviderName string // "akash", "aws", "azure"
	ProviderAddr string // provider wallet / endpoint
}

// GPUProvider is the interface every compute backend must implement.
type GPUProvider interface {
	// Name returns the provider identifier (e.g. "akash").
	Name() string
	// Deploy launches a containerised workload and returns an opaque deployment ID.
	Deploy(ctx context.Context, spec DeploySpec) (DeployResult, error)
	// Terminate tears down a deployment by its ID.
	Terminate(ctx context.Context, deploymentID string) error
	// Available returns best-effort estimated free capacity for a GPU model.
	Available(ctx context.Context, gpuModel string) (int, error)
}

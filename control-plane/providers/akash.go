package providers

import (
	"context"

	"github.com/bluequbit/faas/control-plane/akash"
	"github.com/sirupsen/logrus"
)

// AkashProvider wraps akash.Client to implement GPUProvider.
type AkashProvider struct {
	client *akash.Client
	logger *logrus.Logger
}

func NewAkashProvider(logger *logrus.Logger) *AkashProvider {
	return &AkashProvider{client: akash.NewClient(logger), logger: logger}
}

func (p *AkashProvider) Name() string { return "akash" }

func (p *AkashProvider) Deploy(_ context.Context, spec DeploySpec) (DeployResult, error) {
	sdlData := akash.SDLData{
		DockerImage:     spec.DockerImage,
		JobID:           spec.JobID,
		ExecutionID:     spec.ExecutionID,
		ControlPlaneURL: spec.ControlPlaneURL,
		GPUModel:        spec.GPUModel,
		EnvVars:         spec.EnvVars,
	}
	sdl, err := p.client.GenerateTrainingSDL(sdlData)
	if err != nil {
		return DeployResult{}, err
	}
	deployment, err := p.client.CreateDeployment(sdl, spec.JobID)
	if err != nil {
		return DeployResult{}, err
	}
	return DeployResult{
		DeploymentID: deployment.ID,
		ProviderName: "akash",
		ProviderAddr: deployment.ProviderAddr,
	}, nil
}

func (p *AkashProvider) Terminate(_ context.Context, deploymentID string) error {
	return p.client.CloseDeployment(deploymentID)
}

func (p *AkashProvider) Available(_ context.Context, gpuModel string) (int, error) {
	providers, err := p.client.QueryProviders(gpuModel)
	if err != nil {
		return 0, err
	}
	return len(providers), nil
}

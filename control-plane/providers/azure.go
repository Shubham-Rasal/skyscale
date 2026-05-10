package providers

import "context"

// AzureProvider is a stub for future Azure GPU integration.
type AzureProvider struct{}

func NewAzureProvider() *AzureProvider { return &AzureProvider{} }

func (p *AzureProvider) Name() string { return "azure" }

func (p *AzureProvider) Deploy(_ context.Context, _ DeploySpec) (DeployResult, error) {
	return DeployResult{}, ErrNotImplemented
}

func (p *AzureProvider) Terminate(_ context.Context, _ string) error {
	return ErrNotImplemented
}

func (p *AzureProvider) Available(_ context.Context, _ string) (int, error) {
	return 0, ErrNotImplemented
}

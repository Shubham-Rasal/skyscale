package providers

import "context"

// AWSProvider is a stub for future AWS GPU integration.
type AWSProvider struct{}

func NewAWSProvider() *AWSProvider { return &AWSProvider{} }

func (p *AWSProvider) Name() string { return "aws" }

func (p *AWSProvider) Deploy(_ context.Context, _ DeploySpec) (DeployResult, error) {
	return DeployResult{}, ErrNotImplemented
}

func (p *AWSProvider) Terminate(_ context.Context, _ string) error {
	return ErrNotImplemented
}

func (p *AWSProvider) Available(_ context.Context, _ string) (int, error) {
	return 0, ErrNotImplemented
}

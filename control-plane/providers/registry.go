package providers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Registry tries providers in priority order, falling back on error.
type Registry struct {
	providers []GPUProvider
	logger    *logrus.Logger
}

// NewRegistry builds a Registry from the GPU_PROVIDER_ORDER env var (default: "modal,akash").
// Supported names: "modal", "akash", "huggingface"/"hf", "aws", "azure".
func NewRegistry(logger *logrus.Logger) *Registry {
	order := os.Getenv("GPU_PROVIDER_ORDER")
	if order == "" {
		order = "modal,akash"
	}
	names := strings.Split(order, ",")
	var list []GPUProvider
	for _, name := range names {
		provider, ok := newProviderByName(name, logger)
		if ok {
			list = append(list, provider)
		} else {
			logger.Warnf("providers: unknown provider %q in GPU_PROVIDER_ORDER", name)
		}
	}
	if len(list) == 0 {
		list = []GPUProvider{NewAkashProvider(logger)}
	}
	return &Registry{providers: list, logger: logger}
}

// Dispatch tries each provider in order and returns the first successful result.
func (r *Registry) Dispatch(ctx context.Context, spec DeploySpec) (DeployResult, error) {
	if strings.TrimSpace(spec.ProviderName) != "" {
		return r.DispatchTo(ctx, spec.ProviderName, spec)
	}
	var lastErr error
	for _, p := range r.providers {
		result, err := p.Deploy(ctx, spec)
		if err == nil {
			return result, nil
		}
		r.logger.Warnf("providers: %s failed: %v, trying next", p.Name(), err)
		lastErr = err
	}
	return DeployResult{}, fmt.Errorf("all providers failed: %w", lastErr)
}

// DispatchTo sends a workload to one explicitly selected provider.
func (r *Registry) DispatchTo(ctx context.Context, name string, spec DeploySpec) (DeployResult, error) {
	provider, ok := newProviderByName(name, r.logger)
	if !ok {
		return DeployResult{}, fmt.Errorf("unknown provider %q", name)
	}
	return provider.Deploy(ctx, spec)
}

// Providers returns the ordered provider list (for pool pre-warm decisions).
func (r *Registry) Providers() []GPUProvider { return r.providers }

func newProviderByName(name string, logger *logrus.Logger) (GPUProvider, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "akash":
		return NewAkashProvider(logger), true
	case "huggingface", "hf":
		return NewHuggingFaceProvider(logger), true
	case "aws":
		return NewAWSProvider(), true
	case "azure":
		return NewAzureProvider(), true
	case "modal":
		return NewModalProvider(logger), true
	default:
		return nil, false
	}
}

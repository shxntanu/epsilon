package types

import (
	"context"
)

// Provider adapts a model API to the harness' provider-neutral message types.
type Provider interface {
	Respond(ctx context.Context, req ModelRequest) (*ModelResponse, error)
}

// ModelDelta is one incremental provider update emitted during streaming.
type ModelDelta struct {
	Text string
}

// StreamingProvider can stream incremental model output before returning the
// final provider-neutral response.
type StreamingProvider interface {
	Provider
	StreamRespond(ctx context.Context, req ModelRequest,
		emit func(ModelDelta) error) (*ModelResponse, error)
}

// ModelCatalogProvider can list provider models and their metadata when the
// backing provider exposes model discovery.
type ModelCatalogProvider interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// SelectedModelProvider identifies the provider's currently configured model.
type SelectedModelProvider interface {
	SelectedModel() string
}

package types

import (
	"context"
)

// Provider adapts a model API to the harness' provider-neutral message types.
type Provider interface {
	Respond(ctx context.Context, req ModelRequest) (*ModelResponse, error)
}

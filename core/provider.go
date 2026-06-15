package core

import (
	"context"

	"github.com/shxntanu/epsilon/core/types"
)

// Provider adapts a model API to the harness' provider-neutral message types.
type Provider interface {
	Respond(ctx context.Context, req types.ModelRequest) (*types.ModelResponse, error)
}

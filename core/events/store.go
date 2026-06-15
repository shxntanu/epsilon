package events

import (
	"context"

	"github.com/shxntanu/epsilon/core/types"
)

// Store persists and loads run events.
type Store interface {
	Append(ctx context.Context, event types.Event) error
	Load(ctx context.Context, runID string) ([]types.Event, error)
}

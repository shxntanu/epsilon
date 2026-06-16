package events

import (
	"context"

	"github.com/shxntanu/epsilon/core/types"
)

// Store persists and loads session events.
type Store interface {
	Append(ctx context.Context, event types.Event) error
	Load(ctx context.Context, sessionID string) ([]types.Event, error)
}

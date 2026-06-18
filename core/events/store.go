package events

import (
	"context"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

// SessionInfo summarizes one persisted session.
type SessionInfo struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

// Store persists and loads session events.
type Store interface {
	Append(ctx context.Context, event types.Event) error
	Load(ctx context.Context, sessionID string) ([]types.Event, error)
}

// SessionLister lists persisted sessions when a store supports discovery.
type SessionLister interface {
	ListSessions(ctx context.Context) ([]SessionInfo, error)
}

// SessionRenamer updates persisted per-session metadata.
// Implementations may create a metadata-only session record when needed.
type SessionRenamer interface {
	RenameSession(ctx context.Context, sessionID string, title string) (bool, error)
}

// SessionChecker reports whether a persisted session record exists, even when
// it has no conversation events yet.
type SessionChecker interface {
	SessionExists(ctx context.Context, sessionID string) (bool, error)
}

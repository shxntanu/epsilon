package store

import "context"

// SpaceStore persists Google Chat space/team configuration.
type SpaceStore interface {
	UpsertSpace(ctx context.Context, space Space) error
}

// ThreadStore persists durable per-thread orchestration state.
type ThreadStore interface {
	UpsertThread(ctx context.Context, thread Thread) error
}

// MessageStore persists normalized Chat events.
type MessageStore interface {
	// InsertChatMessage stores a message idempotently.
	// It returns true when a new row was inserted and false for duplicate delivery.
	InsertChatMessage(ctx context.Context, message ChatMessage) (bool, error)
}

// TaskStore persists the early task graph/state transitions epsilond needs.
type TaskStore interface {
	CreateTask(ctx context.Context, task Task) error
	UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus) error
}

// UsageStore records model/token spend attribution.
type UsageStore interface {
	RecordUsage(ctx context.Context, entry UsageLedgerEntry) error
}

// Store is the minimal aggregate dependency for epsilond's first storage wiring.
type Store interface {
	SpaceStore
	ThreadStore
	MessageStore
	TaskStore
	UsageStore
}

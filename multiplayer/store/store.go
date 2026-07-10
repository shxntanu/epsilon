package store

import "context"

// SpaceStore persists Google Chat space/team configuration.
type SpaceStore interface {
	UpsertSpace(ctx context.Context, space Space) error
}

// ThreadStore persists durable per-thread orchestration state.
type ThreadStore interface {
	UpsertThread(ctx context.Context, thread Thread) error
	UpdateThreadState(ctx context.Context, thread Thread, expectedVersion int64) (bool, error)
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

// ArtifactStore persists attachment and generated artifact metadata.
type ArtifactStore interface {
	InsertChatArtifact(ctx context.Context, artifact ChatArtifact) error
}

// WorkerRunStore persists sandbox worker execution records.
type WorkerRunStore interface {
	CreateWorkerRun(ctx context.Context, run WorkerRun) error
	UpdateWorkerRunStatus(ctx context.Context, runID string, status WorkerRunStatus, resultPointer string, errMessage string) error
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
	ArtifactStore
	TaskStore
	WorkerRunStore
	UsageStore
}

// Migrator updates backing storage schema for prototype deployments.
type Migrator interface {
	AutoMigrate(ctx context.Context) error
}

// HealthChecker verifies the backing store can serve requests.
type HealthChecker interface {
	Ready(ctx context.Context) error
}

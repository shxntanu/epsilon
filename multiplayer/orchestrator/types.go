package orchestrator

import (
	"context"
	"time"
)

// ThreadKey identifies one durable conversation thread managed by epsilond.
type ThreadKey struct {
	Platform         string
	SpaceID          string
	ThreadID         string
	SpaceExternalID  string
	ThreadExternalID string
}

// NewChatMessageSignal is sent when epsilond receives a new user message for a
// thread workflow.
type NewChatMessageSignal struct {
	Thread            ThreadKey
	MessageID         string
	ExternalMessageID string
	SenderID          string
	SenderName        string
	Text              string
	ArgumentText      string
	Artifacts         []ArtifactRef
	ReceivedAt        time.Time
}

// InterruptSignal asks a running thread workflow to pause, cancel, or redirect
// current work.
type InterruptSignal struct {
	Thread    ThreadKey
	Kind      InterruptKind
	Reason    string
	MessageID string
	IssuedBy  string
	IssuedAt  time.Time
}

// StatusQuery requests the current orchestration status for a thread.
type StatusQuery struct {
	Thread ThreadKey
}

// Status is the current externally visible state of a thread workflow.
type Status struct {
	Thread        ThreadKey
	State         TaskState
	ActiveTaskID  string
	Plan          TaskPlan
	LastMessage   string
	BlockedReason string
	Error         string
	UpdatedAt     time.Time
}

// UpdateTaskRequest records a state transition or result for one task in a
// thread-owned task graph.
type UpdateTaskRequest struct {
	Thread        ThreadKey
	TaskID        string
	State         TaskState
	Result        WorkerResult
	BlockedReason string
	Error         string
	UpdatedAt     time.Time
}

// TaskPlan describes the task graph currently owned by one thread workflow.
type TaskPlan struct {
	ID        string
	Thread    ThreadKey
	Goal      string
	State     TaskState
	Subtasks  []Subtask
	Budget    Budget
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Subtask is one node in a thread workflow's task graph.
type Subtask struct {
	ID           string
	ParentID     string
	Goal         string
	State        TaskState
	WorkerRole   string
	Dependencies []string
	Artifacts    []ArtifactRef
	Memory       []MemoryRef
	Result       WorkerResult
	CreatedAt    time.Time
	StartedAt    time.Time
	CompletedAt  time.Time
}

// TaskState is the lifecycle state for a plan or subtask.
type TaskState string

const (
	// TaskStateQueued means the task has been accepted but not planned or run.
	TaskStateQueued TaskState = "queued"
	// TaskStatePlanning means the orchestrator is decomposing the request.
	TaskStatePlanning TaskState = "planning"
	// TaskStateRunning means work is actively executing.
	TaskStateRunning TaskState = "running"
	// TaskStateBlocked means progress requires user input or external change.
	TaskStateBlocked TaskState = "blocked"
	// TaskStateCompleted means work finished successfully.
	TaskStateCompleted TaskState = "completed"
	// TaskStateFailed means work ended with an unrecoverable error.
	TaskStateFailed TaskState = "failed"
	// TaskStateCancelled means work was stopped before completion.
	TaskStateCancelled TaskState = "cancelled"
)

// ArtifactRef points to an attachment, generated file, or stored artifact used
// by orchestration.
type ArtifactRef struct {
	ID          string
	Kind        string
	Name        string
	MimeType    string
	URI         string
	ContentHash string
	Metadata    map[string]string
}

// MemoryRef points to durable context selected for a thread or task.
type MemoryRef struct {
	ID      string
	Kind    string
	Summary string
	URI     string
}

// WorkerResult is the structured output returned by a worker subtask.
type WorkerResult struct {
	State        TaskState
	Summary      string
	Artifacts    []ArtifactRef
	Memory       []MemoryRef
	Evidence     []EvidenceRef
	Error        string
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
	CompletedAt  time.Time
}

// EvidenceRef points to information that supports a worker result.
type EvidenceRef struct {
	ID       string
	Kind     string
	Source   string
	Excerpt  string
	URI      string
	Metadata map[string]string
}

// Budget captures execution limits for a plan or task.
type Budget struct {
	MaxInputTokens  int64
	MaxOutputTokens int64
	MaxCostUSD      float64
	MaxWorkers      int
	Deadline        time.Time
}

// InterruptKind describes the requested interruption behavior.
type InterruptKind string

const (
	// InterruptKindPause asks the workflow to stop after reaching a stable point.
	InterruptKindPause InterruptKind = "pause"
	// InterruptKindCancel asks the workflow to cancel outstanding work.
	InterruptKindCancel InterruptKind = "cancel"
	// InterruptKindClarify records new user guidance for blocked or running work.
	InterruptKindClarify InterruptKind = "clarify"
	// InterruptKindReplan asks the workflow to revise the current task graph.
	InterruptKindReplan InterruptKind = "replan"
)

// Client controls durable thread workflows through implementation-neutral
// orchestration operations.
type Client interface {
	StartOrSignal(ctx context.Context, signal NewChatMessageSignal) (Status, error)
	SignalMessage(ctx context.Context, signal NewChatMessageSignal) error
	Interrupt(ctx context.Context, signal InterruptSignal) (Status, error)
	UpdateTask(ctx context.Context, request UpdateTaskRequest) (Status, error)
	Status(ctx context.Context, query StatusQuery) (Status, error)
}

// ThreadWorkflowClient is an explicit name for implementations that back the
// orchestration client with a thread workflow engine.
type ThreadWorkflowClient interface {
	Client
}

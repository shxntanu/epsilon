package store

import (
	"encoding/json"
	"time"
)

// Space is the durable configuration for one Google Chat space/team.
type Space struct {
	ID              string
	GoogleSpaceID   string
	GoogleSpaceName string
	DisplayName     string
	Enabled         bool
	BudgetPolicy    json.RawMessage
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Thread tracks orchestrator state for one mentioned Google Chat thread.
type Thread struct {
	ID                 string
	SpaceID            string
	GoogleThreadID     string
	GoogleThreadName   string
	Status             ThreadStatus
	SummaryPointer     string
	PinnedFacts        json.RawMessage
	WorkspaceTier      string
	TemporalWorkflowID string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ChatMessage is a normalized message from a mentioned Google Chat thread.
type ChatMessage struct {
	ID                string
	SpaceID           string
	ThreadID          string
	GoogleMessageID   string
	GoogleMessageName string
	GoogleEventID     string
	SenderID          string
	SenderDisplayName string
	Text              string
	ArgumentText      string
	MentionedEpsilon  bool
	RawEvent          json.RawMessage
	GoogleCreateTime  time.Time
	ReceivedAt        time.Time
	CreatedAt         time.Time
}

// ChatArtifact stores attachment metadata and extraction pointers.
type ChatArtifact struct {
	ID               string
	MessageID        string
	SpaceID          string
	ThreadID         string
	GoogleAttachment string
	FileName         string
	MimeType         string
	ContentHash      string
	StoragePath      string
	ExtractionStatus ArtifactExtractionStatus
	Metadata         json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Task is a node in a thread-owned task graph.
type Task struct {
	ID              string
	ThreadID        string
	SpaceID         string
	ParentTaskID    string
	Status          TaskStatus
	Goal            string
	Plan            json.RawMessage
	ResultPointer   string
	Budget          json.RawMessage
	CancellationGen int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartedAt       time.Time
	CompletedAt     time.Time
}

// WorkerRun records one isolated sandbox execution for a task.
type WorkerRun struct {
	ID            string
	TaskID        string
	ThreadID      string
	WorkerRole    string
	Model         string
	Status        WorkerRunStatus
	SandboxImage  string
	ResultPointer string
	ExitCode      *int
	ErrorMessage  string
	InputTokens   int64
	OutputTokens  int64
	EstimatedCost string
	StartedAt     time.Time
	CompletedAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UsageLedgerEntry attributes model spend to a space, thread, task, and worker run.
type UsageLedgerEntry struct {
	ID            string
	SpaceID       string
	ThreadID      string
	TaskID        string
	WorkerRunID   string
	Provider      string
	Model         string
	Reason        string
	InputTokens   int64
	OutputTokens  int64
	EstimatedCost string
	Metadata      json.RawMessage
	CreatedAt     time.Time
}

type ThreadStatus string

const (
	ThreadStatusActive   ThreadStatus = "active"
	ThreadStatusDormant  ThreadStatus = "dormant"
	ThreadStatusArchived ThreadStatus = "archived"
)

type ArtifactExtractionStatus string

const (
	ArtifactExtractionPending ArtifactExtractionStatus = "pending"
	ArtifactExtractionSkipped ArtifactExtractionStatus = "skipped"
	ArtifactExtractionReady   ArtifactExtractionStatus = "ready"
	ArtifactExtractionFailed  ArtifactExtractionStatus = "failed"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type WorkerRunStatus string

const (
	WorkerRunStatusPending   WorkerRunStatus = "pending"
	WorkerRunStatusRunning   WorkerRunStatus = "running"
	WorkerRunStatusSucceeded WorkerRunStatus = "succeeded"
	WorkerRunStatusFailed    WorkerRunStatus = "failed"
	WorkerRunStatusCancelled WorkerRunStatus = "cancelled"
)

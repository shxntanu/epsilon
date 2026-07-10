// Package activities contains Temporal activity scaffolding for multiplayer
// workflows.
package activities

import (
	"context"
	"errors"

	"github.com/shxntanu/epsilon/multiplayer/chat"
	"github.com/shxntanu/epsilon/multiplayer/memory"
	"github.com/shxntanu/epsilon/multiplayer/sandbox"
	"github.com/shxntanu/epsilon/multiplayer/store"
	"github.com/shxntanu/epsilon/multiplayer/workspace"
)

var (
	errMissingStore            = errors.New("activities: store dependency is nil")
	errMissingChatClient       = errors.New("activities: chat client dependency is nil")
	errMissingMemoryService    = errors.New("activities: memory service dependency is nil")
	errMissingSandboxRunner    = errors.New("activities: sandbox runner dependency is nil")
	errMissingWorkspaceManager = errors.New("activities: workspace manager dependency is nil")
)

// Activities groups dependencies used by Temporal activities.
type Activities struct {
	Store            store.Store
	ChatClient       chat.Client
	MemoryService    memory.Service
	SandboxRunner    sandbox.Runner
	WorkspaceManager *workspace.Manager
}

// PersistMessageActivityInput is the input for PersistMessageActivity.
type PersistMessageActivityInput struct {
	Message store.ChatMessage
}

// PersistMessageActivityOutput is the output for PersistMessageActivity.
type PersistMessageActivityOutput struct {
	Inserted bool
}

// PostChatMessageActivityInput is the input for PostChatMessageActivity.
type PostChatMessageActivityInput struct {
	Reply chat.Reply
}

// PostChatMessageActivityOutput is the output for PostChatMessageActivity.
type PostChatMessageActivityOutput struct {
	Posted bool
}

// FetchAttachmentActivityInput is the input for FetchAttachmentActivity.
type FetchAttachmentActivityInput struct {
	Attachment chat.Attachment
	Artifact   store.ChatArtifact
}

// FetchAttachmentActivityOutput is the output for FetchAttachmentActivity.
type FetchAttachmentActivityOutput struct {
	Artifact    store.ChatArtifact
	Skipped     bool
	Reason      string
	ContentHash string
	StoragePath string
}

// SyncRepoCacheActivityInput is the input for SyncRepoCacheActivity.
type SyncRepoCacheActivityInput struct {
	ThreadID string
	Repo     string
	Ref      string
}

// SyncRepoCacheActivityOutput is the output for SyncRepoCacheActivity.
type SyncRepoCacheActivityOutput struct {
	RepoCachePath string
	Skipped       bool
	Reason        string
}

// AcquireWorkspaceActivityInput is the input for AcquireWorkspaceActivity.
type AcquireWorkspaceActivityInput struct {
	ThreadID string
}

// AcquireWorkspaceActivityOutput is the output for AcquireWorkspaceActivity.
type AcquireWorkspaceActivityOutput struct {
	Workspace *workspace.Workspace
}

// RunSandboxWorkerActivityInput is the input for RunSandboxWorkerActivity.
type RunSandboxWorkerActivityInput struct {
	Spec sandbox.TaskSpec
}

// RunSandboxWorkerActivityOutput is the output for RunSandboxWorkerActivity.
type RunSandboxWorkerActivityOutput struct {
	Result sandbox.Result
}

// RetrieveMemoryActivityInput is the input for RetrieveMemoryActivity.
type RetrieveMemoryActivityInput struct {
	Request memory.SearchRequest
}

// RetrieveMemoryActivityOutput is the output for RetrieveMemoryActivity.
type RetrieveMemoryActivityOutput struct {
	Results []memory.SearchResult
}

// WriteMemoryActivityInput is the input for WriteMemoryActivity.
type WriteMemoryActivityInput struct {
	Request memory.WriteRequest
}

// WriteMemoryActivityOutput is the output for WriteMemoryActivity.
type WriteMemoryActivityOutput struct {
	Memory memory.Memory
}

// RecordUsageActivityInput is the input for RecordUsageActivity.
type RecordUsageActivityInput struct {
	Entry store.UsageLedgerEntry
}

// RecordUsageActivityOutput is the output for RecordUsageActivity.
type RecordUsageActivityOutput struct {
	Recorded bool
}

// PersistMessageActivity persists a normalized chat message idempotently.
func (a Activities) PersistMessageActivity(ctx context.Context, input PersistMessageActivityInput) (PersistMessageActivityOutput, error) {
	if a.Store == nil {
		return PersistMessageActivityOutput{}, errMissingStore
	}
	inserted, err := a.Store.InsertChatMessage(ctx, input.Message)
	if err != nil {
		return PersistMessageActivityOutput{}, err
	}
	return PersistMessageActivityOutput{Inserted: inserted}, nil
}

// PostChatMessageActivity posts a chat reply.
func (a Activities) PostChatMessageActivity(ctx context.Context, input PostChatMessageActivityInput) (PostChatMessageActivityOutput, error) {
	if a.ChatClient == nil {
		return PostChatMessageActivityOutput{}, errMissingChatClient
	}
	if err := a.ChatClient.Reply(ctx, input.Reply); err != nil {
		return PostChatMessageActivityOutput{}, err
	}
	return PostChatMessageActivityOutput{Posted: true}, nil
}

// FetchAttachmentActivity is a placeholder for attachment download/extraction.
func (a Activities) FetchAttachmentActivity(ctx context.Context, input FetchAttachmentActivityInput) (FetchAttachmentActivityOutput, error) {
	if err := ctx.Err(); err != nil {
		return FetchAttachmentActivityOutput{}, err
	}
	return FetchAttachmentActivityOutput{
		Artifact: input.Artifact,
		Skipped:  true,
		Reason:   "attachment fetching is not implemented",
	}, nil
}

// SyncRepoCacheActivity is a placeholder for repository cache synchronization.
func (a Activities) SyncRepoCacheActivity(ctx context.Context, input SyncRepoCacheActivityInput) (SyncRepoCacheActivityOutput, error) {
	if err := ctx.Err(); err != nil {
		return SyncRepoCacheActivityOutput{}, err
	}
	output := SyncRepoCacheActivityOutput{
		Skipped: true,
		Reason:  "repo cache sync is not implemented",
	}
	if a.WorkspaceManager != nil && input.ThreadID != "" && input.Repo != "" {
		ws, err := a.WorkspaceManager.Acquire(ctx, input.ThreadID)
		if err != nil {
			return SyncRepoCacheActivityOutput{}, err
		}
		repoCachePath, err := ws.RepoCachePath(input.Repo)
		if err != nil {
			return SyncRepoCacheActivityOutput{}, err
		}
		output.RepoCachePath = repoCachePath
	}
	return output, nil
}

// AcquireWorkspaceActivity ensures a workspace exists for a thread.
func (a Activities) AcquireWorkspaceActivity(ctx context.Context, input AcquireWorkspaceActivityInput) (AcquireWorkspaceActivityOutput, error) {
	if a.WorkspaceManager == nil {
		return AcquireWorkspaceActivityOutput{}, errMissingWorkspaceManager
	}
	ws, err := a.WorkspaceManager.Acquire(ctx, input.ThreadID)
	if err != nil {
		return AcquireWorkspaceActivityOutput{}, err
	}
	return AcquireWorkspaceActivityOutput{Workspace: ws}, nil
}

// RunSandboxWorkerActivity runs a sandbox worker task.
func (a Activities) RunSandboxWorkerActivity(ctx context.Context, input RunSandboxWorkerActivityInput) (RunSandboxWorkerActivityOutput, error) {
	if a.SandboxRunner == nil {
		return RunSandboxWorkerActivityOutput{}, errMissingSandboxRunner
	}
	result, err := a.SandboxRunner.Run(ctx, input.Spec)
	if err != nil {
		return RunSandboxWorkerActivityOutput{}, err
	}
	return RunSandboxWorkerActivityOutput{Result: result}, nil
}

// RetrieveMemoryActivity searches scoped memory.
func (a Activities) RetrieveMemoryActivity(ctx context.Context, input RetrieveMemoryActivityInput) (RetrieveMemoryActivityOutput, error) {
	if a.MemoryService == nil {
		return RetrieveMemoryActivityOutput{}, errMissingMemoryService
	}
	results, err := a.MemoryService.Search(ctx, input.Request)
	if err != nil {
		return RetrieveMemoryActivityOutput{}, err
	}
	return RetrieveMemoryActivityOutput{Results: results}, nil
}

// WriteMemoryActivity writes one memory.
func (a Activities) WriteMemoryActivity(ctx context.Context, input WriteMemoryActivityInput) (WriteMemoryActivityOutput, error) {
	if a.MemoryService == nil {
		return WriteMemoryActivityOutput{}, errMissingMemoryService
	}
	mem, err := a.MemoryService.Write(ctx, input.Request)
	if err != nil {
		return WriteMemoryActivityOutput{}, err
	}
	return WriteMemoryActivityOutput{Memory: mem}, nil
}

// RecordUsageActivity records a usage ledger entry.
func (a Activities) RecordUsageActivity(ctx context.Context, input RecordUsageActivityInput) (RecordUsageActivityOutput, error) {
	if a.Store == nil {
		return RecordUsageActivityOutput{}, errMissingStore
	}
	if err := a.Store.RecordUsage(ctx, input.Entry); err != nil {
		return RecordUsageActivityOutput{}, err
	}
	return RecordUsageActivityOutput{Recorded: true}, nil
}

// Package activities contains Temporal activity scaffolding for multiplayer
// workflows.
package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/shxntanu/epsilon/multiplayer/chat"
	"github.com/shxntanu/epsilon/multiplayer/memory"
	"github.com/shxntanu/epsilon/multiplayer/sandbox"
	"github.com/shxntanu/epsilon/multiplayer/store"
	"github.com/shxntanu/epsilon/multiplayer/workspace"
)

var (
	errMissingStore            = errors.New("activities: store dependency is nil")
	errMissingChatClient       = errors.New("activities: chat client dependency is nil")
	errMissingAttachmentClient = errors.New("activities: attachment client dependency is nil")
	errMissingMemoryService    = errors.New("activities: memory service dependency is nil")
	errMissingSandboxRunner    = errors.New("activities: sandbox runner dependency is nil")
	errMissingWorkspaceManager = errors.New("activities: workspace manager dependency is nil")
)

// Activities groups dependencies used by Temporal activities.
type Activities struct {
	Store            store.Store
	ChatClient       chat.Client
	AttachmentClient chat.AttachmentDownloader
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

// FetchAttachmentActivity downloads, stores, and indexes one attachment body.
func (a Activities) FetchAttachmentActivity(ctx context.Context, input FetchAttachmentActivityInput) (FetchAttachmentActivityOutput, error) {
	if err := ctx.Err(); err != nil {
		return FetchAttachmentActivityOutput{}, err
	}
	if a.WorkspaceManager == nil {
		return FetchAttachmentActivityOutput{}, errMissingWorkspaceManager
	}
	if a.AttachmentClient == nil {
		return FetchAttachmentActivityOutput{}, errMissingAttachmentClient
	}
	if a.Store == nil {
		return FetchAttachmentActivityOutput{}, errMissingStore
	}
	ws, err := a.WorkspaceManager.Acquire(ctx, input.Artifact.ThreadID)
	if err != nil {
		return FetchAttachmentActivityOutput{}, err
	}
	body, err := a.AttachmentClient.DownloadAttachment(ctx, input.Attachment)
	if err != nil {
		return FetchAttachmentActivityOutput{}, fmt.Errorf("download attachment: %w", err)
	}
	sum := sha256.Sum256(body)
	contentHash := hex.EncodeToString(sum[:])
	storagePath, err := ws.ArtifactPath(input.Artifact.FileName, contentHash)
	if err != nil {
		return FetchAttachmentActivityOutput{}, err
	}
	if err := writeFileIfAbsent(ctx, storagePath, body, 0o640); err != nil {
		return FetchAttachmentActivityOutput{}, err
	}

	artifact := input.Artifact
	artifact.ContentHash = contentHash
	artifact.StoragePath = storagePath
	artifact.ExtractionStatus = store.ArtifactExtractionSkipped
	metadata := artifactMetadata(artifact.Metadata)
	metadata["content_size_bytes"] = strconv.Itoa(len(body))

	if isTextLikeArtifact(artifact.FileName, artifact.MimeType) && utf8.Valid(body) {
		textPath, err := ws.ExtractedTextPath(contentHash)
		if err != nil {
			return FetchAttachmentActivityOutput{}, err
		}
		text := body
		if len(text) > 1<<20 {
			text = text[:1<<20]
			metadata["extraction_truncated"] = "true"
		}
		if err := writeFileIfAbsent(ctx, textPath, text, 0o640); err != nil {
			return FetchAttachmentActivityOutput{}, err
		}
		artifact.ExtractionStatus = store.ArtifactExtractionReady
		metadata["extraction_path"] = textPath
	} else {
		metadata["extraction_reason"] = "unsupported_or_binary_type"
	}
	artifact.Metadata = jsonRawObject(metadata)
	if err := a.Store.UpdateChatArtifact(ctx, artifact); err != nil {
		return FetchAttachmentActivityOutput{}, err
	}
	return FetchAttachmentActivityOutput{
		Artifact:    artifact,
		ContentHash: contentHash,
		StoragePath: storagePath,
	}, nil
}

// SyncRepoCacheActivity clones or fetches a bare repository cache.
func (a Activities) SyncRepoCacheActivity(ctx context.Context, input SyncRepoCacheActivityInput) (SyncRepoCacheActivityOutput, error) {
	if err := ctx.Err(); err != nil {
		return SyncRepoCacheActivityOutput{}, err
	}
	if a.WorkspaceManager == nil {
		return SyncRepoCacheActivityOutput{}, errMissingWorkspaceManager
	}
	ws, err := a.WorkspaceManager.Acquire(ctx, input.ThreadID)
	if err != nil {
		return SyncRepoCacheActivityOutput{}, err
	}
	repoCachePath, err := ws.RepoCachePath(input.Repo)
	if err != nil {
		return SyncRepoCacheActivityOutput{}, err
	}
	if _, err := os.Stat(filepath.Join(repoCachePath, "HEAD")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(repoCachePath), 0o750); err != nil {
			return SyncRepoCacheActivityOutput{}, fmt.Errorf("create repo cache root: %w", err)
		}
		if err := runGit(ctx, "clone", "--mirror", input.Repo, repoCachePath); err != nil {
			return SyncRepoCacheActivityOutput{}, err
		}
	} else if err != nil {
		return SyncRepoCacheActivityOutput{}, fmt.Errorf("inspect repo cache: %w", err)
	} else if strings.TrimSpace(input.Ref) != "" {
		if err := runGit(ctx, "-C", repoCachePath, "fetch", "--prune", "origin", input.Ref); err != nil {
			return SyncRepoCacheActivityOutput{}, err
		}
	} else if err := runGit(ctx, "-C", repoCachePath, "remote", "update", "--prune"); err != nil {
		return SyncRepoCacheActivityOutput{}, err
	}
	return SyncRepoCacheActivityOutput{RepoCachePath: repoCachePath}, nil
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

func writeFileIfAbsent(ctx context.Context, path string, body []byte, perm os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create artifact file: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return fmt.Errorf("write artifact file: %w", err)
	}
	return nil
}

func runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, truncateForError(output))
	}
	return nil
}

func truncateForError(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 8192 {
		return text[:8192]
	}
	return text
}

func isTextLikeArtifact(fileName string, mimeType string) bool {
	mediaType, _, _ := mime.ParseMediaType(mimeType)
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/x-yaml", "application/yaml":
		return true
	}
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".txt", ".log", ".md", ".json", ".yaml", ".yml", ".csv", ".tsv", ".xml",
		".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".java", ".rb", ".rs", ".sh",
		".toml", ".ini", ".env":
		return true
	default:
		return false
	}
}

func artifactMetadata(raw json.RawMessage) map[string]string {
	metadata := map[string]string{}
	if len(raw) == 0 {
		return metadata
	}
	var stringMap map[string]string
	if err := json.Unmarshal(raw, &stringMap); err == nil {
		return stringMap
	}
	var anyMap map[string]any
	if err := json.Unmarshal(raw, &anyMap); err != nil {
		return metadata
	}
	for key, value := range anyMap {
		metadata[key] = fmt.Sprint(value)
	}
	return metadata
}

func jsonRawObject(metadata map[string]string) json.RawMessage {
	body, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(body)
}

// Package workspace manages epsilond's persistent per-thread workspace layout.
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config controls local workspace paths.
type Config struct {
	WorkspaceRoot string
	RepoCacheRoot string
}

// Workspace is the local filesystem layout for one Google Chat thread.
type Workspace struct {
	ThreadID      string
	Root          string
	ArtifactsDir  string
	SummariesDir  string
	TasksDir      string
	RepoCacheRoot string
}

// TaskScratch describes the writable scratch directory for one task.
type TaskScratch struct {
	TaskID string
	Path   string
}

// Manager prepares local workspace directories.
type Manager struct {
	cfg Config
}

// NewManager returns a workspace manager.
func NewManager(cfg Config) (*Manager, error) {
	cfg.WorkspaceRoot = strings.TrimSpace(cfg.WorkspaceRoot)
	if cfg.WorkspaceRoot == "" {
		return nil, fmt.Errorf("workspace root is empty")
	}
	cfg.RepoCacheRoot = strings.TrimSpace(cfg.RepoCacheRoot)
	if cfg.RepoCacheRoot == "" {
		return nil, fmt.Errorf("repo cache root is empty")
	}
	return &Manager{cfg: cfg}, nil
}

// Acquire ensures a local workspace exists for a thread.
func (m *Manager) Acquire(ctx context.Context, threadID string) (*Workspace, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("acquire workspace cancelled: %w", ctx.Err())
	default:
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("thread ID is empty")
	}
	root := filepath.Join(m.cfg.WorkspaceRoot, safePathID(threadID))
	ws := &Workspace{
		ThreadID:      threadID,
		Root:          root,
		ArtifactsDir:  filepath.Join(root, "artifacts"),
		SummariesDir:  filepath.Join(root, "summaries"),
		TasksDir:      filepath.Join(root, "tasks"),
		RepoCacheRoot: m.cfg.RepoCacheRoot,
	}
	for _, dir := range []string{ws.Root, ws.ArtifactsDir, ws.SummariesDir, ws.TasksDir, ws.RepoCacheRoot} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create workspace directory %s: %w", dir, err)
		}
	}
	return ws, nil
}

// TaskScratch ensures and returns the writable scratch directory for a task.
func (w *Workspace) TaskScratch(ctx context.Context, taskID string) (TaskScratch, error) {
	select {
	case <-ctx.Done():
		return TaskScratch{}, fmt.Errorf("create task scratch cancelled: %w", ctx.Err())
	default:
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TaskScratch{}, fmt.Errorf("task ID is empty")
	}
	path := filepath.Join(w.TasksDir, safePathID(taskID), "scratch")
	if err := os.MkdirAll(path, 0o750); err != nil {
		return TaskScratch{}, fmt.Errorf("create task scratch: %w", err)
	}
	return TaskScratch{TaskID: taskID, Path: path}, nil
}

// RepoCachePath returns the local bare-repo cache path for a repo identifier.
func (w *Workspace) RepoCachePath(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", fmt.Errorf("repo is empty")
	}
	return filepath.Join(w.RepoCacheRoot, safePathID(repo)+".git"), nil
}

// ArtifactPath returns a deterministic artifact path for a file name and content hash.
func (w *Workspace) ArtifactPath(fileName string, contentHash string) (string, error) {
	contentHash = strings.TrimSpace(contentHash)
	if contentHash == "" {
		return "", fmt.Errorf("content hash is empty")
	}
	name := safePathID(fileName)
	if name == "" {
		name = "artifact"
	}
	return filepath.Join(w.ArtifactsDir, contentHash+"-"+name), nil
}

func safePathID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	safe := strings.Trim(b.String(), "._-")
	if safe == "" || len(safe) > 80 {
		sum := sha256.Sum256([]byte(value))
		safe = hex.EncodeToString(sum[:])[:32]
	}
	return safe
}

// SummaryPath returns a timestamped summary artifact path.
func (w *Workspace) SummaryPath(now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return filepath.Join(w.SummariesDir, now.UTC().Format("20060102T150405Z")+".md")
}

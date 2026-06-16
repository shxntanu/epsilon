package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

const defaultWriteFileMaxBytes = 512 * 1024
const defaultWriteFileDiffMaxBytes = 64 * 1024

// WriteFileTool writes UTF-8 text to files inside a workspace.
type WriteFileTool struct {
	workspace *Workspace
	maxBytes  int
}

// NewWriteFileTool returns a write_file tool scoped to workspaceRoot.
func NewWriteFileTool(workspaceRoot string) (*WriteFileTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}

	return &WriteFileTool{
		workspace: workspace,
		maxBytes:  defaultWriteFileMaxBytes,
	}, nil
}

// Name returns the tool name exposed to the model.
func (t *WriteFileTool) Name() string {
	return "write_file"
}

// Description returns the tool description exposed to the model.
func (t *WriteFileTool) Description() string {
	return "Write UTF-8 text to a workspace-relative file. Creates parent directories and " +
		"requires overwrite=true for existing files."
}

// InputSchema returns the tool input schema.
func (t *WriteFileTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description"` +
		`:"Workspace-relative path to write."},"content":{"type":"string","description":"Complete` +
		` file contents to write."},"overwrite":{"type":"boolean","description":"Set true to ` +
		`replace an existing file."}},"required":["path","content"],"additionalProperties":false}`)
}

// Permission returns the default permission mode for this tool.
func (t *WriteFileTool) Permission() types.PermissionMode {
	return types.PermissionAsk
}

// Run executes the tool.
func (t *WriteFileTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("write_file cancelled: %w", ctx.Err())
	default:
	}

	var req struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode write_file input: %w", err)
	}
	if len(req.Content) > t.maxBytes {
		return nil, fmt.Errorf("content is too large: %d bytes exceeds %d", len(req.Content),
			t.maxBytes)
	}

	path, err := t.workspace.resolveWritePath(req.Path)
	if err != nil {
		return nil, err
	}

	var previous []byte
	existed := false
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return nil, fmt.Errorf("path is a directory: %s", req.Path)
		}
		if !req.Overwrite {
			return nil, fmt.Errorf("file exists and overwrite is false: %s", req.Path)
		}
		existed = true
		previous, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read existing file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	parent := filepath.Dir(path)
	temp, err := os.CreateTemp(parent, ".epsilon-write-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tempName := temp.Name()
	shouldRemoveTemp := true
	defer func() {
		if shouldRemoveTemp {
			_ = os.Remove(tempName)
		}
	}()

	if _, err := temp.WriteString(req.Content); err != nil {
		if closeErr := temp.Close(); closeErr != nil {
			return nil, fmt.Errorf("write temp file: %w; close temp file: %w", err, closeErr)
		}
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("write_file cancelled: %w", ctx.Err())
	default:
	}

	if err := os.Rename(tempName, path); err != nil {
		return nil, fmt.Errorf("replace target file: %w", err)
	}
	shouldRemoveTemp = false

	result := types.TextToolResult(fmt.Sprintf("wrote %d bytes to %s", len(req.Content), req.Path))
	result.Metadata = map[string]string{
		"path":      req.Path,
		"bytes":     fmt.Sprintf("%d", len(req.Content)),
		"workspace": t.workspace.Root(),
	}
	if diff := writeFileDiff(req.Path, previous, []byte(req.Content), existed); diff != "" {
		result.Metadata["diff"] = diff
	}

	return &result, nil
}

func writeFileDiff(path string, before []byte, after []byte, existed bool) string {
	if existed && bytes.Equal(before, after) {
		return ""
	}

	var b strings.Builder
	if existed {
		fmt.Fprintf(&b, "--- a/%s\n", path)
	} else {
		b.WriteString("--- /dev/null\n")
	}
	fmt.Fprintf(&b, "+++ b/%s\n@@\n", path)
	if existed {
		writeFullFileDiffLines(&b, '-', string(before))
	}
	writeFullFileDiffLines(&b, '+', string(after))

	diff := b.String()
	if len(diff) <= defaultWriteFileDiffMaxBytes {
		return diff
	}

	return fmt.Sprintf("--- a/%s\n+++ b/%s\n@@\n%s\n", path, path,
		"[diff omitted: write_file change is too large to render inline]")
}

func writeFullFileDiffLines(b *strings.Builder, prefix byte, text string) {
	if text == "" {
		return
	}

	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for _, line := range lines {
		b.WriteByte(prefix)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if !strings.HasSuffix(text, "\n") {
		b.WriteString(`\ No newline at end of file`)
		b.WriteByte('\n')
	}
}

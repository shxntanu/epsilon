package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/shxntanu/epsilon/core/types"
)

const defaultReadFileMaxBytes = 128 * 1024

// ReadFileTool reads UTF-8 text from files inside a workspace.
type ReadFileTool struct {
	workspace *Workspace
	maxBytes  int64
}

// NewReadFileTool returns a read_file tool scoped to workspaceRoot.
func NewReadFileTool(workspaceRoot string) (*ReadFileTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}

	return &ReadFileTool{
		workspace: workspace,
		maxBytes:  defaultReadFileMaxBytes,
	}, nil
}

// Name returns the tool name exposed to the model.
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Description returns the tool description exposed to the model.
func (t *ReadFileTool) Description() string {
	return "Read a text file from the workspace using a relative path. The file is truncated" +
		" when it exceeds the configured byte limit."
}

// InputSchema returns the tool input schema.
func (t *ReadFileTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description"` +
		`:"Workspace-relative path to read."}},"required":["path"],"additionalProperties":false}`)
}

// Permission returns the default permission mode for this tool.
func (t *ReadFileTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

// Run executes the tool.
func (t *ReadFileTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("read_file cancelled: %w", ctx.Err())
	default:
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode read_file input: %w", err)
	}

	path, err := t.workspace.resolveReadPath(req.Path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", req.Path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	limited := io.LimitReader(file, t.maxBytes+1)
	data, err := io.ReadAll(limited)
	closeErr := file.Close()
	if err != nil {
		if closeErr != nil {
			return nil, fmt.Errorf("read file: %w; close file: %w", err, closeErr)
		}
		return nil, fmt.Errorf("read file: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close file: %w", closeErr)
	}

	truncated := int64(len(data)) > t.maxBytes
	if truncated {
		data = data[:int(t.maxBytes)]
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("read_file cancelled: %w", ctx.Err())
	default:
	}

	result := types.TextToolResult(string(data))
	result.Metadata = map[string]string{
		"path":       req.Path,
		"bytes":      fmt.Sprintf("%d", len(data)),
		"truncated":  fmt.Sprintf("%t", truncated),
		"workspace":  t.workspace.Root(),
		"max_bytes":  fmt.Sprintf("%d", t.maxBytes),
		"file_bytes": fmt.Sprintf("%d", info.Size()),
	}

	return &result, nil
}

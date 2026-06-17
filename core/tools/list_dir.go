package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

const defaultListDirMaxEntries = 200

type listDirInput struct {
	Path       string `json:"path,omitempty"`
	MaxEntries int    `json:"max_entries,omitempty"`
}

type ListDirTool struct {
	workspace *Workspace
}

func NewListDirTool(workspaceRoot string) (*ListDirTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &ListDirTool{workspace: workspace}, nil
}

func (t *ListDirTool) Name() string {
	return "list_dir"
}

func (t *ListDirTool) Description() string {
	return "List the immediate children of a workspace directory, like ls. Use this before " +
		"reading files when gradually exploring an unfamiliar repository."
}

func (t *ListDirTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string",` +
		`"description":"Workspace-relative directory path to list. Defaults to '.'."},` +
		`"max_entries":{"type":"integer","minimum":1,"maximum":1000,"description":` +
		`"Maximum entries to return. Defaults to 200."}},"additionalProperties":false}`)
}

func (t *ListDirTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

func (t *ListDirTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var req listDirInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode list_dir input: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list_dir cancelled: %w", err)
	}

	maxEntries := req.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultListDirMaxEntries
	}
	if maxEntries > 1000 {
		maxEntries = 1000
	}

	displayPath, path, err := resolveWorkspaceReadTarget(t.workspace, req.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", displayPath)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	sort.Slice(entries, func(i int, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	var b strings.Builder
	fmt.Fprintf(&b, "%s/\n", displayPath)
	written := 0
	truncated := false
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list_dir cancelled: %w", err)
		}
		if written >= maxEntries {
			truncated = true
			break
		}
		name := entry.Name()
		if entry.IsDir() {
			fmt.Fprintf(&b, "  %s/\n", name)
		} else {
			size := ""
			if info, err := entry.Info(); err == nil {
				size = fmt.Sprintf("  %d bytes", info.Size())
			}
			fmt.Fprintf(&b, "  %s%s\n", name, size)
		}
		written++
	}
	if truncated {
		fmt.Fprintf(&b, "[Truncated after %d entries. Narrow path or increase max_entries.]\n",
			maxEntries)
	}

	result := types.TextToolResult(strings.TrimRight(b.String(), "\n"))
	result.Metadata = map[string]string{
		"path":          displayPath,
		"workspace":     t.workspace.Root(),
		"entries":       fmt.Sprintf("%d", len(entries)),
		"entries_shown": fmt.Sprintf("%d", written),
		"truncated":     fmt.Sprintf("%t", truncated),
	}
	return &result, nil
}

func resolveWorkspaceReadTarget(workspace *Workspace, path string) (string, string, error) {
	displayPath := strings.TrimSpace(path)
	if displayPath == "" || displayPath == "." {
		return ".", workspace.Root(), nil
	}

	resolvedPath, err := workspace.resolveReadPath(displayPath)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(workspace.Root(), resolvedPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve display path: %w", err)
	}
	return rel, resolvedPath, nil
}

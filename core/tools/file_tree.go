package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

const (
	defaultFileTreeMaxDepth   = 2
	defaultFileTreeMaxEntries = 100
)

type fileTreeInput struct {
	Path       string `json:"path,omitempty"`
	MaxDepth   int    `json:"max_depth,omitempty"`
	MaxEntries int    `json:"max_entries,omitempty"`
}

type FileTreeTool struct {
	workspace *Workspace
}

func NewFileTreeTool(workspaceRoot string) (*FileTreeTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &FileTreeTool{workspace: workspace}, nil
}

func (t *FileTreeTool) Name() string {
	return "file_tree"
}

func (t *FileTreeTool) Description() string {
	return "Return a bounded recursive tree for a workspace directory."
}

func (t *FileTreeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string",` +
		`"description":"Workspace-relative directory. Defaults to '.'."},` +
		`"max_depth":{"type":"integer","minimum":1,"maximum":10,"description":` +
		`"Maximum depth. Defaults to 2."},"max_entries":{"type":"integer",` +
		`"minimum":1,"maximum":2000,"description":"Maximum entries. Defaults to 100."}},` +
		`"additionalProperties":false}`)
}

func (t *FileTreeTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

func (t *FileTreeTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var req fileTreeInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode file_tree input: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("file_tree cancelled: %w", err)
	}

	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultFileTreeMaxDepth
	}
	if maxDepth > 10 {
		maxDepth = 10
	}
	maxEntries := req.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultFileTreeMaxEntries
	}
	if maxEntries > 2000 {
		maxEntries = 2000
	}

	displayPath, root, err := resolveWorkspaceReadTarget(t.workspace, req.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat tree root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", displayPath)
	}

	builder := fileTreeBuilder{
		workspaceRoot: t.workspace.Root(),
		root:          root,
		maxDepth:      maxDepth,
		maxEntries:    maxEntries,
	}
	text, err := builder.build(ctx, displayPath)
	if err != nil {
		return nil, err
	}

	result := types.TextToolResult(text)
	result.Metadata = map[string]string{
		"path":          displayPath,
		"workspace":     t.workspace.Root(),
		"max_depth":     fmt.Sprintf("%d", maxDepth),
		"entries_shown": fmt.Sprintf("%d", builder.written),
		"truncated":     fmt.Sprintf("%t", builder.truncated),
	}
	return &result, nil
}

type fileTreeBuilder struct {
	workspaceRoot string
	root          string
	maxDepth      int
	maxEntries    int
	written       int
	truncated     bool
	out           strings.Builder
}

func (b *fileTreeBuilder) build(ctx context.Context, displayPath string) (string, error) {
	fmt.Fprintf(&b.out, "%s/\n", displayPath)
	if err := b.walk(ctx, b.root, "", 1); err != nil {
		return "", err
	}
	if b.truncated {
		fmt.Fprintf(&b.out, "[Truncated after %d entries. Narrow path or increase max_entries.]\n",
			b.maxEntries)
	}
	return strings.TrimRight(b.out.String(), "\n"), nil
}

func (b *fileTreeBuilder) walk(ctx context.Context, dir string, indent string, depth int) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("file_tree cancelled: %w", err)
	}
	if depth > b.maxDepth || b.truncated {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read tree directory: %w", err)
	}
	entries = filterTreeEntries(entries)
	for index, entry := range entries {
		if b.written >= b.maxEntries {
			b.truncated = true
			return nil
		}

		last := index == len(entries)-1
		branch := "|-- "
		nextIndent := indent + "|   "
		if last {
			branch = "`-- "
			nextIndent = indent + "    "
		}

		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		fmt.Fprintf(&b.out, "%s%s%s\n", indent, branch, name)
		b.written++

		if entry.IsDir() && !shouldSkipTreeDir(entry.Name()) {
			child := filepath.Join(dir, entry.Name())
			if err := b.walk(ctx, child, nextIndent, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func filterTreeEntries(entries []fs.DirEntry) []fs.DirEntry {
	filtered := entries[:0]
	for _, entry := range entries {
		if entry.IsDir() && shouldSkipTreeDir(entry.Name()) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i int, j int) bool {
		if filtered[i].IsDir() != filtered[j].IsDir() {
			return filtered[i].IsDir()
		}
		return filtered[i].Name() < filtered[j].Name()
	})
	return filtered
}

func shouldSkipTreeDir(name string) bool {
	return shouldSkipGrepDir(name) || name == ".epsilon" || name == ".tmp" ||
		name == "coverage" || name == ".next"
}

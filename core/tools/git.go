package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

const defaultGitOutputMaxBytes = 128 * 1024

type gitStatusInput struct {
	Short bool `json:"short,omitempty"`
}

type gitDiffInput struct {
	Path    string `json:"path,omitempty"`
	Staged  bool   `json:"staged,omitempty"`
	Context int    `json:"context,omitempty"`
}

type GitStatusTool struct {
	workspace *Workspace
	maxBytes  int
}

func NewGitStatusTool(workspaceRoot string) (*GitStatusTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &GitStatusTool{workspace: workspace, maxBytes: defaultGitOutputMaxBytes}, nil
}

func (t *GitStatusTool) Name() string {
	return "git_status"
}

func (t *GitStatusTool) Description() string {
	return "Show read-only git working tree status for the workspace. Use this before edits " +
		"to understand existing user changes."
}

func (t *GitStatusTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"short":{"type":"boolean",` +
		`"description":"Use short porcelain status. Defaults to true."}},` +
		`"additionalProperties":false}`)
}

func (t *GitStatusTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

func (t *GitStatusTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var req gitStatusInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode git_status input: %w", err)
	}
	args := []string{"status", "--short"}
	if inputHasField(input, "short") && !req.Short {
		args = []string{"status"}
	}
	return runGitTool(ctx, t.workspace, t.maxBytes, "git_status", args, nil)
}

type GitDiffTool struct {
	workspace *Workspace
	maxBytes  int
}

func NewGitDiffTool(workspaceRoot string) (*GitDiffTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &GitDiffTool{workspace: workspace, maxBytes: defaultGitOutputMaxBytes}, nil
}

func (t *GitDiffTool) Name() string {
	return "git_diff"
}

func (t *GitDiffTool) Description() string {
	return "Show read-only git diff output for unstaged or staged workspace changes. " +
		"Optionally scope to a workspace-relative path."
}

func (t *GitDiffTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string",` +
		`"description":"Optional workspace-relative file or directory path to diff."},` +
		`"staged":{"type":"boolean","description":"Show staged changes with --cached."},` +
		`"context":{"type":"integer","minimum":0,"maximum":100,"description":"Unified ` +
		`diff context lines. Defaults to git's default."}},"additionalProperties":false}`)
}

func (t *GitDiffTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

func (t *GitDiffTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var req gitDiffInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode git_diff input: %w", err)
	}

	args := []string{"diff"}
	if req.Staged {
		args = append(args, "--cached")
	}
	if req.Context >= 0 && req.Context <= 100 && inputHasField(input, "context") {
		args = append(args, fmt.Sprintf("--unified=%d", req.Context))
	}

	meta := map[string]string{
		"staged": fmt.Sprintf("%t", req.Staged),
	}
	if strings.TrimSpace(req.Path) != "" {
		displayPath, err := resolveWorkspaceGitPath(t.workspace, req.Path)
		if err != nil {
			return nil, err
		}
		args = append(args, "--", displayPath)
		meta["path"] = displayPath
	}

	return runGitTool(ctx, t.workspace, t.maxBytes, "git_diff", args, meta)
}

func resolveWorkspaceGitPath(workspace *Workspace, path string) (string, error) {
	displayPath := strings.TrimSpace(path)
	if displayPath == "" || displayPath == "." {
		return ".", nil
	}
	cleanPath, err := workspace.cleanPath(displayPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(workspace.Root(), cleanPath)
	if err != nil {
		return "", fmt.Errorf("resolve git path: %w", err)
	}
	return rel, nil
}

func runGitTool(ctx context.Context, workspace *Workspace, maxBytes int, name string,
	args []string, metadata map[string]string) (*types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s cancelled: %w", name, err)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workspace.Root()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s cancelled: %w", name, ctx.Err())
	}

	output := strings.TrimRight(stdout.String(), "\n")
	if output == "" && err == nil {
		output = "No output."
	}
	truncated := false
	if len(output) > maxBytes {
		output = output[:maxBytes] + "\n[Truncated: output exceeded byte limit.]"
		truncated = true
	}

	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		result := types.ErrorToolResult(msg)
		result.Metadata = gitMetadata(workspace, args, metadata, false)
		return &result, nil
	}

	result := types.TextToolResult(output)
	result.Metadata = gitMetadata(workspace, args, metadata, truncated)
	return &result, nil
}

func gitMetadata(workspace *Workspace, args []string, metadata map[string]string,
	truncated bool) map[string]string {
	result := map[string]string{
		"workspace": workspace.Root(),
		"command":   "git " + strings.Join(args, " "),
		"truncated": fmt.Sprintf("%t", truncated),
	}
	for key, value := range metadata {
		result[key] = value
	}
	return result
}

func inputHasField(input json.RawMessage, field string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		return false
	}
	_, ok := raw[field]
	return ok
}

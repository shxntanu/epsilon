package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

const (
	defaultRipgrepMaxBytes  = 32 * 1024
	defaultRipgrepTimeoutMs = 30 * 1000
	maxRipgrepTimeoutMs     = 2 * 60 * 1000
	defaultRipgrepMatches   = 50
	maxRipgrepMatches       = 200
	maxRipgrepContext       = 10
	maxRipgrepColumns       = 200
)

type ripgrepInput struct {
	Pattern       string `json:"pattern"`
	Path          string `json:"path,omitempty"`
	Glob          string `json:"glob,omitempty"`
	Literal       bool   `json:"literal,omitempty"`
	IgnoreCase    bool   `json:"ignore_case,omitempty"`
	IncludeHidden bool   `json:"include_hidden,omitempty"`
	ContextLines  int    `json:"context_lines,omitempty"`
	MaxMatches    int    `json:"max_matches,omitempty"`
	TimeoutMs     int    `json:"timeout_ms,omitempty"`
}

// RipgrepTool searches workspace files with rg.
type RipgrepTool struct {
	workspace *Workspace
	maxBytes  int
}

func NewRipgrepTool(workspaceRoot string) (*RipgrepTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &RipgrepTool{
		workspace: workspace,
		maxBytes:  defaultRipgrepMaxBytes,
	}, nil
}

func (t *RipgrepTool) Name() string {
	return "ripgrep"
}

func (t *RipgrepTool) Description() string {
	return "Fast workspace search using rg with bounded output."
}

func (t *RipgrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string",` +
		`"description":"Regex pattern, or literal text when literal=true."},"path":{` +
		`"type":"string","description":"Workspace-relative file or directory. Defaults to '.'."},` +
		`"glob":{"type":"string","description":"Optional rg glob, e.g. '*.go'."},` +
		`"literal":{"type":"boolean","description":"Search pattern as literal text."},` +
		`"ignore_case":{"type":"boolean","description":"Case-insensitive search."},` +
		`"include_hidden":{"type":"boolean","description":"Include hidden files and dirs."},` +
		`"context_lines":{"type":"integer","minimum":0,"maximum":10,"description":"Context lines around matches."},` +
		`"max_matches":{"type":"integer","minimum":1,"maximum":200,"description":"Matches per file. Defaults to 50."},` +
		`"timeout_ms":{"type":"integer","minimum":1,"maximum":120000,"description":"Timeout in ms. Defaults to 30000."}},` +
		`"required":["pattern"],"additionalProperties":false}`)
}

func (t *RipgrepTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

func (t *RipgrepTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var req ripgrepInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode ripgrep input: %w", err)
	}
	pattern := strings.TrimSpace(req.Pattern)
	if pattern == "" {
		return nil, fmt.Errorf("pattern cannot be empty")
	}

	rgPath, err := exec.LookPath("rg")
	if err != nil {
		result := types.ErrorToolResult("ripgrep executable `rg` was not found in PATH")
		result.Metadata = map[string]string{"missing": "rg"}
		return &result, nil
	}

	displayPath, targetPath, err := resolveWorkspaceReadTarget(t.workspace, req.Path)
	if err != nil {
		return nil, err
	}

	maxMatches := req.MaxMatches
	if maxMatches <= 0 {
		maxMatches = defaultRipgrepMatches
	}
	if maxMatches > maxRipgrepMatches {
		maxMatches = maxRipgrepMatches
	}
	contextLines := req.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > maxRipgrepContext {
		contextLines = maxRipgrepContext
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultRipgrepTimeoutMs
	}
	if timeoutMs > maxRipgrepTimeoutMs {
		timeoutMs = maxRipgrepTimeoutMs
	}

	args := []string{
		"--color=never",
		"--line-number",
		"--with-filename",
		"--max-columns", fmt.Sprintf("%d", maxRipgrepColumns),
		"--max-columns-preview",
		"--max-count", fmt.Sprintf("%d", maxMatches),
	}
	if contextLines > 0 {
		args = append(args, "--context", fmt.Sprintf("%d", contextLines))
	}
	if req.Literal {
		args = append(args, "--fixed-strings")
	}
	if req.IgnoreCase {
		args = append(args, "--ignore-case")
	}
	if req.IncludeHidden {
		args = append(args, "--hidden")
	}
	if strings.TrimSpace(req.Glob) != "" {
		args = append(args, "--glob", strings.TrimSpace(req.Glob))
	}
	args = append(args, "--", pattern, targetPath)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(runCtx, rgPath, args...)
	cmd.Dir = t.workspace.Root()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)
	timedOut := runCtx.Err() == context.DeadlineExceeded
	if ctx.Err() != nil && !timedOut {
		return nil, fmt.Errorf("ripgrep cancelled: %w", ctx.Err())
	}

	exitCode := ripgrepExitCode(err, timedOut)
	output, outputTruncated := truncateRipgrepOutput(stdout.String(), t.maxBytes)
	errorOutput, stderrTruncated := truncateRipgrepOutput(stderr.String(), t.maxBytes)

	result := types.TextToolResult(formatRipgrepOutput(output, errorOutput, exitCode, timedOut))
	if timedOut || exitCode > 1 {
		result.IsError = true
	}
	result.Metadata = map[string]string{
		"path":             displayPath,
		"pattern":          pattern,
		"exit_code":        fmt.Sprintf("%d", exitCode),
		"timed_out":        fmt.Sprintf("%t", timedOut),
		"timeout_ms":       fmt.Sprintf("%d", timeoutMs),
		"duration_ms":      fmt.Sprintf("%d", duration.Milliseconds()),
		"max_matches":      fmt.Sprintf("%d", maxMatches),
		"context_lines":    fmt.Sprintf("%d", contextLines),
		"stdout_truncated": fmt.Sprintf("%t", outputTruncated),
		"stderr_truncated": fmt.Sprintf("%t", stderrTruncated),
		"workspace":        t.workspace.Root(),
	}
	if strings.TrimSpace(req.Glob) != "" {
		result.Metadata["glob"] = strings.TrimSpace(req.Glob)
	}
	return &result, nil
}

func formatRipgrepOutput(stdout string, stderr string, exitCode int, timedOut bool) string {
	stdout = strings.TrimRight(stdout, "\n")
	stderr = strings.TrimRight(stderr, "\n")
	if exitCode == 1 && stdout == "" && stderr == "" {
		return "No matches found."
	}

	var b strings.Builder
	if stdout != "" {
		b.WriteString(stdout)
	}
	if stderr != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(stderr)
	}
	if timedOut {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("ripgrep timed out.")
	} else if exitCode > 1 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "ripgrep exited with code %d.", exitCode)
	}
	if b.Len() == 0 {
		return "No output."
	}
	return b.String()
}

func truncateRipgrepOutput(text string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text, false
	}
	if maxBytes < 64 {
		return text[:maxBytes], true
	}

	half := maxBytes / 2
	omitted := text[half : len(text)-half]
	return fmt.Sprintf("%s\n[truncated %d lines]\n%s",
		text[:half], countLines(omitted), text[len(text)-half:]), true
}

func ripgrepExitCode(err error, timedOut bool) int {
	if timedOut {
		return 124
	}
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 2
}

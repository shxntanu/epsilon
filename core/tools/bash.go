package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

const (
	defaultBashTimeoutMs = 60 * 1000
	maxBashTimeoutMs     = 10 * 60 * 1000
	defaultBashMaxBytes  = 32 * 1024
)

type bashInput struct {
	Command   string `json:"command"`
	Timeout   int    `json:"timeout,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// BashTool runs shell commands in the workspace.
type BashTool struct {
	workspace *Workspace
	maxBytes  int
}

// NewBashTool returns a bash tool scoped to workspaceRoot.
func NewBashTool(workspaceRoot string) (*BashTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &BashTool{
		workspace: workspace,
		maxBytes:  defaultBashMaxBytes,
	}, nil
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return fmt.Sprintf(`Run a bash command in the workspace with bounded output.

Security and correctness rules:
- Use dedicated tools for workspace inspection when possible: list_dir/file_tree, grep_search, read_file, git_status, and git_diff.
- Use bash for tests, builds, package-manager commands, and user-requested shell commands.
- Before creating files or directories, verify the parent location with a dedicated read/list tool.
- Quote paths and arguments carefully. Commands are parsed before execution and only simple commands joined by |, &&, ||, or ; are allowed.
- Do not use interactive commands or commands that require an editor; GIT_EDITOR is set to true.
- Read-only network exploration is allowed with curl/wget/httpie-style clients, but request bodies, file downloads, uploads, raw sockets, browsers, redirection, variable expansion, command substitution, and subshells are blocked.
- Absolute filesystem paths and .. traversal arguments are blocked; keep file access inside the workspace.
- Avoid changing directories; commands already run from the workspace root. If needed, prefer scoped commands such as "go test ./core/tools".
- Commands are not persistent: environment changes and cd do not carry to later bash calls.
- Output is truncated after %d bytes for stdout and stderr independently.
- timeout is optional, uses milliseconds, defaults to %d, and is capped at %d.`,
		defaultBashMaxBytes, defaultBashTimeoutMs, maxBashTimeoutMs)
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string",` +
		`"description":"Command to run with bash -lc."},"timeout":{"type":"integer",` +
		`"minimum":1,"maximum":600000,"description":"Timeout in milliseconds. Defaults to 60000."}},` +
		`"required":["command"],"additionalProperties":false}`)
}

func (t *BashTool) Permission() types.PermissionMode {
	return types.PermissionAsk
}

func (t *BashTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var req bashInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode bash input: %w", err)
	}

	command := strings.TrimSpace(req.Command)
	if command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}
	if strings.ContainsRune(command, 0) {
		return nil, fmt.Errorf("command cannot contain NUL bytes")
	}
	policy := classifyShellCommand(command)
	if !policy.Allowed {
		result := types.ErrorToolResult("command is not allowed: " + policy.Reason)
		result.Metadata = map[string]string{
			"workspace": t.workspace.Root(),
			"command":   command,
			"blocked":   "true",
			"reason":    policy.Reason,
			"bases":     strings.Join(policy.Bases, ","),
		}
		return &result, nil
	}

	timeoutMs := req.Timeout
	if timeoutMs <= 0 {
		timeoutMs = req.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = defaultBashTimeoutMs
	}
	if timeoutMs > maxBashTimeoutMs {
		timeoutMs = maxBashTimeoutMs
	}

	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "-lc", command)
	cmd.Dir = t.workspace.Root()
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	end := time.Now()
	timedOut := runCtx.Err() == context.DeadlineExceeded
	if ctx.Err() != nil && !timedOut {
		return nil, fmt.Errorf("bash cancelled: %w", ctx.Err())
	}

	stdoutText, stdoutTruncated := truncateBashOutput(stdout.String(), t.maxBytes)
	stderrText, stderrTruncated := truncateBashOutput(stderr.String(), t.maxBytes)
	exitCode := commandExitCode(err, timedOut)
	output := formatBashOutput(stdoutText, stderrText, exitCode, timedOut)

	result := types.TextToolResult(output)
	if err != nil || timedOut {
		result.IsError = true
	}
	result.Metadata = map[string]string{
		"workspace":        t.workspace.Root(),
		"command":          command,
		"exit_code":        fmt.Sprintf("%d", exitCode),
		"timed_out":        fmt.Sprintf("%t", timedOut),
		"timeout_ms":       fmt.Sprintf("%d", timeoutMs),
		"duration_ms":      fmt.Sprintf("%d", end.Sub(start).Milliseconds()),
		"stdout_truncated": fmt.Sprintf("%t", stdoutTruncated),
		"stderr_truncated": fmt.Sprintf("%t", stderrTruncated),
		"network":          fmt.Sprintf("%t", policy.Network),
		"bases":            strings.Join(policy.Bases, ","),
	}
	return &result, nil
}

func formatBashOutput(stdout string, stderr string, exitCode int, timedOut bool) string {
	var b strings.Builder
	stdout = strings.TrimRight(stdout, "\n")
	stderr = strings.TrimRight(stderr, "\n")
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
		b.WriteString("Command timed out.")
	} else if exitCode != 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Exit code %d.", exitCode)
	}
	if b.Len() == 0 {
		return "No output."
	}
	return b.String()
}

func truncateBashOutput(text string, maxBytes int) (string, bool) {
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

func commandExitCode(err error, timedOut bool) int {
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
	return 1
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

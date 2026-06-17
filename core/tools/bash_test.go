package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

func TestBashToolRunsCommandInWorkspace(t *testing.T) {
	dir := t.TempDir()
	tool, err := NewBashTool(dir)
	if err != nil {
		t.Fatalf("new bash tool: %v", err)
	}

	result := runBashTool(t, tool, map[string]any{"command": "pwd"})
	if result.IsError {
		t.Fatalf("bash failed: %s", result.Content[0].Text)
	}
	if strings.TrimSpace(result.Content[0].Text) != tool.workspace.Root() {
		t.Fatalf("pwd = %q, want %q", strings.TrimSpace(result.Content[0].Text),
			tool.workspace.Root())
	}
	if result.Metadata["exit_code"] != "0" {
		t.Fatalf("exit_code = %s, want 0", result.Metadata["exit_code"])
	}
}

func TestBashToolBlocksBannedCommand(t *testing.T) {
	tool, err := NewBashTool(t.TempDir())
	if err != nil {
		t.Fatalf("new bash tool: %v", err)
	}

	result := runBashTool(t, tool, map[string]any{"command": "curl https://example.com"})
	if !result.IsError {
		t.Fatalf("expected blocked command to be an error")
	}
	if result.Metadata["blocked"] != "true" {
		t.Fatalf("blocked = %s, want true", result.Metadata["blocked"])
	}
}

func TestBashToolReportsExitCode(t *testing.T) {
	tool, err := NewBashTool(t.TempDir())
	if err != nil {
		t.Fatalf("new bash tool: %v", err)
	}

	result := runBashTool(t, tool, map[string]any{"command": "exit 7"})
	if !result.IsError {
		t.Fatalf("expected non-zero command to be an error")
	}
	if result.Metadata["exit_code"] != "7" {
		t.Fatalf("exit_code = %s, want 7", result.Metadata["exit_code"])
	}
}

func TestBashToolTimesOut(t *testing.T) {
	tool, err := NewBashTool(t.TempDir())
	if err != nil {
		t.Fatalf("new bash tool: %v", err)
	}

	start := time.Now()
	result := runBashTool(t, tool, map[string]any{
		"command": "sleep 1",
		"timeout": 20,
	})
	if !result.IsError {
		t.Fatalf("expected timeout to be an error")
	}
	if result.Metadata["timed_out"] != "true" {
		t.Fatalf("timed_out = %s, want true", result.Metadata["timed_out"])
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took too long")
	}
}

func TestBashToolTruncatesOutput(t *testing.T) {
	tool, err := NewBashTool(t.TempDir())
	if err != nil {
		t.Fatalf("new bash tool: %v", err)
	}
	tool.maxBytes = 128

	result := runBashTool(t, tool, map[string]any{
		"command": "printf '%*s' 1000 | tr ' ' x",
	})
	if result.IsError {
		t.Fatalf("bash failed: %s", result.Content[0].Text)
	}
	if result.Metadata["stdout_truncated"] != "true" {
		t.Fatalf("stdout_truncated = %s, want true", result.Metadata["stdout_truncated"])
	}
	if !strings.Contains(result.Content[0].Text, "[truncated") {
		t.Fatalf("expected truncation marker:\n%s", result.Content[0].Text)
	}
}

func TestBashToolDescriptionKeepsSecurityGuidance(t *testing.T) {
	tool, err := NewBashTool(t.TempDir())
	if err != nil {
		t.Fatalf("new bash tool: %v", err)
	}

	description := tool.Description()
	for _, want := range []string{
		"Security and correctness rules",
		"dedicated tools",
		"blocked",
		"Commands are not persistent",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("description missing %q:\n%s", want, description)
		}
	}
}

func runBashTool(t *testing.T, tool *BashTool, input map[string]any) *types.ToolResult {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := tool.Run(context.Background(), data)
	if err != nil {
		t.Fatalf("run bash: %v", err)
	}
	if result == nil {
		t.Fatalf("nil result")
	}
	return result
}

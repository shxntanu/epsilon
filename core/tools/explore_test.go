package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestListDirToolListsImmediateChildren(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	tool, err := NewListDirTool(dir)
	if err != nil {
		t.Fatalf("new list_dir tool: %v", err)
	}
	result := runTool(t, tool, map[string]any{"path": "."})

	for _, want := range []string{"./", "src/", "README.md"} {
		if !strings.Contains(result.Content[0].Text, want) {
			t.Fatalf("list_dir output missing %q:\n%s", want, result.Content[0].Text)
		}
	}
}

func TestFileTreeToolReturnsBoundedTree(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{"cmd/app", "node_modules/pkg"} {
		if err := os.MkdirAll(filepath.Join(dir, path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "app", "main.go"),
		[]byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"),
		[]byte("module.exports = {}\n"), 0o644); err != nil {
		t.Fatalf("write dependency: %v", err)
	}

	tool, err := NewFileTreeTool(dir)
	if err != nil {
		t.Fatalf("new file_tree tool: %v", err)
	}
	result := runTool(t, tool, map[string]any{"path": ".", "max_depth": 3})
	text := result.Content[0].Text

	if !strings.Contains(text, "cmd/") || !strings.Contains(text, "main.go") {
		t.Fatalf("file_tree output missing source files:\n%s", text)
	}
	if strings.Contains(text, "node_modules") {
		t.Fatalf("file_tree output included skipped directory:\n%s", text)
	}
}

func TestGitToolsReportStatusAndDiff(t *testing.T) {
	dir := t.TempDir()
	runGitCommand(t, dir, "init")
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	runGitCommand(t, dir, "add", "sample.txt")

	statusTool, err := NewGitStatusTool(dir)
	if err != nil {
		t.Fatalf("new git_status tool: %v", err)
	}
	status := runTool(t, statusTool, map[string]any{})
	if !strings.Contains(status.Content[0].Text, "A  sample.txt") {
		t.Fatalf("git_status output missing staged file:\n%s", status.Content[0].Text)
	}

	diffTool, err := NewGitDiffTool(dir)
	if err != nil {
		t.Fatalf("new git_diff tool: %v", err)
	}
	diff := runTool(t, diffTool, map[string]any{"staged": true, "path": "sample.txt"})
	for _, want := range []string{"diff --git", "sample.txt", "+hello"} {
		if !strings.Contains(diff.Content[0].Text, want) {
			t.Fatalf("git_diff output missing %q:\n%s", want, diff.Content[0].Text)
		}
	}
}

func TestToolDefinitionsHaveValidCompactSchemas(t *testing.T) {
	dir := t.TempDir()
	factories := []func(string) (Tool, error){
		func(root string) (Tool, error) { return NewReadFileTool(root) },
		func(root string) (Tool, error) { return NewWriteFileTool(root) },
		func(root string) (Tool, error) { return NewGrepTool(root) },
		func(root string) (Tool, error) { return NewListDirTool(root) },
		func(root string) (Tool, error) { return NewFileTreeTool(root) },
		func(root string) (Tool, error) { return NewGitStatusTool(root) },
		func(root string) (Tool, error) { return NewGitDiffTool(root) },
		func(root string) (Tool, error) { return NewPatchTool(root) },
	}

	for _, factory := range factories {
		tool, err := factory(dir)
		if err != nil {
			t.Fatalf("create tool: %v", err)
		}
		if !json.Valid(tool.InputSchema()) {
			t.Fatalf("%s schema is invalid JSON: %s", tool.Name(), tool.InputSchema())
		}
		if len(tool.Description()) > 100 {
			t.Fatalf("%s description too long: %q", tool.Name(), tool.Description())
		}
	}
}

func TestGrepToolCapsMatchesAndOutput(t *testing.T) {
	dir := t.TempDir()
	var content strings.Builder
	for i := 0; i < 80; i++ {
		content.WriteString("needle line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "many.txt"), []byte(content.String()), 0o644); err != nil {
		t.Fatalf("write many: %v", err)
	}

	tool, err := NewGrepTool(dir)
	if err != nil {
		t.Fatalf("new grep tool: %v", err)
	}
	result := runTool(t, tool, map[string]any{"pattern": "needle", "path": "many.txt"})

	if result.Metadata["written_matches"] != "50" {
		t.Fatalf("written_matches = %s, want 50", result.Metadata["written_matches"])
	}
	if result.Metadata["truncated"] != "true" {
		t.Fatalf("truncated = %s, want true", result.Metadata["truncated"])
	}
	if len(result.Content[0].Text) > maxGrepWrittenBytes+200 {
		t.Fatalf("grep output too large: %d bytes", len(result.Content[0].Text))
	}
}

func TestGrepToolTruncatesLongLines(t *testing.T) {
	dir := t.TempDir()
	longLine := "needle " + strings.Repeat("x", 400)
	if err := os.WriteFile(filepath.Join(dir, "long.txt"), []byte(longLine+"\n"), 0o644); err != nil {
		t.Fatalf("write long: %v", err)
	}

	tool, err := NewGrepTool(dir)
	if err != nil {
		t.Fatalf("new grep tool: %v", err)
	}
	result := runTool(t, tool, map[string]any{"pattern": "needle", "path": "long.txt"})

	if !strings.Contains(result.Content[0].Text, "[line truncated]") {
		t.Fatalf("expected truncated line marker:\n%s", result.Content[0].Text)
	}
	if strings.Contains(result.Content[0].Text, strings.Repeat("x", 300)) {
		t.Fatalf("grep output included too much of the long line:\n%s", result.Content[0].Text)
	}
}

func runTool(t *testing.T, tool Tool, input map[string]any) *types.ToolResult {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := tool.Run(context.Background(), data)
	if err != nil {
		t.Fatalf("run %s: %v", tool.Name(), err)
	}
	if result == nil {
		t.Fatalf("%s returned nil result", tool.Name())
	}
	if result.IsError {
		t.Fatalf("%s returned error: %s", tool.Name(), result.Content[0].Text)
	}
	return result
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

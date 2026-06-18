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
		func(root string) (Tool, error) { return NewRipgrepTool(root) },
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

func TestRipgrepToolFindsMatches(t *testing.T) {
	requireRipgrep(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"),
		[]byte("package sample\n\nfunc Target() {}\n"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"),
		[]byte("Target in text\n"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}

	tool, err := NewRipgrepTool(dir)
	if err != nil {
		t.Fatalf("new ripgrep tool: %v", err)
	}
	result := runTool(t, tool, map[string]any{
		"pattern": "Target",
		"glob":    "*.go",
	})

	text := result.Content[0].Text
	if !strings.Contains(text, "sample.go") || !strings.Contains(text, "func Target") {
		t.Fatalf("ripgrep output missing Go match:\n%s", text)
	}
	if strings.Contains(text, "sample.txt") {
		t.Fatalf("ripgrep output ignored glob:\n%s", text)
	}
	if result.Metadata["exit_code"] != "0" {
		t.Fatalf("exit_code = %s, want 0", result.Metadata["exit_code"])
	}
}

func TestRipgrepToolNoMatchesIsNotError(t *testing.T) {
	requireRipgrep(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	tool, err := NewRipgrepTool(dir)
	if err != nil {
		t.Fatalf("new ripgrep tool: %v", err)
	}
	result, err := tool.Run(context.Background(), mustJSON(t, map[string]any{
		"pattern": "missing",
	}))
	if err != nil {
		t.Fatalf("run ripgrep: %v", err)
	}
	if result.IsError {
		t.Fatalf("no-match ripgrep result should not be an error: %s", result.Content[0].Text)
	}
	if result.Content[0].Text != "No matches found." {
		t.Fatalf("content = %q, want no matches", result.Content[0].Text)
	}
	if result.Metadata["exit_code"] != "1" {
		t.Fatalf("exit_code = %s, want 1", result.Metadata["exit_code"])
	}
}

func TestRipgrepToolTruncatesOutput(t *testing.T) {
	requireRipgrep(t)

	dir := t.TempDir()
	var content strings.Builder
	for i := 0; i < 200; i++ {
		content.WriteString("needle ")
		content.WriteString(strings.Repeat("x", 80))
		content.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "many.txt"), []byte(content.String()), 0o644); err != nil {
		t.Fatalf("write many: %v", err)
	}

	tool, err := NewRipgrepTool(dir)
	if err != nil {
		t.Fatalf("new ripgrep tool: %v", err)
	}
	tool.maxBytes = 256
	result := runTool(t, tool, map[string]any{"pattern": "needle"})

	if result.Metadata["stdout_truncated"] != "true" {
		t.Fatalf("stdout_truncated = %s, want true", result.Metadata["stdout_truncated"])
	}
	if !strings.Contains(result.Content[0].Text, "[truncated") {
		t.Fatalf("expected truncation marker:\n%s", result.Content[0].Text)
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
	data := mustJSON(t, input)
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

func mustJSON(t *testing.T, input map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return data
}

func requireRipgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
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

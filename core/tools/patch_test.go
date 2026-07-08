package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchToolAppliesCodexPatchMultipleOperations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "modify.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write modify: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "delete.txt"), []byte("obsolete\n"), 0o644); err != nil {
		t.Fatalf("write delete: %v", err)
	}

	tool, err := NewPatchTool(dir)
	if err != nil {
		t.Fatalf("new patch tool: %v", err)
	}
	patch := `*** Begin Patch
*** Add File: nested/new.txt
+created
*** Update File: modify.txt
@@
 alpha
-beta
+BETA
 gamma
*** Delete File: delete.txt
*** End Patch`

	result, err := tool.Run(context.Background(), mustPatchJSON(t, patch))
	if err != nil {
		t.Fatalf("run patch: %v", err)
	}
	if result.IsError {
		t.Fatalf("patch failed: %s", result.Content[0].Text)
	}
	assertFileContent(t, filepath.Join(dir, "nested", "new.txt"), "created\n")
	assertFileContent(t, filepath.Join(dir, "modify.txt"), "alpha\nBETA\ngamma\n")
	if _, err := os.Stat(filepath.Join(dir, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("delete.txt still exists or stat failed: %v", err)
	}
	for _, want := range []string{"nested/new.txt", "modify.txt", "delete.txt"} {
		if !strings.Contains(result.Metadata["modified_files"], want) {
			t.Fatalf("modified_files missing %q: %q", want, result.Metadata["modified_files"])
		}
	}
}

func TestPatchToolCodexPatchMoveOnlyPreservesContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "old"), 0o755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old", "name.txt"), []byte("no final newline"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}

	tool, err := NewPatchTool(dir)
	if err != nil {
		t.Fatalf("new patch tool: %v", err)
	}
	patch := `*** Begin Patch
*** Update File: old/name.txt
*** Move to: renamed/name.txt
*** End Patch`

	result, err := tool.Run(context.Background(), mustPatchJSON(t, patch))
	if err != nil {
		t.Fatalf("run patch: %v", err)
	}
	if result.IsError {
		t.Fatalf("patch failed: %s", result.Content[0].Text)
	}
	assertFileContent(t, filepath.Join(dir, "renamed", "name.txt"), "no final newline")
	if _, err := os.Stat(filepath.Join(dir, "old", "name.txt")); !os.IsNotExist(err) {
		t.Fatalf("old file still exists or stat failed: %v", err)
	}
}

func TestPatchToolCodexPatchReportsFailureWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "modify.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write modify: %v", err)
	}

	tool, err := NewPatchTool(dir)
	if err != nil {
		t.Fatalf("new patch tool: %v", err)
	}
	patch := `*** Begin Patch
*** Update File: modify.txt
@@
-missing
+changed
*** End Patch`

	result, err := tool.Run(context.Background(), mustPatchJSON(t, patch))
	if err != nil {
		t.Fatalf("run patch: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected patch failure")
	}
	if result.Metadata["status"] != "failed" {
		t.Fatalf("status = %q, want failed", result.Metadata["status"])
	}
	if diff := result.Metadata["diff"]; diff != "" {
		t.Fatalf("failed patch reported committed diff:\n%s", diff)
	}
	assertFileContent(t, path, "alpha\nbeta\n")
}

func mustPatchJSON(t *testing.T, patch string) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	return data
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}

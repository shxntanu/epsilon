package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestPatchToolAppliesUnifiedInsertionAtCorrectLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	tool, err := NewPatchTool(dir)
	if err != nil {
		t.Fatalf("new patch tool: %v", err)
	}

	result := runPatchTool(t, tool, `--- a/sample.txt
+++ b/sample.txt
@@ -2,0 +3,1 @@
+inserted
`)
	if result.IsError {
		t.Fatalf("patch failed: %s", result.Content[0].Text)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	want := "one\ntwo\ninserted\nthree\n"
	if string(got) != want {
		t.Fatalf("unexpected content:\nwant %q\n got %q", want, string(got))
	}
}

func TestPatchToolAppliesAgentUpdatePatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	tool, err := NewPatchTool(dir)
	if err != nil {
		t.Fatalf("new patch tool: %v", err)
	}

	result := runPatchTool(t, tool, `*** Begin Patch
*** Update File: sample.txt
@@
 alpha
-beta
+bravo
 gamma
*** End Patch
`)
	if result.IsError {
		t.Fatalf("patch failed: %s", result.Content[0].Text)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	want := "alpha\nbravo\ngamma\n"
	if string(got) != want {
		t.Fatalf("unexpected content:\nwant %q\n got %q", want, string(got))
	}
	if result.Metadata["diff"] == "" {
		t.Fatalf("expected patch diff metadata")
	}
}

func TestPatchToolAppliesAgentAddAndDeletePatch(t *testing.T) {
	dir := t.TempDir()
	deletePath := filepath.Join(dir, "delete-me.txt")
	if err := os.WriteFile(deletePath, []byte("gone\n"), 0o644); err != nil {
		t.Fatalf("write delete target: %v", err)
	}

	tool, err := NewPatchTool(dir)
	if err != nil {
		t.Fatalf("new patch tool: %v", err)
	}

	result := runPatchTool(t, tool, `*** Begin Patch
*** Add File: nested/new.txt
+hello
+world
*** Delete File: delete-me.txt
*** End Patch
`)
	if result.IsError {
		t.Fatalf("patch failed: %s", result.Content[0].Text)
	}

	added, err := os.ReadFile(filepath.Join(dir, "nested", "new.txt"))
	if err != nil {
		t.Fatalf("read added file: %v", err)
	}
	if string(added) != "hello\nworld\n" {
		t.Fatalf("unexpected added content: %q", string(added))
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("delete target still exists or stat failed: %v", err)
	}
}

func TestPatchToolBoundsLargeDiffMetadata(t *testing.T) {
	dir := t.TempDir()
	tool, err := NewPatchTool(dir)
	if err != nil {
		t.Fatalf("new patch tool: %v", err)
	}

	var patch strings.Builder
	patch.WriteString("*** Begin Patch\n")
	patch.WriteString("*** Add File: large.txt\n")
	for i := 0; i < 2000; i++ {
		patch.WriteString("+0123456789abcdef\n")
	}
	patch.WriteString("*** End Patch\n")

	result := runPatchTool(t, tool, patch.String())
	if result.IsError {
		t.Fatalf("patch failed: %s", result.Content[0].Text)
	}
	diff := result.Metadata["diff"]
	if len(diff) > defaultPatchDiffMaxBytes {
		t.Fatalf("diff metadata too large: %d", len(diff))
	}
	if !strings.Contains(diff, "diff omitted") {
		t.Fatalf("expected omitted diff marker, got:\n%s", diff)
	}
}

func runPatchTool(t *testing.T, tool *PatchTool, patch string) *types.ToolResult {
	t.Helper()

	input, err := json.Marshal(PatchInput{Patch: patch})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := tool.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("run patch tool: %v", err)
	}
	if result == nil {
		t.Fatalf("nil patch result")
	}
	return result
}

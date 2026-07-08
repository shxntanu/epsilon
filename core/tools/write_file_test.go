package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileToolIncludesDiffMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	tool, err := NewWriteFileTool(dir)
	if err != nil {
		t.Fatalf("new write file tool: %v", err)
	}

	input, err := json.Marshal(map[string]any{
		"path":      "sample.txt",
		"content":   "new\n",
		"overwrite": true,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	result, err := tool.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("run write file: %v", err)
	}
	diff := result.Metadata["diff"]
	for _, want := range []string{"--- a/sample.txt", "+++ b/sample.txt", "-old", "+new"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("expected diff to contain %q, got:\n%s", want, diff)
		}
	}
}

func TestWriteFileToolBoundsLargeDiffMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("old\n", 6000)), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	tool, err := NewWriteFileTool(dir)
	if err != nil {
		t.Fatalf("new write file: %v", err)
	}

	input, err := json.Marshal(map[string]any{
		"path":      "large.txt",
		"content":   strings.Repeat("new\n", 6000),
		"overwrite": true,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	result, err := tool.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("run write file: %v", err)
	}
	diff := result.Metadata["diff"]
	if len(diff) > defaultWriteFileDiffMaxBytes {
		t.Fatalf("diff metadata too large: %d", len(diff))
	}
	for _, want := range []string{
		"diff truncated",
		"-old",
		"+new",
	} {
		if !strings.Contains(diff, want) {
			t.Fatalf("expected bounded diff to contain %q, got:\n%s", want, diff)
		}
	}
	if result.Metadata["diff_truncated"] != "true" {
		t.Fatalf("diff_truncated = %q, want true", result.Metadata["diff_truncated"])
	}
}

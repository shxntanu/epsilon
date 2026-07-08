package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileReturnsFullSmallFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "small.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewReadFileTool(root)
	if err != nil {
		t.Fatalf("new read_file: %v", err)
	}

	result, err := tool.Run(context.Background(), json.RawMessage(`{"path":"small.txt"}`))
	if err != nil {
		t.Fatalf("run read_file: %v", err)
	}
	if got := result.Content[0].Text; got != "one\ntwo\n" {
		t.Fatalf("content = %q, want exact small file", got)
	}
	if result.Metadata["preview"] != "false" {
		t.Fatalf("preview metadata = %q, want false", result.Metadata["preview"])
	}
	if result.Metadata["truncated"] != "false" {
		t.Fatalf("truncated metadata = %q, want false", result.Metadata["truncated"])
	}
}

func TestReadFileLargeFileReturnsBoundedPreview(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 2000; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", 48))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewReadFileTool(root)
	if err != nil {
		t.Fatalf("new read_file: %v", err)
	}

	result, err := tool.Run(context.Background(), json.RawMessage(`{"path":"large.txt"}`))
	if err != nil {
		t.Fatalf("run read_file: %v", err)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "[read_file preview:") {
		t.Fatalf("preview header missing from %q", text[:min(len(text), 120)])
	}
	if !strings.Contains(text, "[... omitted ") {
		t.Fatalf("omission marker missing from preview")
	}
	if len(text) > defaultReadFilePreviewMaxBytes+512 {
		t.Fatalf("preview too large: %d bytes", len(text))
	}
	if result.Metadata["preview"] != "true" {
		t.Fatalf("preview metadata = %q, want true", result.Metadata["preview"])
	}
	if result.Metadata["truncated"] != "true" {
		t.Fatalf("truncated metadata = %q, want true", result.Metadata["truncated"])
	}
	if result.Metadata["total_lines"] != "2000" {
		t.Fatalf("total_lines metadata = %q, want 2000", result.Metadata["total_lines"])
	}
}

func TestReadFileLineRangeStillReturnsExactLines(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "range.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewReadFileTool(root)
	if err != nil {
		t.Fatalf("new read_file: %v", err)
	}

	result, err := tool.Run(context.Background(), json.RawMessage(
		`{"path":"range.txt","start_line":2,"end_line":3}`,
	))
	if err != nil {
		t.Fatalf("run read_file: %v", err)
	}
	if got := result.Content[0].Text; got != "two\nthree\n" {
		t.Fatalf("content = %q, want requested range", got)
	}
	if result.Metadata["preview"] != "false" {
		t.Fatalf("preview metadata = %q, want false", result.Metadata["preview"])
	}
	if result.Metadata["lines_returned"] != "2" {
		t.Fatalf("lines_returned metadata = %q, want 2", result.Metadata["lines_returned"])
	}
}

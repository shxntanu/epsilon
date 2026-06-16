package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/shxntanu/epsilon/core/types"
)

const defaultReadFileMaxBytes = 128 * 1024

type readFileInput struct {
	Path      string `json:"path"`
	StartLine *int   `json:"start_line,omitempty"`
	EndLine   *int   `json:"end_line,omitempty"`
}

type readFileLineRange struct {
	enabled bool
	start   int
	end     int
}

type readFileData struct {
	content       string
	bytes         int
	truncated     bool
	rangeEnabled  bool
	startLine     int
	endLine       int
	linesReturned int
}

// ReadFileTool reads UTF-8 text from files inside a workspace.
type ReadFileTool struct {
	workspace *Workspace
	maxBytes  int64
}

// NewReadFileTool returns a read_file tool scoped to workspaceRoot.
func NewReadFileTool(workspaceRoot string) (*ReadFileTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}

	return &ReadFileTool{
		workspace: workspace,
		maxBytes:  defaultReadFileMaxBytes,
	}, nil
}

// Name returns the tool name exposed to the model.
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Description returns the tool description exposed to the model.
func (t *ReadFileTool) Description() string {
	return "Read a text file from the workspace using a relative path. Optionally read an " +
		"inclusive 1-based line range with start_line and end_line. Results are truncated " +
		"when they exceed the configured byte limit."
}

// InputSchema returns the tool input schema.
func (t *ReadFileTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description"` +
		`:"Workspace-relative path to read."},"start_line":{"type":"integer","minimum":1,` +
		`"description":"Optional 1-based line number to start reading from, inclusive."},` +
		`"end_line":{"type":"integer","minimum":1,"description":"Optional 1-based line ` +
		`number to stop reading at, inclusive."}},"required":["path"],"additionalProperties":false}`)
}

// Permission returns the default permission mode for this tool.
func (t *ReadFileTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

// Run executes the tool.
func (t *ReadFileTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("read_file cancelled: %w", ctx.Err())
	default:
	}

	var req readFileInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode read_file input: %w", err)
	}
	lineRange, err := parseReadFileLineRange(req)
	if err != nil {
		return nil, err
	}

	path, err := t.workspace.resolveReadPath(req.Path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", req.Path)
	}

	data, err := t.readFileContent(ctx, path, lineRange)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("read_file cancelled: %w", ctx.Err())
	default:
	}

	result := types.TextToolResult(data.content)
	result.Metadata = map[string]string{
		"path":       req.Path,
		"bytes":      fmt.Sprintf("%d", data.bytes),
		"truncated":  fmt.Sprintf("%t", data.truncated),
		"workspace":  t.workspace.Root(),
		"max_bytes":  fmt.Sprintf("%d", t.maxBytes),
		"file_bytes": fmt.Sprintf("%d", info.Size()),
	}
	if data.rangeEnabled {
		result.Metadata["start_line"] = fmt.Sprintf("%d", data.startLine)
		result.Metadata["lines_returned"] = fmt.Sprintf("%d", data.linesReturned)
		if data.endLine > 0 {
			result.Metadata["end_line"] = fmt.Sprintf("%d", data.endLine)
		}
	}

	return &result, nil
}

func parseReadFileLineRange(req readFileInput) (readFileLineRange, error) {
	if req.StartLine == nil && req.EndLine == nil {
		return readFileLineRange{}, nil
	}

	lineRange := readFileLineRange{
		enabled: true,
		start:   1,
	}
	if req.StartLine != nil {
		if *req.StartLine < 1 {
			return readFileLineRange{}, fmt.Errorf("start_line must be >= 1")
		}
		lineRange.start = *req.StartLine
	}
	if req.EndLine != nil {
		if *req.EndLine < 1 {
			return readFileLineRange{}, fmt.Errorf("end_line must be >= 1")
		}
		lineRange.end = *req.EndLine
	}
	if lineRange.end > 0 && lineRange.end < lineRange.start {
		return readFileLineRange{}, fmt.Errorf("end_line must be >= start_line")
	}

	return lineRange, nil
}

func (t *ReadFileTool) readFileContent(ctx context.Context, path string, lineRange readFileLineRange) (readFileData, error) {
	if lineRange.enabled {
		return t.readFileLineRange(ctx, path, lineRange)
	}

	return t.readFullFile(path)
}

func (t *ReadFileTool) readFullFile(path string) (readFileData, error) {
	file, err := os.Open(path)
	if err != nil {
		return readFileData{}, fmt.Errorf("open file: %w", err)
	}

	limited := io.LimitReader(file, t.maxBytes+1)
	data, err := io.ReadAll(limited)
	closeErr := file.Close()
	if err != nil {
		if closeErr != nil {
			return readFileData{}, fmt.Errorf("read file: %w; close file: %w", err, closeErr)
		}
		return readFileData{}, fmt.Errorf("read file: %w", err)
	}
	if closeErr != nil {
		return readFileData{}, fmt.Errorf("close file: %w", closeErr)
	}

	truncated := int64(len(data)) > t.maxBytes
	if truncated {
		data = data[:int(t.maxBytes)]
	}

	return readFileData{
		content:   string(data),
		bytes:     len(data),
		truncated: truncated,
	}, nil
}

func (t *ReadFileTool) readFileLineRange(ctx context.Context, path string, lineRange readFileLineRange) (readFileData, error) {
	file, err := os.Open(path)
	if err != nil {
		return readFileData{}, fmt.Errorf("open file: %w", err)
	}

	reader := bufio.NewReader(file)
	var data []byte
	lineNumber := 0
	linesReturned := 0
	truncated := false

	for {
		select {
		case <-ctx.Done():
			if closeErr := file.Close(); closeErr != nil {
				return readFileData{}, fmt.Errorf("read_file cancelled: %w; close file: %w",
					ctx.Err(), closeErr)
			}
			return readFileData{}, fmt.Errorf("read_file cancelled: %w", ctx.Err())
		default:
		}

		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNumber++
			if lineNumber >= lineRange.start && (lineRange.end == 0 || lineNumber <= lineRange.end) {
				remaining := int(t.maxBytes) - len(data)
				if remaining <= 0 {
					truncated = true
					break
				}
				if len(line) > remaining {
					data = append(data, line[:remaining]...)
					truncated = true
					linesReturned++
					break
				}
				data = append(data, line...)
				linesReturned++
			}
			if lineRange.end > 0 && lineNumber >= lineRange.end {
				break
			}
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			break
		}
		if closeErr := file.Close(); closeErr != nil {
			return readFileData{}, fmt.Errorf("read file: %w; close file: %w", err, closeErr)
		}
		return readFileData{}, fmt.Errorf("read file: %w", err)
	}
	if err := file.Close(); err != nil {
		return readFileData{}, fmt.Errorf("close file: %w", err)
	}

	return readFileData{
		content:       string(data),
		bytes:         len(data),
		truncated:     truncated,
		rangeEnabled:  true,
		startLine:     lineRange.start,
		endLine:       lineRange.end,
		linesReturned: linesReturned,
	}, nil
}

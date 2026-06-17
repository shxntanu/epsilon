package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/shxntanu/epsilon/core/types"
)

const (
	maxGrepLineBytes      = 1024 * 1024
	maxGrepDisplayCols    = 200
	maxGrepWrittenBytes   = 32 * 1024
	maxGrepWrittenMatches = 50
)

// GrepTool searches files inside a workspace.
type GrepTool struct {
	workspace *Workspace
}

// GrepInput defines the structured schema arguments the LLM will provide
type GrepInput struct {
	Pattern       string `json:"pattern"`                  // The regex pattern to search for
	Path          string `json:"path"`                     // The workspace-relative directory or file path
	IncludeExtend int    `json:"include_extend,omitempty"` // Number of surrounding lines of context
}

func NewGrepTool(workspaceRoot string) (*GrepTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}

	return &GrepTool{
		workspace: workspace,
	}, nil
}

func (t *GrepTool) Name() string {
	return "grep_search"
}

func (t *GrepTool) Description() string {
	return "Search workspace files with a regular expression and small line context."
}

func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Regular expression to match."
			},
			"path": {
				"type": "string",
				"description": "Workspace-relative file or directory. Defaults to '.'."
			},
			"include_extend": {
				"type": "integer",
				"description": "Context lines before and after each match. Defaults to 2."
			}
		},
		"required": ["pattern"],
		"additionalProperties": false
	}`)
}

// Permission returns the default permission mode for this read-only tool.
func (t *GrepTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

// Match internal struct holds concurrent search details
type Match struct {
	FilePath string
	Output   string
}

func (t *GrepTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var args GrepInput
	if err := json.Unmarshal(input, &args); err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Error parsing input arguments: %v", err))
		return &res, nil
	}

	// Dynamic baseline fallback for surrounding context
	contextLines := args.IncludeExtend
	if contextLines <= 0 {
		contextLines = 2
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Invalid regular expression pattern: %v", err))
		return &res, nil
	}

	searchPath := strings.TrimSpace(args.Path)
	if searchPath == "" {
		searchPath = "."
	}

	targetPath := t.workspace.Root()
	if searchPath != "." {
		var err error
		targetPath, err = t.workspace.resolveReadPath(searchPath)
		if err != nil {
			return nil, err
		}
	}
	displayRoot, err := filepath.Rel(t.workspace.Root(), targetPath)
	if err != nil {
		return nil, fmt.Errorf("resolve grep search path: %w", err)
	}
	if displayRoot == "." {
		displayRoot = "."
	}

	var wg sync.WaitGroup
	matchChan := make(chan Match, 100)
	errChan := make(chan error, 1)

	// Spin up filesystem walk pipeline
	go func() {
		defer close(matchChan)
		defer close(errChan)

		err := filepath.WalkDir(targetPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.IsDir() {
				// Avoid checking deep build/vcs configurations
				if shouldSkipGrepDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			displayPath, resolvedPath, ok := t.resolveWalkedFile(path)
			if !ok {
				return nil
			}

			wg.Add(1)
			go func(filePath string, displayPath string) {
				defer wg.Done()
				if err := searchFile(ctx, filePath, displayPath, re, contextLines, matchChan); err != nil {
					sendSearchErr(errChan, err)
				}
			}(resolvedPath, displayPath)

			return nil
		})

		if err != nil {
			sendSearchErr(errChan, err)
		}

		wg.Wait()
	}()

	// Buffer our thread outcomes safely
	var sb strings.Builder
	totalMatches := 0
	totalWritten := 0
	truncated := false
	var searchErr error

	for matchChan != nil || errChan != nil {
		select {
		case match, ok := <-matchChan:
			if !ok {
				matchChan = nil
				continue
			}
			totalMatches++
			if totalWritten >= maxGrepWrittenMatches ||
				sb.Len()+len(match.Output) > maxGrepWrittenBytes {
				truncated = true
				continue
			}
			sb.WriteString(match.Output)
			totalWritten++
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if searchErr == nil {
				searchErr = err
			}
		}
	}
	if truncated {
		sb.WriteString("\n[Truncated: Too many matches found. Please refine your regex pattern.]\n")
	}
	if searchErr != nil && totalMatches == 0 {
		return nil, searchErr
	}
	if searchErr != nil {
		sb.WriteString(fmt.Sprintf("\n[Some files could not be searched: %v]\n", searchErr))
	}

	if sb.Len() == 0 {
		sb.WriteString("No matches found for your pattern criteria.")
	}

	// Map unified metadata definitions
	meta := map[string]string{
		"total_matches":   fmt.Sprintf("%d", totalMatches),
		"written_matches": fmt.Sprintf("%d", totalWritten),
		"search_path":     displayRoot,
		"workspace":       t.workspace.Root(),
		"truncated":       fmt.Sprintf("%t", truncated),
	}

	result := types.TextToolResult(sb.String())
	result.Metadata = meta

	return &result, nil
}

func (t *GrepTool) resolveWalkedFile(path string) (string, string, bool) {
	displayPath, err := filepath.Rel(t.workspace.Root(), path)
	if err != nil {
		return "", "", false
	}
	resolvedPath, err := t.workspace.resolveReadPath(displayPath)
	if err != nil {
		return "", "", false
	}
	info, err := os.Stat(resolvedPath)
	if err != nil || info.IsDir() {
		return "", "", false
	}

	return displayPath, resolvedPath, true
}

func sendSearchErr(errChan chan<- error, err error) {
	select {
	case errChan <- err:
	default:
	}
}

func shouldSkipGrepDir(name string) bool {
	switch name {
	case ".git", ".cache", ".vendor", "node_modules", "vendor", "dist", "build", "target":
		return true
	default:
		return false
	}
}

// searchFile scans a single file line-by-line and extracts local context windows
func searchFile(ctx context.Context, path string, displayPath string, re *regexp.Regexp,
	contextLines int, matchChan chan<- Match) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	lines, err := readGrepLines(ctx, file)
	if err != nil {
		return err
	}

	for i, line := range lines {
		if re.MatchString(line) {
			start := i - contextLines
			if start < 0 {
				start = 0
			}
			end := i + contextLines + 1
			if end > len(lines) {
				end = len(lines)
			}

			var matchBuilder strings.Builder
			fmt.Fprintf(&matchBuilder, "\n--- Match found in %s ---\n", displayPath)

			for idx := start; idx < end; idx++ {
				lineNumber := idx + 1
				prefix := "   "
				if idx == i {
					prefix = ">> " // Visual arrow highlighting the exact target line for the model
				}
				matchBuilder.WriteString(fmt.Sprintf("%s%d: %s\n", prefix, lineNumber,
					truncateGrepLine(lines[idx])))
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case matchChan <- Match{
				FilePath: displayPath,
				Output:   matchBuilder.String(),
			}:
			}
		}
	}

	return nil
}

func readGrepLines(ctx context.Context, file *os.File) ([]string, error) {
	reader := bufio.NewReaderSize(file, 64*1024)
	lines := make([]string, 0)
	var lineBuilder strings.Builder
	lineTruncated := false

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		fragment, err := reader.ReadSlice('\n')
		if bytes.Contains(fragment, []byte{0}) {
			return nil, nil
		}
		if len(fragment) > 0 {
			remaining := maxGrepLineBytes - lineBuilder.Len()
			if remaining > 0 {
				if len(fragment) > remaining {
					lineBuilder.Write(fragment[:remaining])
					lineTruncated = true
				} else {
					lineBuilder.Write(fragment)
				}
			} else {
				lineTruncated = true
			}
		}

		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		if lineBuilder.Len() > 0 {
			line := strings.TrimRight(lineBuilder.String(), "\r\n")
			if lineTruncated {
				line += " [line truncated]"
			}
			lines = append(lines, line)
			lineBuilder.Reset()
			lineTruncated = false
		}
		if err == io.EOF {
			break
		}
	}

	return lines, nil
}

func truncateGrepLine(line string) string {
	if len(line) <= maxGrepDisplayCols {
		return line
	}

	cols := 0
	for idx := range line {
		if cols >= maxGrepDisplayCols {
			return line[:idx] + " ... [line truncated]"
		}
		cols++
	}

	return line
}

func isBinary(firstLine string) bool {
	return strings.Contains(firstLine, "\x00")
}

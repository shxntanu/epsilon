package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shxntanu/epsilon/core/types"

	"github.com/sourcegraph/go-diff/diff"
)

// PatchTool applies unified diffs inside a workspace.
type PatchTool struct {
	workspace *Workspace
}

// PatchInput defines what arguments the agent needs to provide
type PatchInput struct {
	// The raw unified diff string containing one or multiple file modifications
	Patch string `json:"patch"`
}

func NewPatchTool(workspaceRoot string) (*PatchTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}

	return &PatchTool{
		workspace: workspace,
	}, nil
}

func (t *PatchTool) Name() string {
	return "apply_patch"
}

func (t *PatchTool) Description() string {
	return "Applies a standard unified diff / patch string to modify files in the codebase. " +
		"Use this for precise edits, additions, or deletions of code sections across one or multiple files."
}

func (t *PatchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"patch": {
				"type": "string",
				"description": "The unified diff block containing hunk mappings (e.g. --- filename\n+++ filename\n@@ ... @@)"
			}
		},
		"required": ["patch"],
		"additionalProperties": false
	}`)
}

// Permission returns the default permission mode for this write-capable tool.
func (t *PatchTool) Permission() types.PermissionMode {
	return types.PermissionAsk
}

func (t *PatchTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var args PatchInput
	if err := json.Unmarshal(input, &args); err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Error parsing input arguments: %v", err))
		return &res, nil
	}

	// sourcegraph/go-diff parses the raw string into FileDiff structs
	fileDiffs, err := diff.ParseMultiFileDiff([]byte(args.Patch))
	if err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Failed to parse unified diff format: %v", err))
		return &res, nil
	}

	if len(fileDiffs) == 0 {
		res := types.ErrorToolResult("No valid file diff segments found in the patch.")
		return &res, nil
	}

	var appliedFiles []string

	// Process each file modified in the patch sequentially
	for _, fileDiff := range fileDiffs {
		if ctx.Err() != nil {
			res := types.ErrorToolResult("Patch operation aborted by context cancellation.")
			return &res, nil
		}

		targetFile, err := patchTargetFile(fileDiff)
		if err != nil {
			res := types.ErrorToolResult(fmt.Sprintf("Invalid patch target: %v", err))
			return &res, nil
		}
		targetPath, err := t.workspace.resolveWritePath(targetFile)
		if err != nil {
			res := types.ErrorToolResult(fmt.Sprintf("Patch target escapes workspace: %v", err))
			return &res, nil
		}

		// Apply hunks to the target file
		err = applyFilePatch(targetPath, fileDiff.Hunks)
		if err != nil {
			res := types.ErrorToolResult(fmt.Sprintf("Failed patching file %s: %v", targetFile, err))
			return &res, nil
		}

		appliedFiles = append(appliedFiles, targetFile)
	}

	summary := fmt.Sprintf("Success! Applied patch modifications across %d file(s):\n- %s",
		len(appliedFiles), strings.Join(appliedFiles, "\n- "))

	result := types.TextToolResult(summary)
	result.Metadata = map[string]string{
		"status":         "patched",
		"modified_files": strings.Join(appliedFiles, ","),
	}

	return &result, nil
}

func patchTargetFile(fileDiff *diff.FileDiff) (string, error) {
	targetFile := fileDiff.NewName
	if targetFile == "" || targetFile == "/dev/null" {
		targetFile = fileDiff.OrigName
	}
	if targetFile == "" || targetFile == "/dev/null" {
		return "", fmt.Errorf("missing file name")
	}

	targetFile = strings.TrimSpace(targetFile)
	if strings.HasPrefix(targetFile, "b/") || strings.HasPrefix(targetFile, "a/") {
		targetFile = targetFile[2:]
	}
	targetFile = filepath.Clean(targetFile)
	if targetFile == "." {
		return "", fmt.Errorf("missing file name")
	}

	return targetFile, nil
}

func applyFilePatch(filePath string, hunks []*diff.Hunk) error {
	var fileLines []string

	// Check if file is a fresh creation
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		fileLines = []string{}
	} else if err != nil {
		return err
	} else {
		// Read existing contents
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if len(content) == 0 {
			fileLines = []string{}
		} else {
			fileLines = strings.Split(string(content), "\n")
		}
	}

	// Shift adjustment offset to track how previous hunk mutations change line numbering
	lineOffset := 0

	for _, hunk := range hunks {
		hunkBodyLines := strings.Split(string(hunk.Body), "\n")
		// Safely handle trailing newline additions from split groups
		if len(hunkBodyLines) > 0 && hunkBodyLines[len(hunkBodyLines)-1] == "" {
			hunkBodyLines = hunkBodyLines[:len(hunkBodyLines)-1]
		}

		// Extract expected old line state and target lines
		var expectedOldLines []string
		var newLines []string

		for _, line := range hunkBodyLines {
			if len(line) == 0 {
				// Context line that happens to be empty
				expectedOldLines = append(expectedOldLines, "")
				newLines = append(newLines, "")
				continue
			}

			prefix := line[0]
			content := line[1:]

			switch prefix {
			case ' ': // Context line
				expectedOldLines = append(expectedOldLines, content)
				newLines = append(newLines, content)
			case '-': // Removed line
				expectedOldLines = append(expectedOldLines, content)
			case '+': // Added line
				newLines = append(newLines, content)
			}
		}

		// Step 1: Attempt to apply exactly where the header says it should be
		targetIndex := int(hunk.OrigStartLine) - 1 + lineOffset
		matched := verifyAnchorMatch(fileLines, targetIndex, expectedOldLines)

		// Step 2: Fallback scan if LLM miscalculated header numbering positions
		if !matched {
			targetIndex = findFuzzyAnchorIndex(fileLines, expectedOldLines)
			if targetIndex == -1 {
				return fmt.Errorf("hunk alignment failed; could not find matching original "+
					"context block for header range beginning at line %d", hunk.OrigStartLine)
			}
		}

		// Calculate deletion extent
		deleteCount := len(expectedOldLines)
		if targetIndex+deleteCount > len(fileLines) {
			deleteCount = len(fileLines) - targetIndex
		}

		// Perform the array stitch array splice rewrite
		updatedLines := append(fileLines[:targetIndex], append(newLines,
			fileLines[targetIndex+deleteCount:]...)...)

		// Recalculate working delta offsets
		lineOffset += len(updatedLines) - len(fileLines)
		fileLines = updatedLines
	}

	// Ensure any parent directory trees exist prior to writing changes
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	// Rejoin contents and update the target file cleanly
	outputData := strings.Join(fileLines, "\n")
	return os.WriteFile(filePath, []byte(outputData), 0644)
}

// Verifies if the file slice at a specific index matches the old code block expectations
func verifyAnchorMatch(fileLines []string, startIdx int, expected []string) bool {
	if startIdx < 0 || startIdx+len(expected) > len(fileLines) {
		return false
	}
	for i, expLine := range expected {
		if fileLines[startIdx+i] != expLine {
			return false
		}
	}
	return true
}

// Robust fallback search loop finding where the target hunk actually lives inside the file
func findFuzzyAnchorIndex(fileLines []string, expected []string) int {
	if len(expected) == 0 {
		return 0 // Edge case appending to empty configurations
	}

	// Exact scan window loop matching
	for i := 0; i <= len(fileLines)-len(expected); i++ {
		match := true
		for j, expLine := range expected {
			if fileLines[i+j] != expLine {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

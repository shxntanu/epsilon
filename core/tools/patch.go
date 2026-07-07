package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

// PatchInstruction defines a single atomic search-and-replace block for a file.
type PatchInstruction struct {
	Path    string `json:"path"`
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

// PatchTool applies structured search-and-replace patches inside a workspace.
type PatchTool struct {
	workspace *Workspace
}

func NewPatchTool(workspaceRoot string) (*PatchTool, error) {
	workspace, err := NewWorkspace(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize workspace for patch tool: %w", err)
	}

	return &PatchTool{
		workspace: workspace,
	}, nil
}

func (t *PatchTool) Name() string {
	return "apply_patch"
}

func (t *PatchTool) Description() string {
	return "Apply highly precise code modifications using structural search-and-replace blocks. " +
		"Specify the path, the exact lines of code to find, and the new code to replace them with."
}

func (t *PatchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "The workspace-relative path of the target file to modify."
			},
			"search": {
				"type": "string",
				"description": "The exact block of code to search for. Provide enough context lines (3-5) to uniquely identify the location. For creating new files, leave this completely empty."
			},
			"replace": {
				"type": "string",
				"description": "The block of code to replace the search block with. Leave empty if deleting the block."
			}
		},
		"required": ["path", "search", "replace"],
		"additionalProperties": false
	}`)
}

func (t *PatchTool) Permission() types.PermissionMode {
	return types.PermissionAsk
}

func (t *PatchTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var instruction PatchInstruction
	if err := json.Unmarshal(input, &instruction); err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Invalid arguments shape: %v", err))
		return &res, nil
	}

	if instruction.Path == "" {
		res := types.ErrorToolResult("Required parameter 'path' cannot be empty.")
		return &res, nil
	}

	targetPath, err := t.workspace.resolveWritePath(instruction.Path)
	if err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Security violation: target path escapes workspace: %v", err))
		return &res, nil
	}

	if ctx.Err() != nil {
		res := types.ErrorToolResult("Operation cancelled prior to writing.")
		return &res, nil
	}

	var before []byte
	existed := false
	if data, err := os.ReadFile(targetPath); err == nil {
		before = data
		existed = true
	} else if !os.IsNotExist(err) {
		res := types.ErrorToolResult(fmt.Sprintf("Unable to read target before patching: %v", err))
		return &res, nil
	}

	// Execute execution phase
	err = t.executePatch(targetPath, instruction)
	if err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Patch application failed on %s: %v", instruction.Path, err))
		return &res, nil
	}

	summary := fmt.Sprintf("Success! Applied structural patch to %s.", instruction.Path)
	result := types.TextToolResult(summary)
	result.Metadata = map[string]string{
		"status":        "patched",
		"modified_file": instruction.Path,
	}
	if after, err := os.ReadFile(targetPath); err == nil {
		if diff := writeFileDiff(instruction.Path, before, after, existed); diff != "" {
			result.Metadata["diff"] = diff
		}
	}

	return &result, nil
}

func (t *PatchTool) executePatch(targetPath string, inst PatchInstruction) error {
	// Scenario A: Creating a brand new file
	if inst.Search == "" {
		_, err := os.Stat(targetPath)
		if err == nil {
			return fmt.Errorf("file already exists; provide a 'search' block to target specific lines")
		}

		// Clean up string representation and preserve layout rules
		fileData := inst.Replace
		if fileData != "" && !strings.HasSuffix(fileData, "\n") {
			fileData += "\n"
		}
		return safeWriteFile(targetPath, []byte(fileData))
	}

	// Scenario B: Modifying an existing file via sliding window replacement
	fileBytes, err := os.ReadFile(targetPath)
	if err != nil {
		return fmt.Errorf("unable to read target file: %w", err)
	}

	fileContent := string(fileBytes)
	hasFinalNewline := strings.HasSuffix(fileContent, "\n")

	fileLines := splitLines(fileContent)
	searchLines := splitLines(inst.Search)
	replaceLines := splitLines(inst.Replace)

	// Step 1: Exact Line Matching
	matchIdx := findExactAnchorIndex(fileLines, searchLines)

	// Step 2: Fallback to Robust Whitespace-Insensitive Fuzzy Matching
	if matchIdx == -1 {
		matchIdx = findFuzzyAnchorIndex(fileLines, searchLines)
		if matchIdx == -1 {
			return fmt.Errorf("structural block alignment failed; search code block could not be uniquely matched inside file content")
		}
	}

	// Splice and merge arrays cleanly
	updatedLines := make([]string, 0, len(fileLines)-len(searchLines)+len(replaceLines))
	updatedLines = append(updatedLines, fileLines[:matchIdx]...)
	updatedLines = append(updatedLines, replaceLines...)
	updatedLines = append(updatedLines, fileLines[matchIdx+len(searchLines):]...)

	outputData := strings.Join(updatedLines, "\n")
	if hasFinalNewline && len(updatedLines) > 0 && !strings.HasSuffix(outputData, "\n") {
		outputData += "\n"
	}

	return safeWriteFile(targetPath, []byte(outputData))
}

// Exact match locator
func findExactAnchorIndex(fileLines []string, searchLines []string) int {
	if len(searchLines) == 0 {
		return -1
	}
	for i := 0; i <= len(fileLines)-len(searchLines); i++ {
		match := true
		for j := 0; j < len(searchLines); j++ {
			if fileLines[i+j] != searchLines[j] {
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

// Fuzzy semantic layout match locator (Tabs vs Spaces, missing padding)
func findFuzzyAnchorIndex(fileLines []string, searchLines []string) int {
	if len(searchLines) == 0 {
		return -1
	}

	normalize := func(s string) string {
		return strings.Join(strings.Fields(strings.TrimSpace(s)), "")
	}

	normSearch := make([]string, len(searchLines))
	for i, line := range searchLines {
		normSearch[i] = normalize(line)
	}

	for i := 0; i <= len(fileLines)-len(searchLines); i++ {
		match := true
		for j := 0; j < len(searchLines); j++ {
			if normalize(fileLines[i+j]) != normSearch[j] {
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

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s == "" {
		return []string{}
	}
	// Trim a single final trailing newline to prevent empty trailing line segment issues
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// Atomically writes files using temp files to avoid corruption mid-stream
func safeWriteFile(filePath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(filePath), ".epsilon-patch-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()

	defer func() {
		_ = os.Remove(tempName) // Clean fallback cleanup if rename loop fails
	}()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	return os.Rename(tempName, filePath)
}

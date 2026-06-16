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

type patchAction int

const (
	patchActionUpdate patchAction = iota
	patchActionAdd
	patchActionDelete
)

type filePatch struct {
	path   string
	action patchAction
	hunks  []patchHunk
	lines  []string
}

type patchHunk struct {
	origStartLine int
	origLines     int
	lines         []patchLine
}

type patchLine struct {
	kind    byte
	content string
}

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
	return "Applies a patch string to modify files in the workspace. Supports standard " +
		"unified diffs and agent-style patches using *** Begin Patch with Add File, " +
		"Update File, and Delete File sections. Use this for precise edits, additions, " +
		"or deletions across one or multiple files."
}

func (t *PatchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"patch": {
				"type": "string",
				"description": "Patch text as a unified diff or an agent-style *** Begin Patch block."
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

	patches, err := parsePatch(args.Patch)
	if err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Failed to parse patch: %v", err))
		return &res, nil
	}

	if len(patches) == 0 {
		res := types.ErrorToolResult("No valid file diff segments found in the patch.")
		return &res, nil
	}

	var appliedFiles []string

	// Process each file modified in the patch sequentially
	for _, patch := range patches {
		if ctx.Err() != nil {
			res := types.ErrorToolResult("Patch operation aborted by context cancellation.")
			return &res, nil
		}

		targetPath, err := t.workspace.resolveWritePath(patch.path)
		if err != nil {
			res := types.ErrorToolResult(fmt.Sprintf("Patch target escapes workspace: %v", err))
			return &res, nil
		}

		// Apply hunks to the target file
		err = applyFilePatch(targetPath, patch)
		if err != nil {
			res := types.ErrorToolResult(fmt.Sprintf("Failed patching file %s: %v", patch.path, err))
			return &res, nil
		}

		appliedFiles = append(appliedFiles, patch.path)
	}

	summary := fmt.Sprintf("Success! Applied patch modifications across %d file(s):\n- %s",
		len(appliedFiles), strings.Join(appliedFiles, "\n- "))

	result := types.TextToolResult(summary)
	result.Metadata = map[string]string{
		"status":         "patched",
		"modified_files": strings.Join(appliedFiles, ","),
		"diff":           strings.TrimSpace(args.Patch),
	}

	return &result, nil
}

func parsePatch(rawPatch string) ([]filePatch, error) {
	patch := strings.TrimSpace(rawPatch)
	if patch == "" {
		return nil, fmt.Errorf("patch is empty")
	}
	if strings.HasPrefix(patch, "*** Begin Patch") {
		return parseAgentPatch(patch)
	}

	return parseUnifiedPatch(patch)
}

func parseUnifiedPatch(rawPatch string) ([]filePatch, error) {
	fileDiffs, err := diff.ParseMultiFileDiff([]byte(rawPatch))
	if err != nil {
		return nil, fmt.Errorf("parse unified diff: %w", err)
	}

	patches := make([]filePatch, 0, len(fileDiffs))
	for _, fileDiff := range fileDiffs {
		targetFile, action, err := patchTargetFile(fileDiff)
		if err != nil {
			return nil, err
		}

		hunks := make([]patchHunk, 0, len(fileDiff.Hunks))
		for _, hunk := range fileDiff.Hunks {
			parsedHunk, err := parseUnifiedHunk(hunk)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", targetFile, err)
			}
			hunks = append(hunks, parsedHunk)
		}

		patches = append(patches, filePatch{
			path:   targetFile,
			action: action,
			hunks:  hunks,
		})
	}

	return patches, nil
}

func parseAgentPatch(rawPatch string) ([]filePatch, error) {
	lines := strings.Split(rawPatch, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return nil, fmt.Errorf("missing *** Begin Patch")
	}

	var patches []filePatch
	var current *filePatch
	var currentHunk *patchHunk

	flushHunk := func() {
		if current != nil && currentHunk != nil {
			current.hunks = append(current.hunks, *currentHunk)
			currentHunk = nil
		}
	}
	flushPatch := func() {
		flushHunk()
		if current != nil {
			patches = append(patches, *current)
			current = nil
		}
	}

	for i := 1; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\r")
		switch {
		case line == "*** End Patch":
			flushPatch()
			return patches, nil
		case strings.HasPrefix(line, "*** Add File: "):
			flushPatch()
			path, err := cleanPatchPath(strings.TrimPrefix(line, "*** Add File: "))
			if err != nil {
				return nil, err
			}
			current = &filePatch{path: path, action: patchActionAdd}
		case strings.HasPrefix(line, "*** Delete File: "):
			flushPatch()
			path, err := cleanPatchPath(strings.TrimPrefix(line, "*** Delete File: "))
			if err != nil {
				return nil, err
			}
			current = &filePatch{path: path, action: patchActionDelete}
		case strings.HasPrefix(line, "*** Update File: "):
			flushPatch()
			path, err := cleanPatchPath(strings.TrimPrefix(line, "*** Update File: "))
			if err != nil {
				return nil, err
			}
			current = &filePatch{path: path, action: patchActionUpdate}
		case strings.HasPrefix(line, "*** Move to: "):
			return nil, fmt.Errorf("move patches are not supported yet")
		case strings.HasPrefix(line, "*** "):
			return nil, fmt.Errorf("unsupported patch directive: %s", line)
		default:
			if current == nil {
				if strings.TrimSpace(line) == "" {
					continue
				}
				return nil, fmt.Errorf("patch content before file directive: %s", line)
			}
			if err := appendAgentPatchLine(current, &currentHunk, line, flushHunk); err != nil {
				return nil, err
			}
		}
	}

	return nil, fmt.Errorf("missing *** End Patch")
}

func appendAgentPatchLine(current *filePatch, currentHunk **patchHunk, line string,
	flushHunk func()) error {
	if current.action == patchActionDelete {
		if strings.TrimSpace(line) == "" {
			return nil
		}
		return fmt.Errorf("delete file patch cannot contain body lines")
	}

	if current.action == patchActionAdd {
		if line == "" {
			return nil
		}
		if line[0] != '+' {
			return fmt.Errorf("add file line must start with '+': %s", line)
		}
		current.lines = append(current.lines, line[1:])
		return nil
	}

	if strings.HasPrefix(line, "@@") {
		flushHunk()
		*currentHunk = &patchHunk{}
		return nil
	}
	if line == "*** End of File" {
		return nil
	}
	if *currentHunk == nil {
		*currentHunk = &patchHunk{}
	}
	if line == "" {
		return fmt.Errorf("update hunk line cannot be empty")
	}

	switch line[0] {
	case ' ', '-', '+':
		(*currentHunk).lines = append((*currentHunk).lines, patchLine{
			kind:    line[0],
			content: line[1:],
		})
	default:
		return fmt.Errorf("update hunk line must start with space, '-', or '+': %s", line)
	}

	return nil
}

func patchTargetFile(fileDiff *diff.FileDiff) (string, patchAction, error) {
	action := patchActionUpdate
	if fileDiff.OrigName == "/dev/null" {
		action = patchActionAdd
	}
	if fileDiff.NewName == "/dev/null" {
		action = patchActionDelete
	}

	targetFile := fileDiff.NewName
	if targetFile == "" || targetFile == "/dev/null" {
		targetFile = fileDiff.OrigName
	}
	if targetFile == "" || targetFile == "/dev/null" {
		return "", action, fmt.Errorf("missing file name")
	}

	targetFile, err := cleanPatchPath(targetFile)
	if err != nil {
		return "", action, err
	}

	return targetFile, action, nil
}

func cleanPatchPath(path string) (string, error) {
	path = strings.Trim(strings.TrimSpace(path), `"`)
	if strings.HasPrefix(path, "b/") || strings.HasPrefix(path, "a/") {
		path = path[2:]
	}
	path = filepath.Clean(path)
	if path == "." {
		return "", fmt.Errorf("missing file name")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("patch path must be relative to workspace: %s", path)
	}

	return path, nil
}

func parseUnifiedHunk(hunk *diff.Hunk) (patchHunk, error) {
	lines, err := parsePatchLines(string(hunk.Body))
	if err != nil {
		return patchHunk{}, err
	}

	return patchHunk{
		origStartLine: int(hunk.OrigStartLine),
		origLines:     int(hunk.OrigLines),
		lines:         lines,
	}, nil
}

func parsePatchLines(body string) ([]patchLine, error) {
	rawLines := strings.Split(body, "\n")
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}

	lines := make([]patchLine, 0, len(rawLines))
	for _, line := range rawLines {
		if line == `\ No newline at end of file` {
			continue
		}
		if line == "" {
			return nil, fmt.Errorf("invalid empty hunk line")
		}

		switch line[0] {
		case ' ', '-', '+':
			lines = append(lines, patchLine{
				kind:    line[0],
				content: line[1:],
			})
		default:
			return nil, fmt.Errorf("invalid hunk line prefix %q", line[0])
		}
	}

	return lines, nil
}

func applyFilePatch(filePath string, patch filePatch) error {
	if patch.action == patchActionDelete {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	fileLines, finalNewline, err := readPatchFile(filePath, patch.action == patchActionAdd)
	if err != nil {
		return err
	}
	if patch.action == patchActionAdd {
		fileLines = patch.lines
		finalNewline = true
		if len(patch.hunks) == 0 {
			return writePatchFile(filePath, fileLines, finalNewline)
		}
	}

	// Shift adjustment offset to track how previous hunk mutations change line numbering
	lineOffset := 0

	for _, hunk := range patch.hunks {
		// Extract expected old line state and target lines
		var expectedOldLines []string
		var newLines []string

		for _, line := range hunk.lines {
			switch line.kind {
			case ' ': // Context line
				expectedOldLines = append(expectedOldLines, line.content)
				newLines = append(newLines, line.content)
			case '-': // Removed line
				expectedOldLines = append(expectedOldLines, line.content)
			case '+': // Added line
				newLines = append(newLines, line.content)
			}
		}

		// Step 1: Attempt to apply exactly where the header says it should be
		targetIndex := targetIndexForHunk(hunk, lineOffset)
		matched := hunk.origStartLine > 0 && verifyAnchorMatch(fileLines, targetIndex, expectedOldLines)

		// Step 2: Fallback scan if LLM miscalculated header numbering positions
		if !matched {
			targetIndex = findFuzzyAnchorIndex(fileLines, expectedOldLines)
			if targetIndex == -1 {
				return fmt.Errorf("hunk alignment failed; could not find matching original "+
					"context block for header range beginning at line %d", hunk.origStartLine)
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

	return writePatchFile(filePath, fileLines, finalNewline)
}

func readPatchFile(filePath string, allowMissing bool) ([]string, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if allowMissing && os.IsNotExist(err) {
			return []string{}, false, nil
		}
		return nil, false, err
	}
	if len(content) == 0 {
		return []string{}, false, nil
	}

	text := string(content)
	finalNewline := strings.HasSuffix(text, "\n")
	if finalNewline {
		text = strings.TrimSuffix(text, "\n")
	}
	return strings.Split(text, "\n"), finalNewline, nil
}

func targetIndexForHunk(hunk patchHunk, lineOffset int) int {
	if hunk.origStartLine <= 0 {
		return lineOffset
	}
	if hunk.origLines == 0 {
		return hunk.origStartLine + lineOffset
	}

	return hunk.origStartLine - 1 + lineOffset
}

func writePatchFile(filePath string, lines []string, finalNewline bool) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	outputData := strings.Join(lines, "\n")
	if finalNewline && len(lines) > 0 {
		outputData += "\n"
	}

	temp, err := os.CreateTemp(filepath.Dir(filePath), ".epsilon-patch-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	shouldRemoveTemp := true
	defer func() {
		if shouldRemoveTemp {
			_ = os.Remove(tempName)
		}
	}()

	if _, err := temp.WriteString(outputData); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, filePath); err != nil {
		return err
	}
	shouldRemoveTemp = false
	return nil
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

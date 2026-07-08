package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/shxntanu/epsilon/core/types"
)

type patchInput struct {
	Patch string `json:"patch"`
}

// PatchTool applies structured patch text inside a workspace.
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
	return "Apply structured patch edits to workspace files."
}

func (t *PatchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"patch": {
				"type": "string",
				"description": "Patch text with this format: begin with '*** Begin Patch' and end with '*** End Patch'. File operations are '*** Add File: path', '*** Delete File: path', and '*** Update File: path'. Paths must be relative. For Add File, every content line must start with '+'. For Update File, use optional '@@' or '@@ context' hunk headers, then lines starting with space for context, '-' for removals, and '+' for additions. Use '*** End of File' inside the final update hunk when matching the end of a file. To rename, put '*** Move to: new/path' immediately after an Update File header before any hunks."
			}
		},
		"required": ["patch"],
		"additionalProperties": false
	}`)
}

func (t *PatchTool) Permission() types.PermissionMode {
	return types.PermissionAllow
}

func (t *PatchTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	var req patchInput
	if err := json.Unmarshal(input, &req); err != nil {
		res := types.ErrorToolResult(fmt.Sprintf("Invalid arguments shape: %v", err))
		return &res, nil
	}
	if strings.TrimSpace(req.Patch) == "" {
		res := types.ErrorToolResult("Required parameter 'patch' cannot be empty.")
		return &res, nil
	}

	return t.runPatch(ctx, req.Patch)
}

type patchKind string

const (
	patchAdd    patchKind = "add"
	patchDelete patchKind = "delete"
	patchUpdate patchKind = "update"
)

type patchHunk struct {
	kind     patchKind
	path     string
	movePath string
	contents string
	chunks   []patchChunk
}

type patchChunk struct {
	changeContext string
	hasContext    bool
	oldLines      []string
	newLines      []string
	endOfFile     bool
}

type patchFileSnapshot struct {
	before   []byte
	after    []byte
	existed  bool
	afterSet bool
}

func (t *PatchTool) runPatch(ctx context.Context, patchText string) (*types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("apply_patch cancelled: %w", err)
	}

	hunks, err := parsePatch(patchText)
	if err != nil {
		result := types.ErrorToolResult(fmt.Sprintf("Invalid patch: %v", err))
		return &result, nil
	}

	snapshots := make(map[string]*patchFileSnapshot)
	affected := make(map[string]string)
	for _, hunk := range hunks {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("apply_patch cancelled: %w", err)
		}
		if err := t.applyHunk(hunk, snapshots, affected); err != nil {
			result := types.ErrorToolResult(fmt.Sprintf("Patch application failed on %s: %v", hunk.path, err))
			result.Metadata = patchResultMetadata(t.workspace.Root(), snapshots, affected)
			result.Metadata["status"] = "failed"
			return &result, nil
		}
	}

	result := types.TextToolResult(formatPatchSummary(affected))
	result.Metadata = patchResultMetadata(t.workspace.Root(), snapshots, affected)
	return &result, nil
}

func (t *PatchTool) applyHunk(hunk patchHunk, snapshots map[string]*patchFileSnapshot, affected map[string]string) error {
	sourcePath, err := t.workspace.resolveWritePath(hunk.path)
	if err != nil {
		return err
	}
	if err := snapshotPatchPath(sourcePath, snapshots); err != nil {
		return err
	}

	switch hunk.kind {
	case patchAdd:
		if err := ensureNotDirectory(sourcePath); err != nil {
			return err
		}
		if err := safeWriteFile(sourcePath, []byte(hunk.contents)); err != nil {
			return err
		}
		affected[hunk.path] = "added"
		return snapshotAfter(sourcePath, snapshots)
	case patchDelete:
		if err := ensureRegularFile(sourcePath); err != nil {
			return err
		}
		if err := os.Remove(sourcePath); err != nil {
			return fmt.Errorf("delete file: %w", err)
		}
		affected[hunk.path] = "deleted"
		return snapshotAfter(sourcePath, snapshots)
	case patchUpdate:
		if err := ensureRegularFile(sourcePath); err != nil {
			return err
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("read file to update: %w", err)
		}
		updated := string(data)
		if len(hunk.chunks) > 0 {
			updated, err = applyChunks(string(data), hunk.path, hunk.chunks)
			if err != nil {
				return err
			}
		}
		if hunk.movePath != "" {
			destPath, err := t.workspace.resolveWritePath(hunk.movePath)
			if err != nil {
				return err
			}
			if err := snapshotPatchPath(destPath, snapshots); err != nil {
				return err
			}
			if err := ensureNotDirectory(destPath); err != nil {
				return err
			}
			if err := safeWriteFile(destPath, []byte(updated)); err != nil {
				return err
			}
			if err := os.Remove(sourcePath); err != nil {
				return fmt.Errorf("remove original after move: %w", err)
			}
			affected[hunk.path] = "modified"
			affected[hunk.movePath] = "added"
			if err := snapshotAfter(destPath, snapshots); err != nil {
				return err
			}
			return snapshotAfter(sourcePath, snapshots)
		}
		if err := safeWriteFile(sourcePath, []byte(updated)); err != nil {
			return err
		}
		affected[hunk.path] = "modified"
		return snapshotAfter(sourcePath, snapshots)
	default:
		return fmt.Errorf("unsupported patch hunk kind: %s", hunk.kind)
	}
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

func parsePatch(text string) ([]patchHunk, error) {
	lines := normalizePatchLines(text)
	if len(lines) == 0 {
		return nil, fmt.Errorf("patch is empty")
	}
	if trimmed := strings.TrimSpace(lines[0]); trimmed != "*** Begin Patch" {
		return nil, fmt.Errorf("first line must be '*** Begin Patch'")
	}
	if trimmed := strings.TrimSpace(lines[len(lines)-1]); trimmed != "*** End Patch" {
		return nil, fmt.Errorf("last line must be '*** End Patch'")
	}

	var hunks []patchHunk
	var current *patchHunk
	lineNumber := 1
	for _, line := range lines[1 : len(lines)-1] {
		lineNumber++
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "*** Add File: "):
			if err := finishHunk(current); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			hunks = append(hunks, patchHunk{
				kind: patchAdd,
				path: strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Add File: ")),
			})
			current = &hunks[len(hunks)-1]
			if err := validatePatchPath(current.path); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
		case strings.HasPrefix(trimmed, "*** Delete File: "):
			if err := finishHunk(current); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			hunks = append(hunks, patchHunk{
				kind: patchDelete,
				path: strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Delete File: ")),
			})
			current = &hunks[len(hunks)-1]
			if err := validatePatchPath(current.path); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
		case strings.HasPrefix(trimmed, "*** Update File: "):
			if err := finishHunk(current); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			hunks = append(hunks, patchHunk{
				kind: patchUpdate,
				path: strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Update File: ")),
			})
			current = &hunks[len(hunks)-1]
			if err := validatePatchPath(current.path); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
		default:
			if current == nil {
				return nil, fmt.Errorf("line %d: expected a file operation header", lineNumber)
			}
			if err := parseHunkLine(current, line, trimmed, lineNumber); err != nil {
				return nil, err
			}
		}
	}
	if err := finishHunk(current); err != nil {
		return nil, err
	}
	if len(hunks) == 0 {
		return nil, fmt.Errorf("patch must contain at least one file operation")
	}
	return hunks, nil
}

func normalizePatchLines(text string) []string {
	text = strings.TrimSpace(text)
	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(rawLines) >= 4 {
		first := strings.TrimSpace(rawLines[0])
		last := strings.TrimSpace(rawLines[len(rawLines)-1])
		if (first == "<<EOF" || first == "<<'EOF'" || first == "<<\"EOF\"") &&
			strings.HasSuffix(last, "EOF") {
			rawLines = rawLines[1 : len(rawLines)-1]
		}
	}
	return rawLines
}

func parseHunkLine(hunk *patchHunk, line string, trimmed string, lineNumber int) error {
	switch hunk.kind {
	case patchAdd:
		content, ok := strings.CutPrefix(line, "+")
		if !ok {
			return fmt.Errorf("line %d: add-file lines must start with '+'", lineNumber)
		}
		hunk.contents += content + "\n"
		return nil
	case patchDelete:
		if trimmed == "" {
			return nil
		}
		return fmt.Errorf("line %d: delete-file operations cannot contain body lines", lineNumber)
	case patchUpdate:
		return parseUpdateLine(hunk, line, trimmed, lineNumber)
	default:
		return fmt.Errorf("line %d: unsupported hunk kind %s", lineNumber, hunk.kind)
	}
}

func parseUpdateLine(hunk *patchHunk, line string, trimmed string, lineNumber int) error {
	if strings.HasPrefix(trimmed, "*** Move to: ") {
		if len(hunk.chunks) > 0 {
			return fmt.Errorf("line %d: move marker must appear before update hunks", lineNumber)
		}
		movePath := strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Move to: "))
		if err := validatePatchPath(movePath); err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		hunk.movePath = movePath
		return nil
	}
	if trimmed == "@@" {
		hunk.chunks = append(hunk.chunks, patchChunk{})
		return nil
	}
	if strings.HasPrefix(trimmed, "@@ ") {
		hunk.chunks = append(hunk.chunks, patchChunk{
			changeContext: strings.TrimPrefix(trimmed, "@@ "),
			hasContext:    true,
		})
		return nil
	}
	if trimmed == "*** End of File" {
		if len(hunk.chunks) == 0 {
			return fmt.Errorf("line %d: end-of-file marker requires an update hunk", lineNumber)
		}
		chunk := &hunk.chunks[len(hunk.chunks)-1]
		if len(chunk.oldLines) == 0 && len(chunk.newLines) == 0 {
			return fmt.Errorf("line %d: update hunk does not contain any lines", lineNumber)
		}
		chunk.endOfFile = true
		return nil
	}
	if len(hunk.chunks) == 0 {
		hunk.chunks = append(hunk.chunks, patchChunk{})
	}
	chunk := &hunk.chunks[len(hunk.chunks)-1]
	if line == "" {
		chunk.oldLines = append(chunk.oldLines, "")
		chunk.newLines = append(chunk.newLines, "")
		return nil
	}
	if content, ok := strings.CutPrefix(line, " "); ok {
		chunk.oldLines = append(chunk.oldLines, content)
		chunk.newLines = append(chunk.newLines, content)
		return nil
	}
	if content, ok := strings.CutPrefix(line, "+"); ok {
		chunk.newLines = append(chunk.newLines, content)
		return nil
	}
	if content, ok := strings.CutPrefix(line, "-"); ok {
		chunk.oldLines = append(chunk.oldLines, content)
		return nil
	}
	return fmt.Errorf("line %d: update lines must start with ' ', '+', '-', or '@@'", lineNumber)
}

func finishHunk(hunk *patchHunk) error {
	if hunk == nil {
		return nil
	}
	switch hunk.kind {
	case patchAdd:
		if hunk.contents == "" {
			return fmt.Errorf("add-file operation for %s is empty", hunk.path)
		}
	case patchUpdate:
		if len(hunk.chunks) == 0 && hunk.movePath == "" {
			return fmt.Errorf("update-file operation for %s is empty", hunk.path)
		}
		for _, chunk := range hunk.chunks {
			if len(chunk.oldLines) == 0 && len(chunk.newLines) == 0 {
				return fmt.Errorf("update hunk for %s does not contain any lines", hunk.path)
			}
		}
	}
	return nil
}

func validatePatchPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("patch path is empty")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("patch path must be relative: %s", path)
	}
	return nil
}

func applyChunks(content string, path string, chunks []patchChunk) (string, error) {
	originalLines := splitLines(content)
	replacements, err := computeReplacements(originalLines, path, chunks)
	if err != nil {
		return "", err
	}
	newLines := applyLineReplacements(originalLines, replacements)
	return joinPatchLines(newLines, true), nil
}

type lineReplacement struct {
	start int
	old   int
	new   []string
}

func computeReplacements(originalLines []string, path string, chunks []patchChunk) ([]lineReplacement, error) {
	replacements := make([]lineReplacement, 0, len(chunks))
	lineIndex := 0
	for _, chunk := range chunks {
		if chunk.hasContext {
			contextIndex := seekLineSequence(originalLines, []string{chunk.changeContext}, lineIndex, false)
			if contextIndex < 0 {
				return nil, fmt.Errorf("failed to find context %q in %s", chunk.changeContext, path)
			}
			lineIndex = contextIndex + 1
		}
		if len(chunk.oldLines) == 0 {
			insertionIndex := len(originalLines)
			replacements = append(replacements, lineReplacement{
				start: insertionIndex,
				new:   append([]string(nil), chunk.newLines...),
			})
			continue
		}

		pattern := chunk.oldLines
		newLines := chunk.newLines
		found := seekLineSequence(originalLines, pattern, lineIndex, chunk.endOfFile)
		if found < 0 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			found = seekLineSequence(originalLines, pattern, lineIndex, chunk.endOfFile)
		}
		if found < 0 {
			return nil, fmt.Errorf("failed to find expected lines in %s:\n%s", path, strings.Join(chunk.oldLines, "\n"))
		}
		replacements = append(replacements, lineReplacement{
			start: found,
			old:   len(pattern),
			new:   append([]string(nil), newLines...),
		})
		lineIndex = found + len(pattern)
	}
	sort.SliceStable(replacements, func(i int, j int) bool {
		return replacements[i].start < replacements[j].start
	})
	return replacements, nil
}

func seekLineSequence(lines []string, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}
	searchStart := start
	if eof && len(lines) >= len(pattern) {
		searchStart = len(lines) - len(pattern)
	}
	matchers := []func(string, string) bool{
		func(a string, b string) bool { return a == b },
		func(a string, b string) bool {
			return strings.TrimRightFunc(a, unicode.IsSpace) == strings.TrimRightFunc(b, unicode.IsSpace)
		},
		func(a string, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) },
		func(a string, b string) bool { return normalizePatchMatchLine(a) == normalizePatchMatchLine(b) },
	}
	for _, matcher := range matchers {
		for i := searchStart; i <= len(lines)-len(pattern); i++ {
			ok := true
			for j := range pattern {
				if !matcher(lines[i+j], pattern[j]) {
					ok = false
					break
				}
			}
			if ok {
				return i
			}
		}
	}
	return -1
}

func normalizePatchMatchLine(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			return '-'
		case '\u2018', '\u2019', '\u201A', '\u201B':
			return '\''
		case '\u201C', '\u201D', '\u201E', '\u201F':
			return '"'
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007',
			'\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			return ' '
		default:
			return r
		}
	}, strings.TrimSpace(s))
}

func applyLineReplacements(lines []string, replacements []lineReplacement) []string {
	result := append([]string(nil), lines...)
	for i := len(replacements) - 1; i >= 0; i-- {
		replacement := replacements[i]
		if replacement.start > len(result) {
			replacement.start = len(result)
		}
		end := min(replacement.start+replacement.old, len(result))
		next := make([]string, 0, len(result)-(end-replacement.start)+len(replacement.new))
		next = append(next, result[:replacement.start]...)
		next = append(next, replacement.new...)
		next = append(next, result[end:]...)
		result = next
	}
	return result
}

func joinPatchLines(lines []string, finalNewline bool) string {
	output := strings.Join(lines, "\n")
	if finalNewline && output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return output
}

func ensureRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	return nil
}

func ensureNotDirectory(path string) error {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func snapshotPatchPath(path string, snapshots map[string]*patchFileSnapshot) error {
	if _, ok := snapshots[path]; ok {
		return nil
	}
	snapshot := &patchFileSnapshot{}
	data, err := os.ReadFile(path)
	if err == nil {
		snapshot.before = data
		snapshot.existed = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read existing file for diff: %w", err)
	}
	snapshots[path] = snapshot
	return nil
}

func snapshotAfter(path string, snapshots map[string]*patchFileSnapshot) error {
	snapshot, ok := snapshots[path]
	if !ok {
		return nil
	}
	data, err := os.ReadFile(path)
	if err == nil {
		snapshot.after = data
		snapshot.afterSet = true
		return nil
	}
	if os.IsNotExist(err) {
		snapshot.after = nil
		snapshot.afterSet = true
		return nil
	}
	return fmt.Errorf("read updated file for diff: %w", err)
}

func patchResultMetadata(workspace string, snapshots map[string]*patchFileSnapshot, 
	affected map[string]string) map[string]string {
	metadata := map[string]string{
		"status":    "patched",
		"workspace": workspace,
	}
	paths := sortedPatchPaths(affected)
	if len(paths) > 0 {
		metadata["modified_files"] = strings.Join(paths, ",")
	}
	var diff strings.Builder
	for _, path := range sortedSnapshotPaths(snapshots) {
		snapshot := snapshots[path]
		if !snapshot.afterSet {
			continue
		}
		displayPath := path
		if rel, err := filepath.Rel(workspace, path); err == nil {
			displayPath = rel
		}
		existedAfter := true
		if _, err := os.Stat(path); err != nil {
			existedAfter = false
		}
		switch {
		case snapshot.existed && !existedAfter:
			diff.WriteString(writeFileDiff(displayPath, snapshot.before, nil, true))
		default:
			diff.WriteString(writeFileDiff(displayPath, snapshot.before, snapshot.after,
				snapshot.existed))
		}
	}
	if diff.String() != "" {
		metadata["diff"] = truncatePatchDiff(diff.String())
	}
	return metadata
}

func sortedPatchPaths(affected map[string]string) []string {
	paths := make([]string, 0, len(affected))
	for path := range affected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func sortedSnapshotPaths(snapshots map[string]*patchFileSnapshot) []string {
	paths := make([]string, 0, len(snapshots))
	for path := range snapshots {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func truncatePatchDiff(diff string) string {
	if len(diff) <= defaultWriteFileDiffMaxBytes {
		return diff
	}
	return "[diff omitted: apply_patch change exceeds 16384 bytes]\n"
}

func formatPatchSummary(affected map[string]string) string {
	paths := sortedPatchPaths(affected)
	if len(paths) == 0 {
		return "No files were modified."
	}
	return fmt.Sprintf("Applied patch to %d file(s): %s", len(paths), strings.Join(paths, ", "))
}

// Atomically writes files using temp files to avoid corruption mid-stream
func safeWriteFile(filePath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
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

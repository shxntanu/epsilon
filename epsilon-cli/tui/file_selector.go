package tui

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

const maxFileSelectorMatches = 8

var fileSelectorSkipDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"vendor":       true,
}

func (m model) fileSelectorState() (string, []string, bool) {
	query, ok := fileSelectorQuery(m.composer.ValueBeforeCursor())
	if !ok {
		return "", nil, false
	}
	return query, matchFilePaths(m.filePaths, query, maxFileSelectorMatches), true
}

func (m model) fileSelectorActive() bool {
	_, _, ok := m.fileSelectorState()
	return ok
}

func fileSelectorQuery(input string) (string, bool) {
	token, ok := activeFileSelectorToken(input)
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(token, "@"), true
}

func activeFileSelectorToken(input string) (string, bool) {
	if strings.Contains(input, "\n") {
		return "", false
	}
	if strings.TrimSpace(input) == "" || strings.HasSuffix(input, " ") || strings.HasSuffix(input, "\t") {
		return "", false
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false
	}

	token := fields[len(fields)-1]
	if !strings.HasPrefix(token, "@") || strings.HasPrefix(token, "@@") {
		return "", false
	}
	if strings.ContainsAny(token, " \t\n") {
		return "", false
	}
	return token, true
}

func (m model) fileSelectorHeight() int {
	_, matches, ok := m.fileSelectorState()
	if !ok {
		return 0
	}
	if len(matches) == 0 {
		return 5
	}
	return len(matches) + 4
}

func (m model) renderFileSelector() (string, bool) {
	query, matches, ok := m.fileSelectorState()
	if !ok {
		return "", false
	}

	width := max(20, m.width-4)
	lines := []string{m.renderSelectorHeader("Files", fileSelectorMeta(query, len(matches), len(m.filePaths)))}
	if len(m.filePaths) == 0 {
		lines = append(lines, m.styles.muted.Render("No files found"))
		lines = append(lines, m.renderSelectorHint("esc closes"))
		return m.fileSelectorStyle().Width(width).Render(strings.Join(lines, "\n")), true
	}
	if len(matches) == 0 {
		lines = append(lines, m.styles.muted.Render("No files match @"+query))
		lines = append(lines, m.renderSelectorHint("keep typing", "esc closes"))
		return m.fileSelectorStyle().Width(width).Render(strings.Join(lines, "\n")), true
	}

	selected := m.clampedFileCursor(len(matches))
	for i, path := range matches {
		line := m.formatFileRow(path, width-4)
		if i == selected {
			line = activeSelectorMarker(m.headerFrame) + " " + line
			line = m.styles.selectorActive.Width(max(1, width-4)).Render(line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, m.renderSelectorHint("type to search", "enter pastes", "esc closes"))

	return m.fileSelectorStyle().Width(width).Render(strings.Join(lines, "\n")), true
}

func fileSelectorMeta(query string, matches int, total int) string {
	if query == "" {
		return fmt.Sprintf("%d files", total)
	}
	return fmt.Sprintf("@%s · %d matches", query, matches)
}

func (m model) formatFileRow(path string, width int) string {
	name := m.styles.selectorKey.Render(path)
	dir := strings.Trim(strings.TrimSuffix(filepath.ToSlash(filepath.Dir(path)), "."), "/")
	if dir != "" {
		dir = m.styles.muted.Render(dir)
	}
	return fitSelectorRow(name, dir, width-2)
}

func (m *model) moveFileSelection(delta int) bool {
	_, matches, ok := m.fileSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	m.fileCursor = (m.clampedFileCursor(len(matches)) + delta + len(matches)) % len(matches)
	return true
}

func (m *model) completeFileSelection() bool {
	_, matches, ok := m.fileSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	selected := m.clampedFileCursor(len(matches))
	token, ok := activeFileSelectorToken(m.composer.ValueBeforeCursor())
	if !ok {
		return false
	}
	replacement := matches[selected]
	after := m.composer.ValueAfterCursor()
	if !strings.HasSuffix(replacement, " ") && !strings.HasPrefix(after, " ") &&
		!strings.HasPrefix(after, "\t") && !strings.HasPrefix(after, "\n") {
		replacement += " "
	}
	if !m.composer.ReplaceTokenBeforeCursor(token, replacement) {
		return false
	}
	m.fileCursor = 0
	m.resize()
	return true
}

func (m *model) closeFileSelector() bool {
	if !m.fileSelectorActive() {
		return false
	}
	token, ok := activeFileSelectorToken(m.composer.ValueBeforeCursor())
	if !ok || !m.composer.ReplaceTokenBeforeCursor(token, "") {
		return false
	}
	m.fileCursor = 0
	m.resize()
	return true
}

func (m model) clampedFileCursor(length int) int {
	if length <= 0 || m.fileCursor < 0 {
		return 0
	}
	if m.fileCursor >= length {
		return length - 1
	}
	return m.fileCursor
}

func (m model) fileSelectorStyle() lipgloss.Style {
	return m.styles.selector.BorderForeground(tuiAccentInfo)
}

func matchFilePaths(paths []string, query string, limit int) []string {
	type scoredPath struct {
		path  string
		score int
	}

	query = strings.ToLower(strings.TrimSpace(query))
	matches := make([]scoredPath, 0, len(paths))
	for _, path := range paths {
		score := fileSearchScore(path, query)
		if score <= 0 {
			continue
		}
		matches = append(matches, scoredPath{path: path, score: score})
	}
	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].path < matches[j].path
	})

	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	results := make([]string, 0, len(matches))
	for _, match := range matches {
		results = append(results, match.path)
	}
	return results
}

func fileSearchScore(path string, query string) int {
	if query == "" {
		return 1
	}
	path = strings.ToLower(strings.TrimSpace(filepath.ToSlash(path)))
	base := strings.ToLower(filepath.Base(path))
	best := fuzzyTextScore(path, query)
	if score := fuzzyTextScore(base, query); score > best {
		best = score + 50
	}
	return best
}

func listWorkspaceFiles() []string {
	if files, ok := listGitWorkspaceFiles(); ok {
		return files
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	var files []string
	_ = filepath.WalkDir(cwd, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != cwd && fileSelectorSkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil || rel == "." {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files
}

func listGitWorkspaceFiles() ([]string, bool) {
	cmd := exec.Command("git", "ls-files", "-co", "--exclude-standard", "-z")
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	parts := strings.Split(string(output), "\x00")
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		info, err := os.Stat(part)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, filepath.ToSlash(part))
	}
	sort.Strings(files)
	return files, true
}

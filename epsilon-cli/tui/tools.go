package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

var diffHunkRangePattern = regexp.MustCompile(`@@ -([0-9]+)(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)

func (m model) renderToolDetails(entry transcriptEntry, prefix string) string {
	lines := make([]string, 0, 4)
	if command := renderBashToolCommand(entry); command != "" {
		lines = append(lines, prefix+m.styles.muted.Render("$ ")+command)
	} else if entry.toolInput != "" {
		lines = append(lines, prefix+m.styles.muted.Render("input ")+strings.TrimSpace(entry.toolInput))
	}
	if metadata := formatMetadata(entry.toolMeta); metadata != "" {
		lines = append(lines, prefix+m.styles.muted.Render("metadata ")+metadata)
	}
	if entry.toolResult != "" {
		labelStyle := m.styles.muted
		if entry.toolError {
			labelStyle = m.styles.error
		}
		lines = append(lines, prefix+labelStyle.Render("result"))
		for _, line := range strings.Split(strings.TrimRight(entry.toolResult, "\n"), "\n") {
			lines = append(lines, prefix+"  "+m.styles.muted.Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

func renderToolTitle(entry transcriptEntry) string {
	if entry.kind == transcriptToolGroup {
		return entry.text
	}
	name := strings.TrimSpace(entry.toolName)
	if name == "" {
		return entry.text
	}
	if entry.toolError {
		return "failed " + name
	}
	if entry.toolActive {
		return "using " + name
	}
	if name == "bash" {
		return renderBashToolTitle(entry)
	}
	if isExplorationTool(name) {
		return renderExplorationToolTitle(entry)
	}
	return name
}

func (m model) renderToolEntry(entry transcriptEntry) string {
	title := m.renderToolEventTitle(entry, renderToolTitle(entry))
	lines := []string{title}
	if summary := renderToolSummary(entry); summary != "" && !m.showEvents {
		lines = append(lines, m.renderToolBranch(summary))
	}
	if m.showEvents {
		if detail := m.renderToolDetails(entry, "  "); detail != "" {
			lines = append(lines, detail)
		}
	}

	body := strings.Join(lines, "\n")
	return m.styles.toolBlock.Render(body)
}

func shouldCollapseDiffToolEntry(entry transcriptEntry, showEvents bool) bool {
	if showEvents || entry.toolActive || entry.toolError {
		return false
	}
	return strings.TrimSpace(entry.toolMeta["diff"]) != ""
}

func (m model) renderToolGroupEntry(entry transcriptEntry) string {
	titleText := renderExplorationTitle(entry)
	if m.showEvents {
		titleText = entry.text
	}
	lines := []string{m.renderToolEventTitle(entry, titleText)}
	if m.showEvents {
		for _, tool := range entry.tools {
			lines = append(lines, m.renderToolBranch(renderExplorationToolTitle(tool)))
			if detail := m.renderToolDetails(tool, "    "); detail != "" {
				lines = append(lines, detail)
			}
		}
	} else if summary := renderToolGroupSummary(entry); summary != "" {
		lines = append(lines, m.renderToolBranch(summary))
	}

	body := strings.Join(lines, "\n")
	return m.styles.toolBlock.Render(body)
}

func (m model) renderToolEventTitle(entry transcriptEntry, title string) string {
	marker := m.styles.tool.Render("•")
	if entry.toolActive {
		marker = m.styles.tool.Render(m.spinner.View())
	}
	if entry.toolError {
		marker = m.styles.error.Render("•")
	}

	style := m.styles.tool
	if entry.toolError {
		style = m.styles.error
	}
	return marker + " " + style.Bold(true).Render(title)
}

func (m model) renderToolBranch(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return m.styles.muted.Render("└  " + text)
}

func renderToolGroupSummary(entry transcriptEntry) string {
	if len(entry.tools) == 0 {
		return ""
	}
	failed := 0
	completed := 0
	for _, tool := range entry.tools {
		if tool.toolError {
			failed++
		}
		if !tool.toolActive {
			completed++
		}
	}
	if failed > 0 {
		return fmt.Sprintf("%d %s, %d failed", len(entry.tools),
			pluralize(len(entry.tools), "tool", "tools"), failed)
	}
	if completed < len(entry.tools) {
		return fmt.Sprintf("%d/%d complete", completed, len(entry.tools))
	}
	return fmt.Sprintf("%d %s", len(entry.tools), pluralize(len(entry.tools), "tool", "tools"))
}

func (m model) renderDiff(diff string) string {
	summary := summarizeDiff(diff)
	title := m.renderDiffTitle(summary)
	body := m.renderDiffBody(diff, summary)
	if body == "" {
		return title
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}

type diffSummary struct {
	path    string
	added   int
	removed int
}

func summarizeDiff(diff string) diffSummary {
	summary := diffSummary{path: "changes"}
	for _, line := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			if path := cleanDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ "))); path != "" &&
				path != "/dev/null" {
				summary.path = path
			}
		case strings.HasPrefix(line, "--- "):
			if summary.path == "changes" {
				if path := cleanDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "--- "))); path != "" &&
					path != "/dev/null" {
					summary.path = path
				}
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			summary.added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			summary.removed++
		}
	}
	return summary
}

func cleanDiffPath(path string) string {
	path = strings.Trim(path, "\"")
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func (m model) renderDiffTitle(summary diffSummary) string {
	verb := "Edited"
	if summary.removed == 0 && summary.added > 0 {
		verb = "Added"
	}
	if summary.added == 0 && summary.removed > 0 {
		verb = "Deleted"
	}

	counts := lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.styles.diffAdd.Render(fmt.Sprintf("+%d", summary.added)),
		m.styles.muted.Render(" "),
		m.styles.diffRemove.Render(fmt.Sprintf("-%d", summary.removed)),
	)
	return m.styles.tool.Render("•") + " " +
		lipgloss.NewStyle().Background(tuiBackground).Foreground(tuiInkStrong).Bold(true).Render(verb) +
		" " +
		lipgloss.NewStyle().Background(tuiBackground).Foreground(tuiInk).Render(summary.path) +
		" " +
		m.styles.muted.Render("(") + counts + m.styles.muted.Render(")")
}

func (m model) renderDiffBody(diff string, summary diffSummary) string {
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	rendered := make([]string, 0, len(lines))
	oldLine := 1
	newLine := 1
	lineNumberWidth := max(3, len(strconv.Itoa(max(1, maxChangedLine(lines)))))
	rowWidth := max(20, m.messageWidth())
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "*** ") ||
			strings.HasPrefix(line, "+++") ||
			strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "@@"):
			if parsedOld, parsedNew, ok := parseDiffHunkStart(line); ok {
				oldLine = parsedOld
				newLine = parsedNew
			}
			if strings.TrimSpace(line) != "@@" {
				rendered = append(rendered, m.styles.diffMeta.Render(line))
			}
		case strings.HasPrefix(line, "+"):
			rendered = append(rendered, m.renderDiffRow(newLine, "+", strings.TrimPrefix(line, "+"),
				lineNumberWidth, rowWidth, true))
			newLine++
		case strings.HasPrefix(line, "-"):
			rendered = append(rendered, m.renderDiffRow(oldLine, "-", strings.TrimPrefix(line, "-"),
				lineNumberWidth, rowWidth, false))
			oldLine++
		case strings.HasPrefix(line, `\ No newline at end of file`):
			rendered = append(rendered, m.styles.diffMeta.Render(line))
		default:
			text := line
			if strings.HasPrefix(text, " ") {
				text = strings.TrimPrefix(text, " ")
			}
			rendered = append(rendered, m.renderDiffContextRow(newLine, text, lineNumberWidth, rowWidth))
			oldLine++
			newLine++
		}
	}

	if len(rendered) == 0 && (summary.added > 0 || summary.removed > 0) {
		return m.styles.diffMeta.Render("diff omitted")
	}
	return strings.Join(rendered, "\n")
}

func parseDiffHunkStart(line string) (int, int, bool) {
	matches := diffHunkRangePattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return 0, 0, false
	}
	oldLine, oldErr := strconv.Atoi(matches[1])
	newLine, newErr := strconv.Atoi(matches[2])
	if oldErr != nil || newErr != nil {
		return 0, 0, false
	}
	return oldLine, newLine, true
}

func maxChangedLine(lines []string) int {
	oldLine := 1
	newLine := 1
	maxLine := 1
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if parsedOld, parsedNew, ok := parseDiffHunkStart(line); ok {
				oldLine = parsedOld
				newLine = parsedNew
				maxLine = max(maxLine, max(oldLine, newLine))
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			maxLine = max(maxLine, newLine)
			newLine++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			maxLine = max(maxLine, oldLine)
			oldLine++
		case strings.HasPrefix(line, "diff --git") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "+++"):
			continue
		default:
			maxLine = max(maxLine, max(oldLine, newLine))
			oldLine++
			newLine++
		}
	}
	return maxLine
}

func (m model) renderDiffRow(lineNumber int, marker string, text string, numberWidth int, rowWidth int,
	added bool) string {
	bg := lipgloss.Color("#12351F")
	fg := lipgloss.Color("#A7F3B8")
	markerColor := lipgloss.Color("#3FB950")
	if !added {
		bg = lipgloss.Color("#4A1717")
		fg = lipgloss.Color("#FFD0CC")
		markerColor = lipgloss.Color("#FF7B72")
	}
	number := fmt.Sprintf("%*d", numberWidth, lineNumber)
	content := lipgloss.NewStyle().Background(bg).Foreground(tuiMuted).Render(number) +
		" " +
		lipgloss.NewStyle().Background(bg).Foreground(markerColor).Bold(true).Render(marker) +
		" " +
		lipgloss.NewStyle().Background(bg).Foreground(fg).Render(text)
	return lipgloss.NewStyle().Background(bg).Width(rowWidth).Render(content)
}

func (m model) renderDiffContextRow(lineNumber int, text string, numberWidth int, rowWidth int) string {
	number := fmt.Sprintf("%*d", numberWidth, lineNumber)
	content := m.styles.muted.Render(number) + "   " + m.styles.muted.Render(text)
	return lipgloss.NewStyle().Background(tuiBackground).Width(rowWidth).Render(content)
}

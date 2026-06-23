package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

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
	body := m.renderDiffBody(diff)
	if m.density == densityCompact {
		return m.styles.tool.Render("Changes") + "\n" + body
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.tool.Render("Changes"),
		m.styles.diffBlock.Width(m.messageWidth()).Render(body),
	)
}

func (m model) renderDiffBody(diff string) string {
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "*** "):
			rendered = append(rendered, m.styles.diffHeader.Render(line))
		case strings.HasPrefix(line, "@@"):
			rendered = append(rendered, m.styles.diffMeta.Render(line))
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			rendered = append(rendered, m.styles.diffHeader.Render(line))
		case strings.HasPrefix(line, "+"):
			rendered = append(rendered, m.styles.diffAdd.Render(line))
		case strings.HasPrefix(line, "-"):
			rendered = append(rendered, m.styles.diffRemove.Render(line))
		default:
			rendered = append(rendered, m.styles.muted.Render(line))
		}
	}

	return strings.Join(rendered, "\n")
}

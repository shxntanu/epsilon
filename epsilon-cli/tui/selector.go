package tui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func (m model) renderSelectorHeader(title string, meta string) string {
	heading := m.styles.selectorTitle.Render("✦ " + title)
	if strings.TrimSpace(meta) == "" {
		return heading
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		heading,
		" ",
		m.styles.selectorMeta.Render(meta),
	)
}

func (m model) renderSelectorHint(parts ...string) string {
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rendered = append(rendered, m.styles.selectorHint.Render(part))
	}
	return strings.Join(rendered, m.styles.selectorHint.Render(" · "))
}

func activeSelectorMarker(frame int) string {
	frames := []string{"▸", "◆", "◇", "▹"}
	return frames[frame%len(frames)]
}

func fitSelectorRow(primary string, secondary string, width int) string {
	width = max(1, width)
	if strings.TrimSpace(secondary) == "" {
		return fitANSI(primary, width)
	}

	line := primary + "  " + secondary
	if lipgloss.Width(line) <= width {
		return line
	}
	if lipgloss.Width(primary) <= width {
		return primary
	}
	return fitANSI(primary, width)
}

func fitANSI(text string, width int) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	plain := ansiEscapePattern.ReplaceAllString(text, "")
	return fitHeaderText(plain, width)
}

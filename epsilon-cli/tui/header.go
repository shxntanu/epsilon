package tui

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const headerTickInterval = 140 * time.Millisecond

type headerTickMsg struct{}

func headerTickCmd() tea.Cmd {
	return tea.Tick(headerTickInterval, func(time.Time) tea.Msg {
		return headerTickMsg{}
	})
}

func (m *model) startHeaderAnimation() tea.Cmd {
	if !m.shouldAnimateHeader() || m.headerAnimating {
		return nil
	}
	m.headerAnimating = true
	return headerTickCmd()
}

func (m model) shouldAnimateHeader() bool {
	status := strings.TrimSpace(m.status)
	return m.busy || m.showSpinner || m.hasActiveTools() || m.permission != nil ||
		m.modelPicker != nil || m.sessionPicker != nil || m.slashSelectorActive() ||
		status == "thinking" || status == "approval" || status == "resuming" ||
		hasStatusPrefix(status, "models") || hasStatusPrefix(status, "sessions")
}

func (m model) renderHeaderText(width int) string {
	logo := m.renderLogo()
	status := m.renderStatusBadge()
	innerWidth := max(20, width-2)
	sessionWidth := innerWidth - lipgloss.Width(logo) - lipgloss.Width(status) - 2

	parts := []string{logo}
	if sessionWidth >= 10 {
		parts = append(parts, m.renderSessionBadge(sessionWidth))
	}
	parts = append(parts, status)

	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

func (m model) renderLogo() string {
	marks := []string{"✶", "✷", "✸", "✹"}
	colors := []color.Color{tuiAccentAgent, tuiAccentInfo, tuiAccentModel, tuiAccentOK}
	frame := m.headerFrame % len(marks)
	mark := marks[0]
	color := tuiAccentAgent
	if m.shouldAnimateHeader() {
		mark = marks[frame]
		color = colors[frame]
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.NewStyle().
			Background(tuiSurface).
			Foreground(color).
			Bold(true).
			Padding(0, 1).
			Render(mark),
		lipgloss.NewStyle().
			Background(tuiSurface2).
			Foreground(tuiInkStrong).
			Bold(true).
			Padding(0, 1).
			Render("Epsilon"),
	)
}

func (m model) renderSessionBadge(width int) string {
	prefix := "session "
	label := strings.TrimSpace(m.currentSessionLabel())
	if label == "" {
		label = "untitled"
	}
	availableLabel := max(1, width-len(prefix)-2)
	label = fitHeaderText(label, availableLabel)

	return lipgloss.NewStyle().
		Background(tuiSurface).
		Foreground(tuiSubtle).
		Padding(0, 1).
		Render(prefix) +
		lipgloss.NewStyle().
			Background(tuiSurface).
			Foreground(tuiAccentAgent).
			Bold(m.shouldAnimateHeader()).
			Render(label)
}

func (m model) renderStatusBadge() string {
	status := strings.TrimSpace(m.status)
	if status == "" || status == "ready" {
		return statusBadgeStyle("ready").Render("● Ready")
	}

	return statusBadgeStyle(status).Render(m.statusIcon(status) + " " + status)
}

func (m model) statusIcon(status string) string {
	if status == "thinking" || status == "approval" || status == "resuming" ||
		hasStatusPrefix(status, "models") || hasStatusPrefix(status, "sessions") {
		frames := []string{"◐", "◓", "◑", "◒"}
		return frames[m.headerFrame%len(frames)]
	}
	switch {
	case status == "error" || hasStatusPrefix(status, "models:error") ||
		hasStatusPrefix(status, "sessions:error"):
		return "×"
	case hasStatusPrefix(status, "model:") || hasStatusPrefix(status, "effort:"):
		return "◆"
	case hasStatusPrefix(status, "details:") || hasStatusPrefix(status, "density:") ||
		hasStatusPrefix(status, "background:"):
		return "◇"
	case hasStatusPrefix(status, "mouse:") || hasStatusPrefix(status, "view:"):
		return "↕"
	default:
		return "•"
	}
}

func fitHeaderText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return "…"
	}
	return string(runes) + "…"
}

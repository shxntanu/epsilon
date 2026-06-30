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
	sessionWidth := max(0, innerWidth-lipgloss.Width(logo)-lipgloss.Width(status)-2)
	session := ""
	if sessionWidth >= 12 {
		session = m.renderSessionBadge(sessionWidth)
	}

	leftWidth := lipgloss.Width(logo)
	rightWidth := lipgloss.Width(status)
	sessionRenderedWidth := lipgloss.Width(session)
	gap := max(1, innerWidth-leftWidth-rightWidth-sessionRenderedWidth)
	leftGap := gap / 2
	rightGap := gap - leftGap

	return logo +
		strings.Repeat(" ", leftGap) +
		session +
		strings.Repeat(" ", rightGap) +
		status
}

func (m model) renderStartupHeader(width int) string {
	contentWidth := max(34, width-4)
	if contentWidth > 96 {
		contentWidth = 96
	}

	title := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.renderLogoMark(),
		" ",
		lipgloss.NewStyle().
			Foreground(tuiInkStrong).
			Bold(true).
			Render("Epsilon"),
		" ",
		m.styles.muted.Render("agent harness"),
	)

	status := m.renderStatusBadge()
	session := "session " + m.currentSessionLabel()
	if lipgloss.Width(session) > contentWidth-lipgloss.Width(status)-3 {
		session = "session " + fitHeaderText(m.currentSessionLabel(),
			max(8, contentWidth-lipgloss.Width(status)-11))
	}

	modelLine := m.startupModelLine()
	if modelLine == "" {
		modelLine = "ready for local code work"
	}

	left := lipgloss.NewStyle().
		Width(max(16, contentWidth/3)).
		Align(lipgloss.Center).
		Render(strings.Join([]string{
			m.styles.title.Render("Welcome back"),
			"",
			m.renderLogoMarkLarge(),
			"",
			m.styles.muted.Render(modelLine),
		}, "\n"))

	rightWidth := max(24, contentWidth-lipgloss.Width(left)-4)
	right := lipgloss.NewStyle().
		Width(rightWidth).
		Render(strings.Join([]string{
			lipgloss.JoinHorizontal(lipgloss.Center,
				m.styles.tool.Bold(true).Render("Native terminal mode"),
				" ",
				status,
			),
			m.styles.muted.Render("Clean scrollback stays concise while the managed view holds rich state."),
			"",
			m.styles.tool.Render("Ctrl+O") + " " + m.styles.muted.Render("opens the detailed transcript"),
			m.styles.tool.Render("Ctrl+G") + " " + m.styles.muted.Render("switches to application view"),
			m.styles.tool.Render("/") + " " + m.styles.muted.Render("runs local commands"),
		}, "\n"))

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		left,
		m.styles.muted.Render(" │ "),
		right,
	)

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(tuiLine).
		Foreground(tuiInk).
		Background(tuiBackground).
		Padding(1, 2).
		Width(contentWidth).
		Render(title + "\n\n" + body)
}

func (m model) startupModelLine() string {
	model := strings.TrimSpace(currentString(m.currentModel))
	effort := strings.TrimSpace(currentString(m.currentEffort))
	if model == "" && effort == "" {
		return ""
	}
	if model == "" {
		return "effort " + effort
	}
	if effort == "" || effort == "off" {
		return model
	}
	return model + " · " + effort
}

func (m model) renderLogo() string {
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.renderLogoMark(),
		" ",
		lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiInkStrong).
			Bold(true).
			Render("Epsilon"),
	)
}

func (m model) renderLogoMark() string {
	marks := []string{"✶", "✷", "✸", "✹"}
	colors := []color.Color{tuiAccentAgent, tuiAccentInfo, tuiAccentModel, tuiAccentOK}
	frame := m.headerFrame % len(marks)
	mark := marks[0]
	color := tuiAccentAgent
	if m.shouldAnimateHeader() {
		mark = marks[frame]
		color = colors[frame]
	}

	return lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(color).
		Bold(true).
		Render(mark)
}

func (m model) renderLogoMarkLarge() string {
	mark := []string{
		"  ▄██▄  ",
		" ██  ██ ",
		" ██▄▄██ ",
		"  ▀██▀  ",
	}
	return lipgloss.NewStyle().
		Foreground(tuiAccentAgent).
		Bold(true).
		Render(strings.Join(mark, "\n"))
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

package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core/slash"
)

const maxSlashSelectorMatches = 6

func (m *model) applySlashResult(result slash.Result, err error) tea.Cmd {
	m.followOutput = true
	m.dirty = true
	m.showSpinner = false
	m.resize()

	if err != nil {
		message := err.Error()
		if errors.Is(err, slash.ErrUnknownCommand) {
			message += ". Type /help for commands."
		}
		m.status = "ready"
		m.appendPlain(m.styles.error.Render(message))
		return nil
	}

	switch result.Action {
	case slash.ActionClearTranscript:
		m.entries = nil
		m.streaming = -1
		m.status = "ready"
		if result.Message != "" {
			m.appendPlain(m.styles.muted.Render(result.Message))
		}
	case slash.ActionSetEvents:
		m.showEvents = result.Bool
		m.status = "details:" + onOff(m.showEvents)
		m.appendSlashMessage(result.Message)
	case slash.ActionSetDensity:
		m.density = densityModeFromSlash(result.Density)
		m.resize()
		m.status = "density:" + m.density.Label()
		m.appendSlashMessage(result.Message)
	case slash.ActionPickModel:
		return m.openModelPicker()
	case slash.ActionPickSession:
		return m.openSessionPicker()
	case slash.ActionSetModel:
		if m.setModel == nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("model changes are not available"))
			return nil
		}
		if err := m.setModel(m.ctx, result.Model); err != nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render(err.Error()))
			return nil
		}
		m.status = "model:" + result.Model
		m.appendSlashMessage(result.Message)
	case slash.ActionSetEffort:
		if m.setEffort == nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("effort changes are not available"))
			return nil
		}
		if err := m.setEffort(result.Effort); err != nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render(err.Error()))
			return nil
		}
		statusEffort := result.Effort
		if statusEffort == "" {
			statusEffort = "default"
		}
		m.status = "effort:" + statusEffort
		m.appendSlashMessage(result.Message)
	case slash.ActionResumeChat:
		return m.resumeChat(result.Session)
	case slash.ActionQuit:
		return quitCmd()
	default:
		m.status = "ready"
		m.appendSlashMessage(result.Message)
	}

	return nil
}

func (m *model) appendSlashMessage(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}

	m.appendPlain(message)
}

func (m model) slashExecution() slash.Execution {
	return slash.Execution{
		SessionID: m.session.ID(),
		State: slash.State{
			Busy:             m.busy,
			AwaitingApproval: m.permission != nil,
			EventsVisible:    m.showEvents,
			Density:          slashDensity(m.density),
			MessageCount:     len(m.session.Messages()),
			Model:            currentString(m.currentModel),
			Effort:           currentString(m.currentEffort),
		},
	}
}

func currentString(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

func slashDensity(density densityMode) slash.Density {
	if density == densityCompact {
		return slash.DensityCompact
	}

	return slash.DensityComfortable
}

func densityModeFromSlash(density slash.Density) densityMode {
	if density == slash.DensityCompact {
		return densityCompact
	}

	return densityComfortable
}

func (m model) slashSelectorState() (string, []slash.Match, bool) {
	query, ok := slashSelectorQuery(m.chatBox.Value())
	if !ok || m.slash == nil {
		return "", nil, false
	}

	matches := m.slash.Match(query, maxSlashSelectorMatches)
	if len(matches) == 0 {
		return query, nil, true
	}
	return query, matches, true
}

func slashSelectorQuery(input string) (string, bool) {
	text := strings.TrimLeft(input, " \t")
	if text == "" || !strings.HasPrefix(text, "/") || strings.HasPrefix(text, "//") {
		return "", false
	}
	if strings.Contains(text, "\n") {
		return "", false
	}

	body := strings.TrimPrefix(text, "/")
	if strings.ContainsAny(body, " \t") {
		return "", false
	}
	return body, true
}

func (m model) slashSelectorHeight() int {
	_, matches, ok := m.slashSelectorState()
	if !ok {
		return 0
	}
	if len(matches) == 0 {
		return 4
	}
	return len(matches) + 3
}

func (m model) renderSlashSelector() (string, bool) {
	query, matches, ok := m.slashSelectorState()
	if !ok {
		return "", false
	}

	width := max(20, m.width-4)
	lines := []string{m.styles.selectorTitle.Render("Slash commands")}
	if len(matches) == 0 {
		lines = append(lines, m.styles.muted.Render("No commands match /"+query))
		return m.styles.selector.Width(width).Render(strings.Join(lines, "\n")), true
	}

	selected := m.clampedSlashCursor(len(matches))
	for i, match := range matches {
		line := "/" + match.Command.Name
		if match.Command.Description != "" {
			line += " " + m.styles.muted.Render(match.Command.Description)
		}
		if match.Alias != "" && match.Alias != match.Command.Name {
			line += " " + m.styles.muted.Render("alias /"+match.Alias)
		}
		if i == selected {
			line = m.styles.selectorActive.Render(line)
		}
		lines = append(lines, line)
	}

	return m.styles.selector.Width(width).Render(strings.Join(lines, "\n")), true
}

func (m *model) moveSlashSelection(delta int) bool {
	_, matches, ok := m.slashSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	m.slashCursor = (m.clampedSlashCursor(len(matches)) + delta + len(matches)) % len(matches)
	return true
}

func (m *model) completeSlashSelection() bool {
	_, matches, ok := m.slashSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	selected := m.clampedSlashCursor(len(matches))
	m.chatBox.SetValue("/" + matches[selected].Command.Name + " ")
	m.slashCursor = 0
	m.resize()
	return true
}

func (m *model) completeIncompleteSlashInput() bool {
	query, _, ok := m.slashSelectorState()
	if !ok {
		return false
	}

	if _, exact := m.slash.Get(query); exact {
		return false
	}

	return m.completeSlashSelection()
}

func (m model) clampedSlashCursor(length int) int {
	if length <= 0 || m.slashCursor < 0 {
		return 0
	}
	if m.slashCursor >= length {
		return length - 1
	}
	return m.slashCursor
}

func selectorStyles(base styles) styles {
	base.selector = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Background(lipgloss.Color("234")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)
	base.selectorActive = lipgloss.NewStyle().
		Background(lipgloss.Color("238")).
		Foreground(lipgloss.Color("252")).
		Bold(true)
	base.selectorTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("146")).
		Bold(true)
	return base
}

package tui

import (
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core/types"
)

type permissionPrompt struct {
	request types.PermissionRequest
	choice  int
	width   int
	styles  permissionPromptStyles
}

type permissionPromptStyles struct {
	box      lipgloss.Style
	title    lipgloss.Style
	body     lipgloss.Style
	muted    lipgloss.Style
	selected lipgloss.Style
	option   lipgloss.Style
}

func newPermissionPrompt(request types.PermissionRequest) permissionPrompt {
	return permissionPrompt{
		request: request,
		choice:  0,
		styles: permissionPromptStyles{
			box: lipgloss.NewStyle().
				Background(tuiBackground).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(tuiAccentWarn).
				Padding(0, 1),
			title: lipgloss.NewStyle().
				Background(tuiBackground).
				Bold(true).
				Foreground(tuiInkStrong),
			body: lipgloss.NewStyle().
				Background(tuiBackground).
				Foreground(tuiInk),
			muted: lipgloss.NewStyle().
				Background(tuiBackground).
				Foreground(tuiSubtle),
			selected: lipgloss.NewStyle().
				Foreground(tuiInkStrong).
				Background(tuiSelected).
				Bold(true).
				Padding(0, 1),
			option: lipgloss.NewStyle().
				Background(tuiBackground).
				Foreground(tuiMuted).
				Padding(0, 1),
		},
	}
}

func (p permissionPrompt) Update(msg tea.KeyPressMsg) permissionPrompt {
	switch msg.Keystroke() {
	case "left", "up", "shift+tab":
		p.choice = (p.choice + 2) % 3
	case "right", "down", "tab":
		p.choice = (p.choice + 1) % 3
	}

	return p
}

func (p permissionPrompt) Decision() types.PermissionDecision {
	switch p.choice {
	case 0:
		return types.PermissionDecisionAllow
	case 1:
		return types.PermissionDecisionAllowSession
	}
	return types.PermissionDecisionDeny
}

func (p permissionPrompt) Reason() string {
	switch p.choice {
	case 0:
		return "allowed from TUI"
	case 1:
		return "allowed for this session from TUI"
	}
	return "denied from TUI"
}

func (p *permissionPrompt) SetWidth(width int) {
	p.width = width
}

func (p permissionPrompt) View() string {
	options := []string{"Allow once", "Allow session", "Deny"}
	rendered := make([]string, len(options))
	for i, option := range options {
		rendered[i] = p.styles.option.Render(option)
		if p.choice == i {
			rendered[i] = p.selectedStyle(i).Render(option)
		}
	}

	input := strings.TrimSpace(string(p.request.Input))
	if formatted := compactJSON(input); formatted != "" {
		input = formatted
	}
	if len(input) > 320 {
		input = input[:320] + "..."
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		p.styles.title.Render("Approve tool call?"),
		p.styles.body.Render(p.request.ToolName),
		p.styles.muted.Render(input),
		lipgloss.JoinHorizontal(lipgloss.Left, rendered[0], " ", rendered[1], " ", rendered[2]),
		p.styles.muted.Render("arrows choose | enter confirm | esc deny"),
	)

	return p.styles.box.Width(max(20, p.width-4)).Render(content)
}

func (p permissionPrompt) selectedStyle(choice int) lipgloss.Style {
	style := p.styles.selected
	switch choice {
	case 0:
		return style.Foreground(tuiBackground).Background(tuiStatusReady)
	case 1:
		return style.Foreground(tuiBackground).Background(tuiStatusWorking)
	case 2:
		return style.Background(tuiStatusDanger)
	default:
		return style
	}
}

func compactJSON(value string) string {
	if value == "" {
		return ""
	}

	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return ""
	}

	data, err := json.Marshal(decoded)
	if err != nil {
		return ""
	}

	return string(data)
}

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
	allow   bool
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
		allow:   true,
		styles: permissionPromptStyles{
			box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("242")).
				Padding(0, 1),
			title: lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("250")),
			body:  lipgloss.NewStyle().Foreground(lipgloss.Color("248")),
			muted: lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
			selected: lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("239")).
				Padding(0, 1),
			option: lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 1),
		},
	}
}

func (p permissionPrompt) Update(msg tea.KeyPressMsg) permissionPrompt {
	switch msg.Keystroke() {
	case "left", "right", "up", "down", "tab", "shift+tab":
		p.allow = !p.allow
	}

	return p
}

func (p permissionPrompt) Decision() types.PermissionDecision {
	if p.allow {
		return types.PermissionDecisionAllow
	}

	return types.PermissionDecisionDeny
}

func (p permissionPrompt) Reason() string {
	if p.allow {
		return "allowed from TUI"
	}

	return "denied from TUI"
}

func (p *permissionPrompt) SetWidth(width int) {
	p.width = width
}

func (p permissionPrompt) View() string {
	allow := p.styles.option.Render("Allow")
	deny := p.styles.option.Render("Deny")
	if p.allow {
		allow = p.styles.selected.Render("Allow")
	} else {
		deny = p.styles.selected.Render("Deny")
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
		lipgloss.JoinHorizontal(lipgloss.Left, allow, " ", deny),
		p.styles.muted.Render("arrows choose | enter confirm | esc deny"),
	)

	return p.styles.box.Width(max(20, p.width-4)).Render(content)
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

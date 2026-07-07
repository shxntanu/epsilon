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
	box       lipgloss.Style
	title     lipgloss.Style
	meta      lipgloss.Style
	body      lipgloss.Style
	muted     lipgloss.Style
	command   lipgloss.Style
	selected  lipgloss.Style
	option    lipgloss.Style
	riskBadge lipgloss.Style
}

func newPermissionPrompt(request types.PermissionRequest) permissionPrompt {
	return permissionPrompt{
		request: request,
		choice:  0,
		styles: permissionPromptStyles{
			box: lipgloss.NewStyle().
				Background(tuiSurface).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(tuiLineStrong).
				Padding(1, 2),
			title: lipgloss.NewStyle().
				Background(tuiSurface).
				Bold(true).
				Foreground(tuiInkStrong),
			meta: lipgloss.NewStyle().
				Background(tuiSurface2).
				Foreground(tuiAccentInfo).
				Padding(0, 1),
			body: lipgloss.NewStyle().
				Background(tuiSurface).
				Foreground(tuiInk),
			muted: lipgloss.NewStyle().
				Background(tuiSurface).
				Foreground(tuiSubtle),
			command: lipgloss.NewStyle().
				Background(tuiSurface2).
				Foreground(tuiInk).
				Padding(0, 1),
			selected: lipgloss.NewStyle().
				Foreground(tuiInkStrong).
				Background(tuiSelected).
				Bold(true).
				Padding(0, 1),
			option: lipgloss.NewStyle().
				Background(tuiSurface).
				Foreground(tuiMuted).
				Padding(0, 1),
			riskBadge: lipgloss.NewStyle().
				Foreground(tuiInkStrong).
				Background(tuiSelected).
				Bold(true).
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

func (p permissionPrompt) Height() int {
	return 11 + len(p.previewLines()) // content + border/padding, with preview wrapping
}

func (p permissionPrompt) View() string {
	options := []string{"✓ Allow once", "◇ Allow session", "✕ Deny"}
	rendered := make([]string, len(options))
	for i, option := range options {
		rendered[i] = p.styles.option.Render(option)
		if p.choice == i {
			rendered[i] = p.selectedStyle(i).Render(option)
		}
	}

	width := min(max(44, p.width-4), 118)
	innerWidth := max(20, width-8)
	title := lipgloss.JoinHorizontal(
		lipgloss.Center,
		p.styles.title.Render("Approve tool call?"),
		" ",
		p.styles.meta.Render(permissionToolMeta(p.request)),
		" ",
		p.styles.riskBadge.Render("review"),
	)
	commandPreview := p.styles.command.
		Width(innerWidth).
		Render(strings.Join(p.previewLines(), "\n"))
	actionRow := lipgloss.JoinHorizontal(lipgloss.Left, rendered[0], " ", rendered[1], " ", rendered[2])
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		p.styles.body.Render("Tool access is paused until you choose how to proceed."),
		"",
		p.styles.muted.Render("request"),
		commandPreview,
		"",
		actionRow,
		p.styles.muted.Render("←/→ choose · enter confirm · esc deny"),
	)

	return p.styles.box.
		Width(width).
		Render(content)
}

func (p permissionPrompt) previewLines() []string {
	input := strings.TrimSpace(string(p.request.Input))
	if formatted := compactJSON(input); formatted != "" {
		input = formatted
	}
	if input == "" {
		input = "{}"
	}
	if len(input) > 360 {
		input = input[:360] + "..."
	}

	width := max(12, max(28, p.width-4)-10)
	lines := strings.Split(lipgloss.Wrap(input, width, " "), "\n")
	const maxLines = 3
	if len(lines) <= maxLines {
		return lines
	}
	lines = lines[:maxLines]
	last := strings.TrimRight(lines[len(lines)-1], ". ")
	lines[len(lines)-1] = last + "..."
	return lines
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

func permissionToolMeta(request types.PermissionRequest) string {
	tool := strings.TrimSpace(request.ToolName)
	if tool == "" {
		tool = "tool"
	}
	mode := strings.TrimSpace(string(request.Mode))
	if mode == "" {
		mode = "ask"
	}
	return tool + " · " + mode
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

package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	compactChatBoxInputHeight     = 3
	comfortableChatBoxInputHeight = 5
)

// composer is used to compose messages, run slash commands, etc.
type composer struct {
	input textarea.Model
	width int
	style lipgloss.Style
}

func newComposer() composer {
	input := textarea.New()
	input.Placeholder = "Message epsilon..."
	input.Prompt = "┃ "
	input.ShowLineNumbers = false
	input.EndOfBufferCharacter = ' '
	input.SetHeight(comfortableChatBoxInputHeight)
	input.Focus()

	styles := textarea.DefaultDarkStyles()
	styles.Focused.Base = styles.Focused.Base.Background(tuiBackground)
	styles.Focused.Text = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiInk)
	styles.Focused.Prompt = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiAccentAgent).
		Bold(true)
	styles.Focused.CursorLine = lipgloss.NewStyle().Background(tuiSurface2)
	styles.Focused.Placeholder = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiSubtle).
		Italic(true)
	styles.Focused.EndOfBuffer = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiSurface2)
	styles.Blurred.Base = styles.Blurred.Base.Background(tuiBackground)
	styles.Blurred.Text = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiInk)
	styles.Blurred.Prompt = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiSubtle)
	styles.Blurred.CursorLine = lipgloss.NewStyle().Background(tuiBackground)
	styles.Blurred.Placeholder = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiSubtle).
		Italic(true)
	styles.Blurred.EndOfBuffer = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiSurface2)
	styles.Cursor.Color = tuiAccentAgent
	input.SetStyles(styles)

	return composer{
		input: input,
		style: lipgloss.NewStyle().
			Background(tuiBackground).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tuiLineStrong).
			Foreground(tuiInk).
			Padding(0, 1),
	}
}

func (c composer) Init() tea.Cmd {
	return c.input.Focus()
}

func (c composer) Update(msg tea.Msg) (composer, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.Keystroke() {
		case "shift+enter", "alt+enter", "ctrl+j":
			c.input.InsertString("\n")
			return c, nil
		}
	}

	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func (c composer) View(slashMode bool, frame int) string {
	style := c.style
	modeColor := tuiAccentAgent
	modeLabel := "message"
	modeHint := "enter sends · shift+enter newline"
	modeMark := "✦"
	if slashMode {
		modeColor = tuiAccentCommand
		modeLabel = "command"
		modeHint = "tab completes · enter runs"
		modeMark = activeSelectorMarker(frame)
	}

	mode := lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.NewStyle().
			Background(tuiSurface).
			Foreground(modeColor).
			Bold(true).
			Padding(0, 1).
			Render(modeMark+" "+modeLabel),
		" ",
		lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiFaint).
			Render(modeHint),
	)
	if slashMode {
		style = style.BorderForeground(tuiAccentCommand)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, mode, c.input.View())
	return style.Width(max(0, c.width-4)).Render(content)
}

func (c composer) Value() string {
	return c.input.Value()
}

func (c *composer) SetValue(value string) {
	c.input.SetValue(value)
	c.input.CursorEnd()
}

func (c *composer) SetWidth(width int) {
	c.width = width
	c.input.SetWidth(max(20, width-10))
}

func (c *composer) SetDensity(density densityMode) {
	height := comfortableChatBoxInputHeight
	if density == densityCompact {
		height = compactChatBoxInputHeight
	}
	c.input.SetHeight(height)
}

func (c composer) Height() int {
	return c.input.Height() + 3
}

func (c *composer) Submit() (string, bool) {
	text := strings.TrimSpace(c.input.Value())
	if text == "" {
		return "", false
	}

	c.input.Reset()
	return text, true
}

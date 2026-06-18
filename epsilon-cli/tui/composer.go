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
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.EndOfBufferCharacter = ' '
	input.SetHeight(comfortableChatBoxInputHeight)
	input.Focus()

	styles := textarea.DefaultDarkStyles()
	styles.Focused.Base = styles.Focused.Base.Background(tuiBackground)
	styles.Focused.Text = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiInk)
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

func (c composer) View() string {
	return c.style.Width(max(0, c.width-4)).Render(c.input.View())
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
	c.input.SetWidth(max(20, width-8))
}

func (c *composer) SetDensity(density densityMode) {
	height := comfortableChatBoxInputHeight
	if density == densityCompact {
		height = compactChatBoxInputHeight
	}
	c.input.SetHeight(height)
}

func (c composer) Height() int {
	return c.input.Height() + 2
}

func (c *composer) Submit() (string, bool) {
	text := strings.TrimSpace(c.input.Value())
	if text == "" {
		return "", false
	}

	c.input.Reset()
	return text, true
}

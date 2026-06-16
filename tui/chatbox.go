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

type chatBox struct {
	input textarea.Model
	width int
	style lipgloss.Style
}

func newChatBox() chatBox {
	input := textarea.New()
	input.Placeholder = "Message epsilon..."
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.EndOfBufferCharacter = ' '
	input.SetHeight(comfortableChatBoxInputHeight)
	input.Focus()

	styles := textarea.DefaultDarkStyles()
	styles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styles.Focused.CursorLine = lipgloss.NewStyle().Background(lipgloss.Color("235"))
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	styles.Focused.EndOfBuffer = lipgloss.NewStyle().Foreground(lipgloss.Color("235"))
	styles.Cursor.Color = lipgloss.Color("146")
	input.SetStyles(styles)

	return chatBox{
		input: input,
		style: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("241")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),
	}
}

func (c chatBox) Init() tea.Cmd {
	return c.input.Focus()
}

func (c chatBox) Update(msg tea.Msg) (chatBox, tea.Cmd) {
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

func (c chatBox) View() string {
	return c.style.Width(max(0, c.width-4)).Render(c.input.View())
}

func (c *chatBox) SetWidth(width int) {
	c.width = width
	c.input.SetWidth(max(20, width-8))
}

func (c *chatBox) SetDensity(density densityMode) {
	height := comfortableChatBoxInputHeight
	if density == densityCompact {
		height = compactChatBoxInputHeight
	}
	c.input.SetHeight(height)
}

func (c chatBox) Height() int {
	return c.input.Height() + 2
}

func (c *chatBox) Submit() (string, bool) {
	text := strings.TrimSpace(c.input.Value())
	if text == "" {
		return "", false
	}

	c.input.Reset()
	return text, true
}

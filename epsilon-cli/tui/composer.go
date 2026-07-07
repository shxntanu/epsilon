package tui

import (
	"strings"
	"unicode/utf8"

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
		if composerNewlineKey(key) {
			var cmd tea.Cmd
			c.input, cmd = c.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			return c, cmd
		}
		if c.handleCommandBackspace(key) {
			return c, nil
		}
		if c.handleCommandArrow(key) {
			return c, nil
		}
	}

	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func composerNewlineKey(key tea.KeyPressMsg) bool {
	switch key.Keystroke() {
	case "shift+enter", "alt+enter", "ctrl+j":
		return true
	}
	return key.Code == tea.KeyEnter && key.Mod.Contains(tea.ModShift)
}

func (c *composer) handleCommandArrow(key tea.KeyPressMsg) bool {
	switch {
	case key.Code == tea.KeyLeft && key.Mod&tea.ModMeta != 0,
		key.Keystroke() == "meta+left",
		key.Keystroke() == "cmd+left",
		key.Keystroke() == "super+left":
		c.input.CursorStart()
		return true
	case key.Code == tea.KeyRight && key.Mod&tea.ModMeta != 0,
		key.Keystroke() == "meta+right",
		key.Keystroke() == "cmd+right",
		key.Keystroke() == "super+right":
		c.input.CursorEnd()
		return true
	default:
		return false
	}
}

func (c *composer) handleCommandBackspace(key tea.KeyPressMsg) bool {
	switch {
	case key.Code == tea.KeyBackspace && key.Mod&tea.ModMeta != 0,
		key.Keystroke() == "meta+backspace",
		key.Keystroke() == "cmd+backspace",
		key.Keystroke() == "super+backspace",
		key.Keystroke() == "ctrl+u":
		column := c.input.Column()
		for range column {
			var cmd tea.Cmd
			c.input, cmd = c.input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
			_ = cmd
		}
		return true
	default:
		return false
	}
}

func (c composer) View(slashMode bool, skillMode bool, fileMode bool, skillPrefix string,
	frame int, sessionLabel string) string {
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
	} else if skillMode {
		modeColor = tuiAccentTool
		modeLabel = "! skill"
		modeHint = "tab completes · enter selects · esc closes"
		modeMark = activeSelectorMarker(frame)
	} else if fileMode {
		modeColor = tuiAccentInfo
		modeLabel = "@ file"
		modeHint = "arrows select · enter pastes · esc closes"
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
	mode = c.modeLine(mode, sessionLabel)
	if slashMode {
		style = style.BorderForeground(tuiAccentCommand)
	} else if skillMode || strings.TrimSpace(skillPrefix) != "" {
		style = style.BorderForeground(tuiAccentTool)
	} else if fileMode {
		style = style.BorderForeground(tuiAccentInfo)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, mode, c.inputView(skillPrefix))
	return style.Width(max(0, c.width-4)).Render(content)
}

func (c composer) modeLine(left string, sessionLabel string) string {
	contentWidth := max(20, c.width-8)
	sessionLabel = strings.TrimSpace(sessionLabel)
	if sessionLabel == "" {
		sessionLabel = "untitled"
	}
	badgePrefix := "session "
	availableLabel := max(1, contentWidth-lipgloss.Width(left)-lipgloss.Width(badgePrefix)-4)
	badgeLabel := fitHeaderText(sessionLabel, availableLabel)
	badge := lipgloss.NewStyle().
		Background(tuiSurface).
		Foreground(tuiSubtle).
		Padding(0, 1).
		Render(badgePrefix) +
		lipgloss.NewStyle().
			Background(tuiSurface).
			Foreground(tuiAccentAgent).
			Bold(true).
			Render(badgeLabel)

	leftWidth := lipgloss.Width(left)
	badgeWidth := lipgloss.Width(badge)
	if leftWidth+badgeWidth+1 > contentWidth {
		return left
	}
	return left + strings.Repeat(" ", max(1, contentWidth-leftWidth-badgeWidth)) + badge
}

func (c composer) inputView(skillPrefix string) string {
	view := c.input.View()
	skillPrefix = strings.TrimSpace(skillPrefix)
	if skillPrefix == "" {
		return view
	}

	plainPrefix := "!" + skillPrefix
	highlighted := lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiAccentTool).
		Render("$" + skillPrefix)
	return replaceVisibleText(view, plainPrefix, highlighted)
}

func replaceVisibleText(text string, target string, replacement string) string {
	if target == "" {
		return text
	}

	var visible strings.Builder
	rawIndexes := make([]int, 0, len(text))
	for i := 0; i < len(text); {
		if text[i] == '\x1b' {
			end := i + 1
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				i = end + 1
				continue
			}
		}

		rawIndexes = append(rawIndexes, i)
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		visible.WriteRune(r)
		i += size
	}

	visibleText := visible.String()
	before, _, ok := strings.Cut(visibleText, target)
	if !ok {
		return text
	}
	visibleStart := utf8.RuneCountInString(before)
	if visibleStart >= len(rawIndexes) {
		return text
	}
	visibleEnd := visibleStart + utf8.RuneCountInString(target)
	rawStart := rawIndexes[visibleStart]
	rawEnd := len(text)
	if visibleEnd < len(rawIndexes) {
		rawEnd = rawIndexes[visibleEnd]
	}
	return text[:rawStart] + replacement + text[rawEnd:]
}

func (c composer) Value() string {
	return c.input.Value()
}

func (c composer) SingleLine() bool {
	return c.input.LineCount() <= 1
}

func (c composer) CursorLine() int {
	return c.input.Line()
}

func (c composer) CursorColumn() int {
	return c.input.Column()
}

func (c composer) ValueBeforeCursor() string {
	value := c.input.Value()
	lines := strings.Split(value, "\n")
	row := c.input.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		return value
	}

	lineRunes := []rune(lines[row])
	col := min(max(0, c.input.Column()), len(lineRunes))
	before := strings.Join(lines[:row], "\n")
	if before != "" {
		before += "\n"
	}
	return before + string(lineRunes[:col])
}

func (c composer) ValueAfterCursor() string {
	value := c.input.Value()
	lines := strings.Split(value, "\n")
	row := c.input.Line()
	if row < 0 || row >= len(lines) {
		return ""
	}

	lineRunes := []rune(lines[row])
	col := min(max(0, c.input.Column()), len(lineRunes))
	after := string(lineRunes[col:])
	if row+1 < len(lines) {
		after += "\n" + strings.Join(lines[row+1:], "\n")
	}
	return after
}

func (c *composer) ReplaceTokenBeforeCursor(token string, replacement string) bool {
	if token == "" {
		return false
	}
	if !strings.HasSuffix(c.ValueBeforeCursor(), token) {
		return false
	}

	for range []rune(token) {
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		_ = cmd
	}
	c.input.InsertString(replacement)
	return true
}

func (c *composer) SetValue(value string) {
	c.input.SetValue(value)
	c.input.CursorEnd()
}

func (c *composer) Reset() {
	c.input.Reset()
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

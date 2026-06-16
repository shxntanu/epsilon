// Package tui contains the Bubble Tea terminal interface for epsilon.
package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core"
	"github.com/shxntanu/epsilon/core/types"
)

// Config configures a TUI session.
type Config struct {
	Harness *core.Harness
	RunID   string
}

// Run starts the TUI.
func Run(ctx context.Context, config Config) error {
	if config.Harness == nil {
		return fmt.Errorf("harness is nil")
	}

	run, err := startOrResume(ctx, config)
	if err != nil {
		return err
	}

	sub, err := run.Subscribe()
	if err != nil {
		return err
	}
	defer sub.Close()

	program := tea.NewProgram(newModel(ctx, run, sub.Events()), tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	return nil
}

func startOrResume(ctx context.Context, config Config) (*core.Run, error) {
	if strings.TrimSpace(config.RunID) == "" {
		return config.Harness.Start(ctx)
	}

	return config.Harness.Resume(ctx, config.RunID)
}

type eventMsg types.Event
type eventStreamClosedMsg struct{}
type stepDoneMsg struct {
	err error
}

type model struct {
	ctx      context.Context
	run      *core.Run
	events   <-chan types.Event
	chatBox  chatBox
	viewport viewport.Model
	entries  []transcriptEntry
	dirty    bool
	width    int
	height   int
	busy     bool
	status   string
	styles   styles
}

type styles struct {
	title      lipgloss.Style
	status     lipgloss.Style
	help       lipgloss.Style
	user       lipgloss.Style
	userBubble lipgloss.Style
	model      lipgloss.Style
	tool       lipgloss.Style
	error      lipgloss.Style
	muted      lipgloss.Style
}

type transcriptEntryKind int

const (
	transcriptPlain transcriptEntryKind = iota
	transcriptUser
)

type transcriptEntry struct {
	kind transcriptEntryKind
	text string
}

func newModel(ctx context.Context, run *core.Run, events <-chan types.Event) model {
	vp := viewport.New()
	vp.SoftWrap = true

	styles := styles{
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		status:     lipgloss.NewStyle().Foreground(lipgloss.Color("86")),
		help:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		user:       lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		userBubble: lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Padding(0, 1),
		model:      lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
		tool:       lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true),
		error:      lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		muted:      lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}

	return model{
		ctx:      ctx,
		run:      run,
		events:   events,
		chatBox:  newChatBox(),
		viewport: vp,
		status:   "ready",
		styles:   styles,
		entries: []transcriptEntry{
			{kind: transcriptPlain, text: styles.muted.Render("Started run " + run.ID())},
		},
		dirty: true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.chatBox.Init(), m.waitForEvent())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "ctrl+c", "esc":
			return m, quitCmd()
		case "enter":
			cmd := m.submit()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		default:
			var cmd tea.Cmd
			m.chatBox, cmd = m.chatBox.Update(msg)
			cmds = append(cmds, cmd)
		}
	case eventMsg:
		m.addEvent(types.Event(msg))
		cmds = append(cmds, m.waitForEvent())
	case eventStreamClosedMsg:
		m.status = "event stream closed"
	case stepDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "error"
			m.appendLog(m.styles.error.Render("error: " + msg.err.Error()))
		} else {
			m.status = "ready"
		}
	case tea.QuitMsg:
		return m, nil
	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
		m.chatBox, cmd = m.chatBox.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.syncViewport()
	return m, tea.Batch(cmds...)
}

func (m model) View() tea.View {
	header := m.styles.title.Render("epsilon") + " " +
		m.styles.muted.Render(m.run.ID()) + " " +
		m.styles.status.Render(m.status)
	help := m.styles.help.Render("enter send | shift+enter newline | esc/ctrl+c quit | tools: echo/read/write")

	body := m.viewport.View()
	content := lipgloss.JoinVertical(lipgloss.Left, header, body, m.chatBox.View(), help)

	var view tea.View
	view.SetContent(content)
	view.AltScreen = true
	return view
}

func (m *model) resize() {
	headerHeight := 1
	helpHeight := 1
	m.chatBox.SetWidth(m.width)
	viewportHeight := max(3, m.height-headerHeight-m.chatBox.Height()-helpHeight)
	viewportWidth := max(20, m.width)

	m.viewport.SetWidth(viewportWidth)
	m.viewport.SetHeight(viewportHeight)
	m.dirty = true
}

func (m *model) submit() tea.Cmd {
	if m.busy {
		return nil
	}

	text, ok := m.chatBox.Submit()
	if !ok {
		return nil
	}

	m.busy = true
	m.status = "thinking"

	return func() tea.Msg {
		if err := m.run.Send(m.ctx, types.UserMessage(text)); err != nil {
			return stepDoneMsg{err: err}
		}
		if err := m.run.Step(m.ctx); err != nil {
			return stepDoneMsg{err: err}
		}
		return stepDoneMsg{}
	}
}

func (m model) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
			return nil
		case event, ok := <-m.events:
			if !ok {
				return eventStreamClosedMsg{}
			}
			return eventMsg(event)
		}
	}
}

func (m *model) addEvent(event types.Event) {
	switch event.Kind {
	case types.EventUserMessageAdded:
		if event.Message != nil {
			text := textFromContent(event.Message.Content)
			if text != "" {
				m.appendUserMessage(text)
			}
		}
	case types.EventModelStarted:
		m.appendLog(m.styles.muted.Render("model started"))
	case types.EventModelMessageCompleted:
		if event.Message != nil {
			text := textFromContent(event.Message.Content)
			if text != "" {
				m.appendLog(m.styles.model.Render("assistant") + ": " + text)
			}
			for _, call := range event.Message.ToolCalls {
				m.appendLog(m.styles.tool.Render("tool requested") + ": " +
					call.Name + " " + string(call.Input))
			}
		}
	case types.EventToolCallStarted:
		if event.ToolCall != nil {
			m.appendLog(m.styles.tool.Render("tool started") + ": " + event.ToolCall.Name)
		}
	case types.EventToolCallCompleted:
		if event.ToolCall != nil && event.ToolResult != nil {
			line := m.styles.tool.Render("tool completed") + ": " +
				event.ToolCall.Name + " -> " + textFromContent(event.ToolResult.Content)
			if metadata := formatMetadata(event.ToolResult.Metadata); metadata != "" {
				line += " " + m.styles.muted.Render(metadata)
			}
			m.appendLog(line)
		}
	case types.EventPermissionRequested:
		if event.Permission != nil {
			m.appendLog(m.styles.tool.Render("permission requested") + ": " +
				event.Permission.Request.ToolName)
		}
	case types.EventPermissionGranted, types.EventPermissionDenied:
		if event.Permission != nil {
			m.appendLog(m.styles.tool.Render(string(event.Kind)) + ": " +
				event.Permission.Reason)
		}
	case types.EventRunError:
		if event.Error != "" {
			m.appendLog(m.styles.error.Render("run error: " + event.Error))
		}
	}
}

func (m *model) appendLog(line string) {
	m.entries = append(m.entries, transcriptEntry{
		kind: transcriptPlain,
		text: line,
	})
	m.dirty = true
}

func (m *model) appendUserMessage(text string) {
	m.entries = append(m.entries, transcriptEntry{
		kind: transcriptUser,
		text: text,
	})
	m.dirty = true
}

func (m *model) syncViewport() {
	if !m.dirty {
		return
	}
	m.viewport.SetContent(m.renderTranscript())
	m.viewport.GotoBottom()
	m.dirty = false
}

func (m model) renderTranscript() string {
	lines := make([]string, 0, len(m.entries))
	for _, entry := range m.entries {
		switch entry.kind {
		case transcriptUser:
			lines = append(lines, m.renderUserMessage(entry.text))
		default:
			lines = append(lines, entry.text)
		}
	}

	return strings.Join(lines, "\n")
}

func (m model) renderUserMessage(text string) string {
	message := strings.Join(prefixLines(strings.Split(text, "\n"), "> ", "  "), "\n")
	width := max(20, m.viewport.Width())
	return m.styles.userBubble.Width(width).Render(message)
}

func prefixLines(lines []string, firstPrefix string, restPrefix string) []string {
	prefixed := make([]string, 0, len(lines))
	for i, line := range lines {
		prefix := restPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		prefixed = append(prefixed, prefix+line)
	}

	return prefixed
}

func quitCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.Quit()
	}
}

func formatMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(data)
}

func textFromContent(content []types.ContentPart) string {
	var b strings.Builder
	for _, part := range content {
		if part.Kind != types.ContentText {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(part.Text)
	}

	return b.String()
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

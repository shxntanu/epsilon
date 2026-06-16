// Package tui contains the Bubble Tea terminal interface for epsilon.
package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core"
	"github.com/shxntanu/epsilon/core/session"
	"github.com/shxntanu/epsilon/core/types"
)

// Config configures a TUI session.
type Config struct {
	Harness   *core.Harness
	SessionID string
}

// Start starts the TUI.
func Start(ctx context.Context, config Config) error {
	if config.Harness == nil {
		return fmt.Errorf("harness is nil")
	}

	sess, err := startOrResume(ctx, config)
	if err != nil {
		return err
	}

	sub, err := sess.Subscribe()
	if err != nil {
		return err
	}
	defer sub.Close()

	program := tea.NewProgram(newModel(ctx, sess, sub.Events()), tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	return nil
}

func startOrResume(ctx context.Context, config Config) (*session.Session, error) {
	if strings.TrimSpace(config.SessionID) == "" {
		return config.Harness.StartSession(ctx)
	}

	return config.Harness.ResumeSession(ctx, config.SessionID)
}

type eventMsg types.Event
type eventStreamClosedMsg struct{}
type stepDoneMsg struct {
	err error
}

type model struct {
	ctx          context.Context
	session      *session.Session
	events       <-chan types.Event
	chatBox      chatBox
	spinner      spinner.Model
	viewport     viewport.Model
	entries      []transcriptEntry
	pendingUsers []string
	dirty        bool
	width        int
	height       int
	busy         bool
	showSpinner  bool
	showEvents   bool
	density      densityMode
	status       string
	styles       styles
}

type styles struct {
	header     lipgloss.Style
	title      lipgloss.Style
	sessionID  lipgloss.Style
	status     lipgloss.Style
	help       lipgloss.Style
	userLabel  lipgloss.Style
	agentLabel lipgloss.Style
	userBlock  lipgloss.Style
	agentBlock lipgloss.Style
	eventBlock lipgloss.Style
	tool       lipgloss.Style
	error      lipgloss.Style
	muted      lipgloss.Style
}

type transcriptEntryKind int

const (
	transcriptPlain transcriptEntryKind = iota
	transcriptUser
	transcriptAgent
	transcriptEvent
)

type transcriptEntry struct {
	kind transcriptEntryKind
	text string
}

type densityMode int

const (
	densityComfortable densityMode = iota
	densityCompact
)

func (d densityMode) Label() string {
	if d == densityCompact {
		return "compact"
	}

	return "comfortable"
}

func newModel(ctx context.Context, sess *session.Session, events <-chan types.Event) model {
	vp := viewport.New()
	vp.SoftWrap = true

	styles := styles{
		header: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1),
		title:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		sessionID: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		status: lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("29")).
			Padding(0, 1),
		help:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1),
		userLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true),
		agentLabel: lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
		userBlock: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("23")).
			Padding(0, 1),
		agentBlock: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("212")).
			PaddingLeft(1),
		eventBlock: lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).
			Background(lipgloss.Color("235")).
			Padding(0, 1),
		tool:  lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true),
		error: lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		muted: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
	spin := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(styles.status),
	)

	return model{
		ctx:      ctx,
		session:  sess,
		events:   events,
		chatBox:  newChatBox(),
		spinner:  spin,
		viewport: vp,
		density:  densityComfortable,
		status:   "ready",
		styles:   styles,
		entries: []transcriptEntry{
			{kind: transcriptEvent, text: styles.muted.Render("Started session " + sess.ID())},
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
		case "ctrl+o":
			m.showEvents = !m.showEvents
			m.dirty = true
		case "ctrl+t":
			m.toggleDensity()
			m.resize()
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
	case spinner.TickMsg:
		if m.showSpinner {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.dirty = true
			cmds = append(cmds, cmd)
		}
	case stepDoneMsg:
		m.busy = false
		m.showSpinner = false
		if msg.err != nil {
			m.status = "error"
			m.appendPlain(m.styles.error.Render("error: " + msg.err.Error()))
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
	headerText := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.styles.title.Render("epsilon"),
		" ",
		m.styles.sessionID.Render(m.session.ID()),
		" ",
		m.styles.status.Render(m.status),
	)
	header := m.styles.header.Width(max(0, m.width-2)).Render(headerText)
	help := m.styles.help.Render("enter send | shift+enter newline | ctrl+o events:" +
		onOff(m.showEvents) + " | ctrl+t density:" + m.density.Label() + " | esc quit")

	body := m.viewport.View()
	content := lipgloss.JoinVertical(lipgloss.Left, header, body, m.chatBox.View(), help)

	var view tea.View
	view.SetContent(content)
	view.AltScreen = true
	return view
}

func (m *model) resize() {
	headerHeight := 2
	helpHeight := 1
	m.chatBox.SetDensity(m.density)
	m.chatBox.SetWidth(m.width)
	viewportHeight := max(3, m.height-headerHeight-m.chatBox.Height()-helpHeight)
	viewportWidth := max(20, m.width)

	m.viewport.SetWidth(viewportWidth)
	m.viewport.SetHeight(viewportHeight)
	m.dirty = true
}

func (m *model) toggleDensity() {
	if m.density == densityComfortable {
		m.density = densityCompact
		return
	}

	m.density = densityComfortable
}

func (m *model) submit() tea.Cmd {
	if m.busy {
		return nil
	}

	text, ok := m.chatBox.Submit()
	if !ok {
		return nil
	}

	m.appendUserMessage(text)
	m.pendingUsers = append(m.pendingUsers, text)
	m.busy = true
	m.showSpinner = true
	m.status = "thinking"
	m.dirty = true

	sessionStep := func() tea.Msg {
		if err := m.session.Send(m.ctx, types.UserMessage(text)); err != nil {
			return stepDoneMsg{err: err}
		}
		if err := m.session.Step(m.ctx); err != nil {
			return stepDoneMsg{err: err}
		}
		return stepDoneMsg{}
	}

	return tea.Batch(m.spinner.Tick, sessionStep)
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
				if m.consumePendingUserMessage(text) {
					return
				}
				m.appendUserMessage(text)
			}
		}
	case types.EventModelStarted:
		m.appendEvent(m.styles.muted.Render("agent started"))
	case types.EventModelMessageCompleted:
		if event.Message != nil {
			text := textFromContent(event.Message.Content)
			if text != "" {
				m.showSpinner = false
				m.appendAgentMessage(text)
			}
			for _, call := range event.Message.ToolCalls {
				m.appendEvent(m.styles.tool.Render("tool requested") + ": " +
					call.Name + " " + string(call.Input))
			}
		}
	case types.EventToolCallStarted:
		if event.ToolCall != nil {
			m.appendEvent(m.styles.tool.Render("tool started") + ": " + event.ToolCall.Name)
		}
	case types.EventToolCallCompleted:
		if event.ToolCall != nil && event.ToolResult != nil {
			line := m.styles.tool.Render("tool completed") + ": " +
				event.ToolCall.Name + " -> " + textFromContent(event.ToolResult.Content)
			if metadata := formatMetadata(event.ToolResult.Metadata); metadata != "" {
				line += " " + m.styles.muted.Render(metadata)
			}
			m.appendEvent(line)
		}
	case types.EventPermissionRequested:
		if event.Permission != nil {
			m.appendEvent(m.styles.tool.Render("permission requested") + ": " +
				event.Permission.Request.ToolName)
		}
	case types.EventPermissionGranted, types.EventPermissionDenied:
		if event.Permission != nil {
			m.appendEvent(m.styles.tool.Render(string(event.Kind)) + ": " +
				event.Permission.Reason)
		}
	case types.EventSessionError:
		if event.Error != "" {
			m.appendPlain(m.styles.error.Render("session error: " + event.Error))
		}
	}
}

func (m *model) appendPlain(line string) {
	m.entries = append(m.entries, transcriptEntry{
		kind: transcriptPlain,
		text: line,
	})
	m.dirty = true
}

func (m *model) appendEvent(line string) {
	m.entries = append(m.entries, transcriptEntry{
		kind: transcriptEvent,
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

func (m *model) consumePendingUserMessage(text string) bool {
	if len(m.pendingUsers) == 0 || m.pendingUsers[0] != text {
		return false
	}

	m.pendingUsers = m.pendingUsers[1:]
	return true
}

func (m *model) appendAgentMessage(text string) {
	m.entries = append(m.entries, transcriptEntry{
		kind: transcriptAgent,
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
		line, ok := m.renderTranscriptEntry(entry)
		if ok {
			lines = append(lines, line)
		}
	}
	if m.showSpinner {
		lines = append(lines, m.renderSpinnerMessage())
	}

	if len(lines) == 0 {
		return m.styles.muted.Render("Ready. Send a message to start.")
	}

	return strings.Join(lines, m.entrySeparator())
}

func (m model) renderTranscriptEntry(entry transcriptEntry) (string, bool) {
	switch entry.kind {
	case transcriptUser:
		return m.renderUserMessage(entry.text), true
	case transcriptAgent:
		return m.renderAgentMessage(entry.text), true
	case transcriptEvent:
		if !m.showEvents {
			return "", false
		}
		return m.renderEvent(entry.text), true
	default:
		return entry.text, true
	}
}

func (m model) renderAgentMessage(text string) string {
	label := m.styles.agentLabel.Render("Agent")
	if m.density == densityCompact {
		return label + " " + m.styles.muted.Render("-") + " " + strings.TrimSpace(text)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		label,
		m.styles.agentBlock.Width(m.messageWidth()).Render(strings.TrimSpace(text)),
	)
}

func (m model) renderSpinnerMessage() string {
	label := m.styles.agentLabel.Render("Agent")
	responding := m.spinner.View() + " " + m.styles.muted.Render("responding")
	if m.density == densityCompact {
		return label + " " + m.styles.muted.Render("-") + " " + responding
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		label,
		m.styles.agentBlock.Width(m.messageWidth()).Render(responding),
	)
}

func (m model) renderUserMessage(text string) string {
	message := strings.TrimSpace(text)
	if m.density == densityCompact {
		return m.styles.userLabel.Render("You") + " " + m.styles.muted.Render("-") + " " + message
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.userLabel.Render("You"),
		m.styles.userBlock.Width(m.messageWidth()).Render(message),
	)
}

func (m model) renderEvent(text string) string {
	if m.density == densityCompact {
		return m.styles.muted.Render(text)
	}

	return m.styles.eventBlock.Width(m.messageWidth()).Render(text)
}

func (m model) messageWidth() int {
	return max(20, m.viewport.Width()-2)
}

func (m model) entrySeparator() string {
	if m.density == densityCompact {
		return "\n"
	}

	return "\n\n"
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

func onOff(value bool) string {
	if value {
		return "on"
	}

	return "off"
}

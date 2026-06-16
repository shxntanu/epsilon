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
	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/session"
	"github.com/shxntanu/epsilon/core/slash"
	"github.com/shxntanu/epsilon/core/types"
)

// Config configures a TUI session.
type Config struct {
	Harness          *core.Harness
	SessionID        string
	PermissionBroker *PermissionBroker
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

	contextSummary := func() contextwindow.Summary {
		if summary, ok := config.Harness.ContextSummary(sess.ID()); ok {
			return summary
		}
		return sess.ContextSummary(nil)
	}
	selectedModel := func() (types.ModelInfo, bool) {
		return config.Harness.CachedSelectedModelInfo()
	}
	program := tea.NewProgram(newModel(ctx, sess, sub, config.PermissionBroker,
		config.Harness.SlashCommands(), contextSummary, selectedModel,
		config.Harness.ListModels, config.Harness.ListSessions, config.Harness.ResumeSession,
		config.Harness.CurrentModel, config.Harness.CurrentEffort,
		config.Harness.SetModel, config.Harness.SetEffort),
		tea.WithContext(ctx))
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
type eventStreamClosedMsg struct {
	sessionID string
}
type stepDoneMsg struct {
	err error
}
type modelListMsg struct {
	models []types.ModelInfo
	err    error
}
type sessionListMsg struct {
	sessions []events.SessionInfo
	err      error
}
type resumeSessionMsg struct {
	sessionID string
	session   *session.Session
	sub       *events.Subscription
	err       error
}

const bottomGutterHeight = 1

type model struct {
	ctx           context.Context
	session       *session.Session
	subscription  *events.Subscription
	events        <-chan types.Event
	chatBox       chatBox
	slash         *slash.Registry
	slashCursor   int
	spinner       spinner.Model
	viewport      viewport.Model
	contextView   func() contextwindow.Summary
	modelInfo     func() (types.ModelInfo, bool)
	listModels    func(context.Context) ([]types.ModelInfo, error)
	listSessions  func(context.Context) ([]events.SessionInfo, error)
	resumeSession func(context.Context, string) (*session.Session, error)
	currentModel  func() string
	currentEffort func() string
	setModel      func(context.Context, string) error
	setEffort     func(string) error
	modelPicker   *modelPicker
	sessionPicker *sessionPicker
	permission    *permissionPrompt
	entries       []transcriptEntry
	pendingUsers  []string
	dirty         bool
	followOutput  bool
	width         int
	height        int
	busy          bool
	showSpinner   bool
	showEvents    bool
	showContext   bool
	density       densityMode
	status        string
	styles        styles
	broker        *PermissionBroker
	streaming     int
}

type styles struct {
	header         lipgloss.Style
	title          lipgloss.Style
	sessionID      lipgloss.Style
	status         lipgloss.Style
	help           lipgloss.Style
	userLabel      lipgloss.Style
	agentLabel     lipgloss.Style
	userBlock      lipgloss.Style
	eventBlock     lipgloss.Style
	diffBlock      lipgloss.Style
	diffHeader     lipgloss.Style
	diffAdd        lipgloss.Style
	diffRemove     lipgloss.Style
	diffMeta       lipgloss.Style
	selector       lipgloss.Style
	selectorActive lipgloss.Style
	selectorTitle  lipgloss.Style
	tool           lipgloss.Style
	error          lipgloss.Style
	muted          lipgloss.Style
}

type transcriptEntryKind int

const (
	transcriptPlain transcriptEntryKind = iota
	transcriptUser
	transcriptAgent
	transcriptEvent
	transcriptDiff
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

func newModel(ctx context.Context, sess *session.Session, sub *events.Subscription,
	broker *PermissionBroker, slashRegistry *slash.Registry,
	contextSummary func() contextwindow.Summary, selectedModel func() (types.ModelInfo, bool),
	listModels func(context.Context) ([]types.ModelInfo, error),
	listSessions func(context.Context) ([]events.SessionInfo, error),
	resumeSession func(context.Context, string) (*session.Session, error),
	currentModel func() string, currentEffort func() string,
	setModel func(context.Context, string) error, setEffort func(string) error) model {
	vp := viewport.New()
	vp.SoftWrap = true
	if slashRegistry == nil {
		slashRegistry = slash.NewDefaultRegistry()
	}
	if contextSummary == nil {
		contextSummary = func() contextwindow.Summary {
			return sess.ContextSummary(nil)
		}
	}
	if selectedModel == nil {
		selectedModel = func() (types.ModelInfo, bool) {
			return types.ModelInfo{}, false
		}
	}

	styles := styles{
		header: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1),
		title:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250")),
		sessionID: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		status: lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("237")).
			Padding(0, 1),
		help:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1),
		userLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color("109")).Bold(true),
		agentLabel: lipgloss.NewStyle().Foreground(lipgloss.Color("146")).Bold(true),
		userBlock: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236")).
			Padding(0, 1),
		eventBlock: lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).
			Background(lipgloss.Color("235")).
			Padding(0, 1),
		diffBlock: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("234")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("244")).
			Padding(0, 1),
		diffHeader: lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Bold(true),
		diffAdd:    lipgloss.NewStyle().Foreground(lipgloss.Color("120")),
		diffRemove: lipgloss.NewStyle().Foreground(lipgloss.Color("210")),
		diffMeta:   lipgloss.NewStyle().Foreground(lipgloss.Color("246")),
		tool:       lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true),
		error:      lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		muted:      lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
	styles = selectorStyles(styles)
	spin := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(styles.status),
	)

	var eventStream <-chan types.Event
	if sub != nil {
		eventStream = sub.Events()
	}

	return model{
		ctx:           ctx,
		session:       sess,
		subscription:  sub,
		events:        eventStream,
		chatBox:       newChatBox(),
		slash:         slashRegistry,
		spinner:       spin,
		viewport:      vp,
		contextView:   contextSummary,
		modelInfo:     selectedModel,
		listModels:    listModels,
		listSessions:  listSessions,
		resumeSession: resumeSession,
		currentModel:  currentModel,
		currentEffort: currentEffort,
		setModel:      setModel,
		setEffort:     setEffort,
		density:       densityComfortable,
		status:        "ready",
		styles:        styles,
		broker:        broker,
		streaming:     -1,
		entries: []transcriptEntry{
			{kind: transcriptEvent, text: styles.muted.Render("Started session " + sess.ID())},
		},
		dirty:        true,
		followOutput: true,
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
		if m.permission != nil {
			cmd := m.updatePermissionPrompt(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.syncViewport()
			return m, tea.Batch(cmds...)
		}
		if m.modelPicker != nil {
			cmd := m.updateModelPicker(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.syncViewport()
			return m, tea.Batch(cmds...)
		}
		if m.sessionPicker != nil {
			cmd := m.updateSessionPicker(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.syncViewport()
			return m, tea.Batch(cmds...)
		}

		switch msg.Keystroke() {
		case "ctrl+c", "esc":
			return m, quitCmd()
		case "up", "ctrl+p":
			if m.moveSlashSelection(-1) {
				return m, nil
			}
			var cmd tea.Cmd
			m.chatBox, cmd = m.chatBox.Update(msg)
			cmds = append(cmds, cmd)
			m.resize()
		case "down", "ctrl+n":
			if m.moveSlashSelection(1) {
				return m, nil
			}
			var cmd tea.Cmd
			m.chatBox, cmd = m.chatBox.Update(msg)
			cmds = append(cmds, cmd)
			m.resize()
		case "tab":
			if m.completeSlashSelection() {
				return m, nil
			}
			var cmd tea.Cmd
			m.chatBox, cmd = m.chatBox.Update(msg)
			cmds = append(cmds, cmd)
			m.resize()
		case "pgup":
			m.viewport.PageUp()
			m.followOutput = false
		case "pgdown":
			m.viewport.PageDown()
			m.followOutput = m.viewport.AtBottom()
		case "ctrl+up", "ctrl+u":
			m.viewport.HalfPageUp()
			m.followOutput = false
		case "ctrl+down", "ctrl+d":
			m.viewport.HalfPageDown()
			m.followOutput = m.viewport.AtBottom()
		case "ctrl+o":
			m.showEvents = !m.showEvents
			m.dirty = true
		case "ctrl+x":
			m.showContext = !m.showContext
			m.resize()
		case "ctrl+t":
			m.toggleDensity()
			m.resize()
		case "enter":
			if m.completeIncompleteSlashInput() {
				return m, nil
			}
			cmd := m.submit()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		default:
			var cmd tea.Cmd
			m.chatBox, cmd = m.chatBox.Update(msg)
			cmds = append(cmds, cmd)
			m.resize()
		}
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
		m.followOutput = m.viewport.AtBottom()
	case eventMsg:
		event := types.Event(msg)
		if event.SessionID == "" || event.SessionID == m.session.ID() {
			m.addEvent(event)
		}
		cmds = append(cmds, m.waitForEvent())
	case eventStreamClosedMsg:
		if msg.sessionID == m.session.ID() {
			m.status = "event stream closed"
		}
	case modelListMsg:
		m.applyModelList(msg)
	case sessionListMsg:
		m.applySessionList(msg)
	case resumeSessionMsg:
		if cmd := m.applyResumeSession(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
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
		m.resize()
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
	help := m.styles.help.Render("ctrl+o events:" + onOff(m.showEvents) +
		" | ctrl+x context:" + onOff(m.showContext) + " | esc quit")
	contextLine := m.renderContextWidget()
	bottomGutter := strings.Repeat("\n", bottomGutterHeight)

	body := m.viewport.View()
	if m.contextPanelWidth() > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, m.renderContextPanel())
	}
	parts := []string{header, body}
	if m.permission != nil {
		parts = append(parts, m.permission.View())
	}
	if selector, ok := m.renderSlashSelector(); ok {
		parts = append(parts, selector)
	}
	if picker, ok := m.renderModelPicker(); ok {
		parts = append(parts, picker)
	}
	if picker, ok := m.renderSessionPicker(); ok {
		parts = append(parts, picker)
	}
	parts = append(parts, m.chatBox.View(), contextLine, help, bottomGutter)
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	var view tea.View
	view.SetContent(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m *model) resize() {
	headerHeight := 2
	helpHeight := 2
	m.chatBox.SetDensity(m.density)
	m.chatBox.SetWidth(m.width)
	permissionHeight := 0
	if m.permission != nil {
		m.permission.SetWidth(m.width)
		permissionHeight = 6
	}
	selectorHeight := m.slashSelectorHeight()
	modelPickerHeight := m.modelPickerHeight()
	sessionPickerHeight := m.sessionPickerHeight()
	viewportHeight := max(3, m.height-headerHeight-m.chatBox.Height()-helpHeight-
		permissionHeight-selectorHeight-modelPickerHeight-sessionPickerHeight-bottomGutterHeight)
	viewportWidth := max(20, m.width-m.contextPanelWidth())

	m.viewport.SetWidth(viewportWidth)
	m.viewport.SetHeight(viewportHeight)
	m.dirty = true
}

func (m model) renderContextWidget() string {
	return newContextWidget(m.contextView(), m.width, m.selectedModelInfo()).View()
}

func (m model) renderContextPanel() string {
	width := m.contextPanelWidth()
	if width == 0 {
		return ""
	}
	return newContextPanel(m.contextView(), width, m.viewport.Height(), m.selectedModelInfo()).Panel()
}

func (m model) contextPanelWidth() int {
	if !m.showContext {
		return 0
	}
	available := m.width - 24
	if available < contextPanelMinWidth {
		return 0
	}
	return min(contextPanelMaxWidth, max(contextPanelMinWidth, m.width/3))
}

func (m model) selectedModelInfo() *types.ModelInfo {
	if m.modelInfo == nil {
		return nil
	}
	model, ok := m.modelInfo()
	if !ok {
		return nil
	}
	return &model
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
	if parsed := slash.ParseInput(text); parsed.Escaped {
		text = slash.UnescapeInput(text)
	} else if parsed.OK {
		result, handled, err := m.slash.Execute(m.ctx, text, m.slashExecution())
		if handled {
			return m.applySlashResult(result, err)
		}
	}

	m.appendUserMessage(text)
	m.pendingUsers = append(m.pendingUsers, text)
	m.busy = true
	m.showSpinner = true
	m.followOutput = true
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
	sessionID := ""
	if m.session != nil {
		sessionID = m.session.ID()
	}
	events := m.events
	return func() tea.Msg {
		if events == nil {
			return eventStreamClosedMsg{sessionID: sessionID}
		}
		select {
		case <-m.ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return eventStreamClosedMsg{sessionID: sessionID}
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
		if m.busy {
			m.showSpinner = true
		}
		m.appendEvent(m.styles.muted.Render("agent started"))
	case types.EventModelTextDelta:
		if event.TextDelta != "" {
			m.showSpinner = false
			m.appendAgentDelta(event.TextDelta)
		}
	case types.EventModelMessageCompleted:
		m.showSpinner = false
		if event.Message != nil {
			text := textFromContent(event.Message.Content)
			if text != "" {
				m.completeAgentMessage(text)
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
			if diff := event.ToolResult.Metadata["diff"]; diff != "" && !event.ToolResult.IsError {
				m.appendDiff(diff)
			}
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
			if m.broker != nil {
				prompt := newPermissionPrompt(event.Permission.Request)
				prompt.SetWidth(m.width)
				m.permission = &prompt
				m.status = "approval"
				m.resize()
			}
		}
	case types.EventPermissionGranted, types.EventPermissionDenied:
		if event.Permission != nil {
			m.appendEvent(m.styles.tool.Render(string(event.Kind)) + ": " +
				event.Permission.Reason)
			m.permission = nil
			m.resize()
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

func (m *model) appendDiff(diff string) {
	m.entries = append(m.entries, transcriptEntry{
		kind: transcriptDiff,
		text: diff,
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
	m.streaming = -1
	m.dirty = true
}

func (m *model) appendAgentDelta(delta string) {
	if m.streaming < 0 || m.streaming >= len(m.entries) {
		m.entries = append(m.entries, transcriptEntry{
			kind: transcriptAgent,
			text: delta,
		})
		m.streaming = len(m.entries) - 1
		m.dirty = true
		return
	}

	m.entries[m.streaming].text += delta
	m.dirty = true
}

func (m *model) completeAgentMessage(text string) {
	if m.streaming >= 0 && m.streaming < len(m.entries) {
		m.entries[m.streaming].text = text
		m.streaming = -1
		m.dirty = true
		return
	}

	m.appendAgentMessage(text)
}

func (m *model) updatePermissionPrompt(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.Keystroke() {
	case "ctrl+c":
		return quitCmd()
	case "esc":
		m.resolvePermission(types.PermissionDecisionDeny, "denied from TUI")
	case "enter":
		m.resolvePermission(m.permission.Decision(), m.permission.Reason())
	default:
		updated := m.permission.Update(msg)
		m.permission = &updated
		m.dirty = true
	}

	return nil
}

func (m *model) resolvePermission(decision types.PermissionDecision, reason string) {
	if m.permission == nil || m.broker == nil {
		return
	}

	m.broker.Resolve(m.permission.request, decision, reason)
	m.permission = nil
	m.status = "thinking"
	m.resize()
	m.dirty = true
}

func (m *model) syncViewport() {
	if !m.dirty {
		return
	}
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.renderTranscript())
	if m.followOutput || atBottom {
		m.viewport.GotoBottom()
		m.followOutput = true
	}
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
	case transcriptPlain:
		return m.renderHarnessMessage(entry.text), true
	case transcriptUser:
		return m.renderUserMessage(entry.text), true
	case transcriptAgent:
		return m.renderAgentMessage(entry.text), true
	case transcriptEvent:
		if !m.showEvents {
			return "", false
		}
		return m.renderEvent(entry.text), true
	case transcriptDiff:
		return m.renderDiff(entry.text), true
	default:
		return entry.text, true
	}
}

func (m model) renderAgentMessage(text string) string {
	return m.harnessMessage().RenderAgent(text)
}

func (m model) renderSpinnerMessage() string {
	responding := m.spinner.View() + " " + m.styles.muted.Render("responding")
	return m.harnessMessage().RenderSpinner(responding)
}

func (m model) renderHarnessMessage(text string) string {
	return m.harnessMessage().Render(text)
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

func (m model) renderDiff(diff string) string {
	body := m.renderDiffBody(diff)
	if m.density == densityCompact {
		return m.styles.tool.Render("Changes") + "\n" + body
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.tool.Render("Changes"),
		m.styles.diffBlock.Width(m.messageWidth()).Render(body),
	)
}

func (m model) renderDiffBody(diff string) string {
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "*** "):
			rendered = append(rendered, m.styles.diffHeader.Render(line))
		case strings.HasPrefix(line, "@@"):
			rendered = append(rendered, m.styles.diffMeta.Render(line))
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			rendered = append(rendered, m.styles.diffHeader.Render(line))
		case strings.HasPrefix(line, "+"):
			rendered = append(rendered, m.styles.diffAdd.Render(line))
		case strings.HasPrefix(line, "-"):
			rendered = append(rendered, m.styles.diffRemove.Render(line))
		default:
			rendered = append(rendered, m.styles.muted.Render(line))
		}
	}

	return strings.Join(rendered, "\n")
}

func (m model) messageWidth() int {
	return max(20, m.viewport.Width()-2)
}

func (m model) harnessMessageWidth() int {
	return max(20, m.viewport.Width())
}

func (m model) harnessMessage() harnessMessage {
	return newHarnessMessage(
		m.harnessMessageWidth(),
		m.density,
		m.styles.agentLabel,
		m.styles.muted,
	)
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
	display := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if key == "diff" {
			continue
		}
		display[key] = value
	}
	if len(display) == 0 {
		return ""
	}
	data, err := json.Marshal(display)
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

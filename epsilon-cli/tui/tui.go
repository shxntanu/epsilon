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

	sess, resumed, err := startOrResume(ctx, config)
	if err != nil {
		return err
	}

	subscribe := sess.Subscribe
	if resumed {
		subscribe = sess.SubscribeLive
	}
	sub, err := subscribe()
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
	initialHistory := []types.Event(nil)
	if resumed {
		initialHistory = sess.History()
	}
	program := tea.NewProgram(newModel(ctx, sess, sub, config.PermissionBroker,
		config.Harness.SlashCommands(), contextSummary, selectedModel,
		config.Harness.ListModels, config.Harness.ListSessions, config.Harness.ResumeSession,
		config.Harness.CurrentModel, config.Harness.CurrentEffort,
		config.Harness.SetModel, config.Harness.SetEffort,
		config.Harness.RenameSession, initialHistory),
		tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	return nil
}

func startOrResume(ctx context.Context, config Config) (*session.Session, bool, error) {
	if strings.TrimSpace(config.SessionID) == "" {
		sess, err := config.Harness.StartSession(ctx)
		return sess, false, err
	}

	sess, err := config.Harness.ResumeSession(ctx, config.SessionID)
	return sess, true, err
}

type eventMsg types.Event
type eventStreamClosedMsg struct {
	sessionID string
}
type stepDoneMsg struct {
	id  uint64
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
	title     string
	history   []types.Event
	session   *session.Session
	sub       *events.Subscription
	err       error
}
type terminalScrollbackPrintedMsg struct {
	printed    int
	headerOut  bool
	replayDone bool
}

const bottomGutterHeight = 1
const defaultSessionTitleMaxRunes = 48

type model struct {
	appState
	providerState
	inputState
	overlayState
	transcriptState
	layoutState
	activityState
	scrollbackState
	stepState
	visualState
}

type styles struct {
	screen         lipgloss.Style
	header         lipgloss.Style
	title          lipgloss.Style
	sessionName    lipgloss.Style
	status         lipgloss.Style
	statusReady    lipgloss.Style
	help           lipgloss.Style
	userLabel      lipgloss.Style
	agentLabel     lipgloss.Style
	userBlock      lipgloss.Style
	eventBlock     lipgloss.Style
	statusLine     lipgloss.Style
	toolBlock      lipgloss.Style
	diffBlock      lipgloss.Style
	diffHeader     lipgloss.Style
	diffAdd        lipgloss.Style
	diffRemove     lipgloss.Style
	diffMeta       lipgloss.Style
	selector       lipgloss.Style
	selectorActive lipgloss.Style
	selectorTitle  lipgloss.Style
	selectorMeta   lipgloss.Style
	selectorKey    lipgloss.Style
	selectorHint   lipgloss.Style
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
	transcriptStatus
	transcriptTool
	transcriptToolGroup
)

type transcriptEntry struct {
	kind       transcriptEntryKind
	text       string
	toolCallID string
	toolName   string
	toolInput  string
	toolResult string
	toolMeta   map[string]string
	toolError  bool
	toolActive bool
	tools      []transcriptEntry
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
	setModel func(context.Context, string) error, setEffort func(string) error,
	renameSession func(context.Context, string, string) (bool, error),
	initialHistory []types.Event) model {
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
		screen: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiInk),
		header: lipgloss.NewStyle().
			Background(tuiBackground).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(tuiLine).
			Padding(0, 1),
		title: lipgloss.NewStyle().
			Background(tuiBackground).
			Bold(true).
			Foreground(tuiInkStrong),
		sessionName: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiAccentAgent),
		status: lipgloss.NewStyle().
			Foreground(tuiInk).
			Background(tuiSurface3).
			Padding(0, 1),
		statusReady: lipgloss.NewStyle().
			Bold(true).
			Foreground(tuiBackground).
			Background(tuiStatusReady).
			Padding(0, 1),
		help: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiFaint).
			Padding(0, 1),
		userLabel: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiAccentUser).
			Bold(true),
		agentLabel: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiAccentAgent).
			Bold(true),
		userBlock: lipgloss.NewStyle().
			Foreground(tuiInk).
			Background(tuiSurface2).
			Padding(0, 1),
		eventBlock: lipgloss.NewStyle().
			Foreground(tuiMuted).
			Background(tuiSurface2).
			Padding(0, 1),
		statusLine: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiSubtle),
		toolBlock: lipgloss.NewStyle().
			Foreground(tuiInk).
			Background(tuiBackground),
		// GitHub-like diff coloring (independent from the rest of the harness palette).
		diffBlock: lipgloss.NewStyle().
			Foreground(tuiInk).
			Background(tuiBackground).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("#30363D")).
			Padding(0, 1),
		diffHeader: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(lipgloss.Color("#8B949E")).
			Bold(true),
		diffAdd: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(lipgloss.Color("#3FB950")),
		diffRemove: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(lipgloss.Color("#FF7B72")),
		diffMeta: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(lipgloss.Color("#8B949E")),
		tool: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiAccentTool),
		error: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiAccentDanger).
			Bold(true),
		muted: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiSubtle),
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

	entries := []transcriptEntry{
		{kind: transcriptEvent, text: "Started session " + sess.ID()},
	}
	if len(initialHistory) > 0 {
		entries = hydrateTranscriptEntries(initialHistory)
	}

	return model{
		appState: appState{
			ctx:          ctx,
			session:      sess,
			subscription: sub,
			events:       eventStream,
			broker:       broker,
		},
		providerState: providerState{
			contextView:   contextSummary,
			modelInfo:     selectedModel,
			listModels:    listModels,
			listSessions:  listSessions,
			resumeSession: resumeSession,
			currentModel:  currentModel,
			currentEffort: currentEffort,
			setModel:      setModel,
			setEffort:     setEffort,
			renameSession: renameSession,
		},
		inputState: inputState{
			composer: newComposer(),
			slash:    slashRegistry,
		},
		transcriptState: transcriptState{
			streaming: -1,
			entries:   entries,
		},
		layoutState: layoutState{
			viewport:       vp,
			showBackground: false,
			mouseCapture:   true,
			density:        densityComfortable,
			followOutput:   true,
		},
		activityState: activityState{
			spinner:      spin,
			thinking:     newThinking("Thinking"),
			status:       "ready",
			sessionTitle: "",
			dirty:        true,
		},
		scrollbackState: scrollbackState{
			scrollReplay: true,
		},
		visualState: visualState{
			styles: styles,
		},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.composer.Init(), m.waitForEvent(), m.startHeaderAnimation())
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
			if msg.Keystroke() == "esc" && m.busy {
				m.interruptModel()
				m.syncViewport()
				return m, nil
			}
			cmd := m.updatePermissionPrompt(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if cmd := m.startHeaderAnimation(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.syncViewport()
			if cmd := m.terminalScrollbackCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if m.modelPicker != nil {
			cmd := m.updateModelPicker(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if cmd := m.startHeaderAnimation(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.syncViewport()
			if cmd := m.terminalScrollbackCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if m.sessionPicker != nil {
			cmd := m.updateSessionPicker(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if cmd := m.startHeaderAnimation(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.syncViewport()
			if cmd := m.terminalScrollbackCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.Keystroke() {
		case "ctrl+c":
			if cmd := m.confirmQuit(); cmd != nil {
				return m, cmd
			}
		case "esc":
			m.quitArmed = false
			if m.busy {
				m.interruptModel()
			}
		case "up", "ctrl+p":
			m.quitArmed = false
			if m.moveSlashSelection(-1) {
				return m, nil
			}
			if m.composer.SingleLine() && m.recallPromptHistory(-1) {
				m.resize()
				return m, nil
			}
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			cmds = append(cmds, cmd)
			m.resize()
		case "down", "ctrl+n":
			m.quitArmed = false
			if m.moveSlashSelection(1) {
				return m, nil
			}
			if m.composer.SingleLine() && m.recallPromptHistory(1) {
				m.resize()
				return m, nil
			}
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			cmds = append(cmds, cmd)
			m.resize()
		case "tab":
			m.quitArmed = false
			if m.completeSlashSelection() {
				return m, nil
			}
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			cmds = append(cmds, cmd)
			m.resize()
		case "pgup":
			m.quitArmed = false
			m.viewport.PageUp()
			m.followOutput = false
		case "pgdown":
			m.quitArmed = false
			m.viewport.PageDown()
			m.followOutput = m.viewport.AtBottom()
		case "ctrl+up", "ctrl+u":
			m.quitArmed = false
			m.viewport.HalfPageUp()
			m.followOutput = false
		case "ctrl+down", "ctrl+d":
			m.quitArmed = false
			m.viewport.HalfPageDown()
			m.followOutput = m.viewport.AtBottom()
		case "ctrl+o":
			m.quitArmed = false
			m.showEvents = !m.showEvents
			m.status = "details:" + onOff(m.showEvents)
			m.dirty = true
		case "ctrl+x":
			m.quitArmed = false
			m.showContext = !m.showContext
			m.resize()
		case "ctrl+g":
			m.quitArmed = false
			m.mouseCapture = !m.mouseCapture
			if m.mouseCapture {
				m.status = "view:terminal"
				m.prepareTerminalScroll()
			} else {
				m.status = "view:app"
			}
			m.dirty = true
		case "ctrl+t":
			m.quitArmed = false
			m.toggleDensity()
			m.resize()
		case "enter":
			m.quitArmed = false
			if m.completeIncompleteSlashInput() {
				return m, nil
			}
			cmd := m.submit()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		default:
			m.quitArmed = false
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
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
		if m.showSpinner || m.hasActiveTools() {
			cmds = append(cmds, m.spinner.Tick)
		}
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
	case terminalScrollbackPrintedMsg:
		m.scrollPrinted = msg.printed
		m.scrollHeaderOut = msg.headerOut
		m.scrollReplay = !msg.replayDone
		m.scrollPrinting = false
	case spinner.TickMsg:
		if m.showSpinner || m.hasActiveTools() {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.dirty = true
			cmds = append(cmds, cmd)
		}
	case thinkingTickMsg:
		if m.showSpinner {
			var cmd tea.Cmd
			m.thinking, cmd = m.thinking.update(msg)
			m.dirty = true
			cmds = append(cmds, cmd)
		}
	case headerTickMsg:
		m.headerAnimating = false
		if m.shouldAnimateHeader() {
			m.headerFrame++
		}
	case stepDoneMsg:
		if msg.id != m.activeStepID {
			break
		}
		m.stepCancel = nil
		m.activeStepID = 0
		m.busy = false
		m.showSpinner = false
		if msg.err != nil {
			if errors.Is(msg.err, context.Canceled) {
				m.status = "ready"
				break
			}
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
		m.composer, cmd = m.composer.Update(msg)
		cmds = append(cmds, cmd)
		m.resize()
	}

	if cmd := m.startHeaderAnimation(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	m.syncViewport()
	if cmd := m.terminalScrollbackCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m model) View() tea.View {
	if m.terminalScrollMode() {
		return m.terminalScrollView()
	}

	headerText := m.renderHeaderText(m.width)
	header := m.styles.header.Width(max(0, m.width-2)).Render(headerText)
	helpText := "ctrl+o details:" + onOff(m.showEvents) +
		" | ctrl+x context:" + onOff(m.showContext) +
		" | ctrl+g view:" + m.viewModeLabel() + " | ctrl+c twice quit"
	if m.busy {
		helpText = "esc interrupt | " + helpText
	}
	help := m.styles.help.Render(helpText)
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
	parts = append(parts, m.composer.View(m.slashSelectorActive(), m.headerFrame),
		contextLine, help, bottomGutter)
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	content = m.styles.screen.
		Width(max(0, m.width)).
		Height(max(0, m.height)).
		Render(content)
	content = paintScreenBackground(content, m.width, m.height)

	var view tea.View
	view.SetContent(content)
	view.BackgroundColor = tuiBackgroundColor
	view.AltScreen = true
	view.MouseMode = m.currentMouseMode()
	return view
}

func (m model) terminalScrollMode() bool {
	return m.mouseCapture
}

func (m model) terminalScrollView() tea.View {
	parts := make([]string, 0, 6)
	if m.shouldRenderStartupHeader() {
		parts = append(parts, m.renderStartupHeader(m.width))
	} else if m.showEvents {
		if detail := m.renderTerminalDetailTranscript(); detail != "" {
			parts = append(parts, detail)
		}
	} else if replay := m.renderTerminalReplayTranscript(); replay != "" {
		parts = append(parts, replay)
	} else if live := m.renderTerminalLiveTranscript(); live != "" {
		parts = append(parts, live)
	}
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
	parts = append(parts, m.composer.View(m.slashSelectorActive(), m.headerFrame))
	if m.busy {
		parts = append(parts, m.styles.help.Render("esc interrupt | ctrl+g app view | ctrl+c twice quit"))
	} else {
		parts = append(parts, m.styles.help.Render("ctrl+g app view | ctrl+c twice quit"))
	}

	var view tea.View
	view.SetContent(lipgloss.JoinVertical(lipgloss.Left, parts...))
	view.BackgroundColor = tuiBackgroundColor
	view.MouseMode = tea.MouseModeNone
	return view
}

func (m model) shouldRenderStartupHeader() bool {
	if m.busy || m.permission != nil || m.modelPicker != nil || m.sessionPicker != nil ||
		m.slashSelectorActive() {
		return false
	}
	for _, entry := range m.entries {
		switch entry.kind {
		case transcriptEvent:
			if m.showEvents {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (m model) currentSessionLabel() string {
	if title := strings.TrimSpace(m.sessionTitle); title != "" {
		return title
	}
	return m.session.ID()
}

func (m model) currentMouseMode() tea.MouseMode {
	if m.mouseCapture && !m.terminalScrollMode() {
		return tea.MouseModeCellMotion
	}
	return tea.MouseModeNone
}

func (m model) viewModeLabel() string {
	if m.terminalScrollMode() {
		return "terminal"
	}
	return "app"
}

func (m *model) resize() {
	headerHeight := 2
	helpHeight := 2
	m.composer.SetDensity(m.density)
	m.composer.SetWidth(m.width)
	permissionHeight := 0
	if m.permission != nil {
		m.permission.SetWidth(m.width)
		permissionHeight = 6
	}
	selectorHeight := m.slashSelectorHeight()
	modelPickerHeight := m.modelPickerHeight()
	sessionPickerHeight := m.sessionPickerHeight()
	viewportHeight := max(3, m.height-headerHeight-m.composer.Height()-helpHeight-
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

func (m *model) recallPromptHistory(delta int) bool {
	if len(m.promptHistory) == 0 {
		return false
	}
	if delta < 0 {
		if !m.historyActive {
			m.historyDraft = m.composer.Value()
			m.historyIndex = len(m.promptHistory) - 1
			m.historyActive = true
		} else if m.historyIndex > 0 {
			m.historyIndex--
		}
		m.composer.SetValue(m.promptHistory[m.historyIndex])
		return true
	}
	if delta > 0 {
		if !m.historyActive {
			return false
		}
		if m.historyIndex < len(m.promptHistory)-1 {
			m.historyIndex++
			m.composer.SetValue(m.promptHistory[m.historyIndex])
			return true
		}

		m.historyActive = false
		m.historyIndex = len(m.promptHistory)
		m.composer.SetValue(m.historyDraft)
		m.historyDraft = ""
		return true
	}

	return false
}

func (m *model) appendPromptHistory(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	m.promptHistory = append(m.promptHistory, text)
	m.historyIndex = len(m.promptHistory)
	m.historyDraft = ""
	m.historyActive = false
}

func (m *model) submit() tea.Cmd {
	if m.busy {
		return nil
	}

	text, ok := m.composer.Submit()
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

	m.appendPromptHistory(text)
	m.appendUserMessage(text)
	m.pendingUsers = append(m.pendingUsers, text)
	shouldDefaultTitle := !m.session.HasMessages() && strings.TrimSpace(m.sessionTitle) == ""
	defaultTitle := defaultSessionTitle(text, defaultSessionTitleMaxRunes)
	if shouldDefaultTitle && defaultTitle != "" {
		m.sessionTitle = defaultTitle
	}
	m.busy = true
	m.showSpinner = true
	m.followOutput = true
	m.status = "thinking"
	m.dirty = true
	stepCtx, cancel := context.WithCancel(m.ctx)
	m.stepCancel = cancel
	m.stepSeq++
	stepID := m.stepSeq
	m.activeStepID = stepID

	sessionStep := func() tea.Msg {
		if err := m.session.Send(stepCtx, types.UserMessage(text)); err != nil {
			return stepDoneMsg{id: stepID, err: err}
		}
		if shouldDefaultTitle && defaultTitle != "" && m.renameSession != nil {
			_, _ = m.renameSession(stepCtx, m.session.ID(), defaultTitle)
		}
		if err := m.session.Step(stepCtx); err != nil {
			return stepDoneMsg{id: stepID, err: err}
		}
		return stepDoneMsg{id: stepID}
	}

	return tea.Batch(m.spinner.Tick, thinkingTick(), sessionStep)
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
	m.applyEvent(event, true)
}

func (m *model) applyEvent(event types.Event, interactive bool) {
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
				m.appendToolRequested(call)
			}
		}
	case types.EventToolCallStarted:
		if event.ToolCall != nil {
			m.markToolStarted(*event.ToolCall)
		}
	case types.EventToolCallCompleted:
		if event.ToolCall != nil && event.ToolResult != nil {
			if diff := event.ToolResult.Metadata["diff"]; diff != "" && !event.ToolResult.IsError {
				m.appendDiff(diff)
			}
			m.markToolCompleted(*event.ToolCall, *event.ToolResult)
		}
	case types.EventPermissionRequested:
		if event.Permission != nil {
			m.appendStatus("Waiting for approval to use " + event.Permission.Request.ToolName)
			if interactive && m.broker != nil {
				prompt := newPermissionPrompt(event.Permission.Request)
				prompt.SetWidth(m.width)
				m.permission = &prompt
				m.status = "approval"
				m.resize()
			}
		}
	case types.EventPermissionGranted, types.EventPermissionDenied:
		if event.Permission != nil {
			m.appendStatus(strings.ReplaceAll(string(event.Kind), "_", " ") + ": " +
				event.Permission.Reason)
			if interactive {
				m.permission = nil
				m.resize()
			}
		}
	case types.EventSessionError:
		if event.Error != "" {
			m.appendPlain(m.styles.error.Render("session error: " + event.Error))
		}
	}
}

func hydrateTranscriptEntries(history []types.Event) []transcriptEntry {
	rehydrated := model{
		transcriptState: transcriptState{streaming: -1},
	}
	for _, event := range history {
		rehydrated.applyEvent(event, false)
	}
	rehydrated.streaming = -1
	rehydrated.showSpinner = false
	rehydrated.busy = false
	return rehydrated.entries
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

func (m *model) appendStatus(text string) {
	m.entries = append(m.entries, transcriptEntry{
		kind: transcriptStatus,
		text: text,
	})
	m.dirty = true
}

func (m *model) appendToolRequested(call types.ToolCall) {
	entry := transcriptEntry{
		kind:       transcriptTool,
		text:       toolRequestedText(call.Name),
		toolCallID: call.ID,
		toolName:   call.Name,
		toolInput:  strings.TrimSpace(string(call.Input)),
	}
	if m.appendExplorationTool(entry) {
		m.dirty = true
		return
	}
	m.entries = append(m.entries, entry)
	m.dirty = true
}

func (m *model) markToolStarted(call types.ToolCall) {
	index, childIndex := m.findToolEntry(call.ID)
	if index < 0 {
		m.appendToolRequested(call)
		index, childIndex = m.findToolEntry(call.ID)
		if index < 0 {
			return
		}
	}
	entry := m.toolEntryAt(index, childIndex)
	entry.text = toolRunningText(call.Name)
	entry.toolActive = true
	entry.toolName = call.Name
	if strings.TrimSpace(entry.toolInput) == "" {
		entry.toolInput = strings.TrimSpace(string(call.Input))
	}
	m.setToolEntryAt(index, childIndex, entry)
	m.refreshToolGroup(index)
	m.dirty = true
}

func (m *model) markToolCompleted(call types.ToolCall, result types.ToolResult) {
	index, childIndex := m.findToolEntry(call.ID)
	if index < 0 {
		m.appendToolRequested(call)
		index, childIndex = m.findToolEntry(call.ID)
		if index < 0 {
			return
		}
	}
	entry := m.toolEntryAt(index, childIndex)
	entry.toolActive = false
	entry.toolName = call.Name
	entry.toolInput = strings.TrimSpace(string(call.Input))
	entry.toolResult = textFromContent(result.Content)
	entry.toolMeta = result.Metadata
	entry.toolError = result.IsError
	if result.IsError {
		entry.text = "Tool failed: " + call.Name
	} else {
		entry.text = toolCompletedText(call.Name)
	}
	m.setToolEntryAt(index, childIndex, entry)
	m.refreshToolGroup(index)
	m.dirty = true
}

func (m *model) appendExplorationTool(entry transcriptEntry) bool {
	if !isExplorationTool(entry.toolName) {
		return false
	}
	index := len(m.entries) - 1
	if index >= 0 && m.entries[index].kind == transcriptToolGroup {
		m.entries[index].tools = append(m.entries[index].tools, entry)
		m.refreshToolGroup(index)
		return true
	}
	if index >= 0 && m.entries[index].kind == transcriptTool &&
		isExplorationTool(m.entries[index].toolName) {
		m.entries[index] = transcriptEntry{
			kind:  transcriptToolGroup,
			tools: []transcriptEntry{m.entries[index], entry},
		}
		m.refreshToolGroup(index)
		return true
	}
	if groupIndex := m.findCurrentExplorationGroup(); groupIndex >= 0 {
		m.entries[groupIndex].tools = append(m.entries[groupIndex].tools, entry)
		m.refreshToolGroup(groupIndex)
		return true
	}
	return false
}

func (m model) findCurrentExplorationGroup() int {
	for i := len(m.entries) - 1; i >= 0; i-- {
		switch m.entries[i].kind {
		case transcriptToolGroup:
			return i
		case transcriptUser:
			return -1
		}
	}
	return -1
}

func (m *model) refreshToolGroup(index int) {
	if index < 0 || index >= len(m.entries) || m.entries[index].kind != transcriptToolGroup {
		return
	}
	group := &m.entries[index]
	active := false
	failed := 0
	for _, tool := range group.tools {
		if tool.toolActive {
			active = true
		}
		if tool.toolError {
			failed++
		}
	}
	group.toolActive = active
	group.toolError = failed > 0
	group.text = toolGroupText(group.tools, active, failed)
}

func (m model) toolEntryAt(index int, childIndex int) transcriptEntry {
	if childIndex >= 0 {
		return m.entries[index].tools[childIndex]
	}
	return m.entries[index]
}

func (m *model) setToolEntryAt(index int, childIndex int, entry transcriptEntry) {
	if childIndex >= 0 {
		m.entries[index].tools[childIndex] = entry
		return
	}
	m.entries[index] = entry
}

func (m model) findToolEntry(callID string) (int, int) {
	if callID == "" {
		return -1, -1
	}
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].kind == transcriptTool && m.entries[i].toolCallID == callID {
			return i, -1
		}
		if m.entries[i].kind == transcriptToolGroup {
			for j := len(m.entries[i].tools) - 1; j >= 0; j-- {
				if m.entries[i].tools[j].toolCallID == callID {
					return i, j
				}
			}
		}
	}
	return -1, -1
}

func (m model) hasActiveTools() bool {
	for _, entry := range m.entries {
		if (entry.kind == transcriptTool || entry.kind == transcriptToolGroup) && entry.toolActive {
			return true
		}
	}
	return false
}

func (m *model) clearActiveTools() {
	for i := range m.entries {
		switch m.entries[i].kind {
		case transcriptTool:
			clearActiveToolEntry(&m.entries[i])
		case transcriptToolGroup:
			for j := range m.entries[i].tools {
				clearActiveToolEntry(&m.entries[i].tools[j])
			}
			m.refreshToolGroup(i)
		}
	}
}

func clearActiveToolEntry(entry *transcriptEntry) {
	if !entry.toolActive {
		return
	}
	entry.toolActive = false
	name := strings.TrimSpace(entry.toolName)
	if name == "" {
		name = strings.TrimSpace(entry.text)
	}
	if name != "" {
		entry.text = "Interrupted " + name
	}
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
		return m.confirmQuit()
	case "esc":
		m.quitArmed = false
		m.resolvePermission(types.PermissionDecisionDeny, "denied from TUI")
	case "enter":
		m.quitArmed = false
		m.resolvePermission(m.permission.Decision(), m.permission.Reason())
	default:
		m.quitArmed = false
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

func (m *model) interruptModel() {
	if m.stepCancel != nil {
		m.stepCancel()
	}
	m.stepCancel = nil
	m.activeStepID = 0
	m.busy = false
	m.showSpinner = false
	m.permission = nil
	m.status = "ready"
	m.clearActiveTools()
	m.appendStatus("Interrupted")
	m.resize()
	m.dirty = true
}

func (m *model) confirmQuit() tea.Cmd {
	if m.quitArmed {
		return quitCmd()
	}

	m.quitArmed = true
	m.status = "press ctrl+c again to quit"
	m.appendStatus("Press ctrl+c again to quit, or type /exit.")
	return nil
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

func (m *model) prepareTerminalScroll() {
	m.scrollPrinted = 0
	m.scrollHeader = m.renderHeaderText(m.width)
	m.scrollHeaderOut = false
	m.scrollReplay = true
	m.scrollPrinting = false
}

func (m *model) terminalScrollbackCmd() tea.Cmd {
	if !m.terminalScrollMode() || m.scrollPrinting {
		return nil
	}

	printed := m.scrollPrinted
	if printed > len(m.entries) {
		printed = len(m.entries)
	}

	headerOut := m.scrollHeaderOut
	lines := make([]string, 0, max(0, len(m.entries)-printed))
	for printed < len(m.entries) {
		if !m.isTerminalPrintableEntry(printed) {
			break
		}
		line, ok := m.renderScrollbackTranscriptEntry(m.entries[printed])
		printed++
		if ok {
			lines = append(lines, line)
		}
	}
	replayDone := printed >= len(m.entries)
	if len(lines) == 0 {
		if replayDone {
			m.scrollReplay = false
		}
		return nil
	}
	m.scrollPrinting = true

	return tea.Sequence(
		tea.Println(strings.Join(lines, m.entrySeparator())),
		func() tea.Msg {
			return terminalScrollbackPrintedMsg{
				printed:    printed,
				headerOut:  headerOut,
				replayDone: replayDone,
			}
		},
	)
}

func (m model) renderTerminalReplayTranscript() string {
	if !m.scrollReplay {
		return ""
	}
	lines := make([]string, 0, max(0, len(m.entries)-m.scrollPrinted))
	for index := m.scrollPrinted; index < len(m.entries); index++ {
		if !m.isTerminalPrintableEntry(index) {
			break
		}
		line, ok := m.renderScrollbackTranscriptEntry(m.entries[index])
		if ok {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, m.entrySeparator())
}

func (m model) renderTerminalDetailTranscript() string {
	if len(m.entries) == 0 {
		return ""
	}
	return m.styles.toolBlock.Render(m.renderTranscript())
}

func (m model) renderScrollbackTranscriptEntry(entry transcriptEntry) (string, bool) {
	collapsed := m
	collapsed.showEvents = false
	return collapsed.renderTranscriptEntry(entry)
}

func (m model) isTerminalPrintableEntry(index int) bool {
	if index < 0 || index >= len(m.entries) {
		return false
	}
	entry := m.entries[index]
	switch entry.kind {
	case transcriptAgent:
		return index != m.streaming
	case transcriptTool:
		return !entry.toolActive && (entry.toolResult != "" || entry.toolError)
	case transcriptToolGroup:
		if entry.toolActive {
			return false
		}
		for _, tool := range entry.tools {
			if tool.toolActive || (tool.toolResult == "" && !tool.toolError) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

func (m model) renderTerminalLiveTranscript() string {
	lines := make([]string, 0, 2)
	if m.streaming >= 0 && m.streaming < len(m.entries) {
		if line, ok := m.renderTranscriptEntry(m.entries[m.streaming]); ok {
			lines = append(lines, line)
		}
	}
	if active := m.renderActiveToolEntries(); active != "" {
		lines = append(lines, active)
	}
	if m.showSpinner {
		lines = append(lines, m.renderSpinnerMessage())
	}
	return strings.Join(lines, m.entrySeparator())
}

func (m model) renderActiveToolEntries() string {
	lines := make([]string, 0, 2)
	for _, entry := range m.entries {
		switch entry.kind {
		case transcriptTool:
			if entry.toolActive {
				lines = append(lines, m.renderToolEntry(entry))
			}
		case transcriptToolGroup:
			if entry.toolActive {
				lines = append(lines, m.renderToolGroupEntry(entry))
			}
		}
	}
	return strings.Join(lines, m.entrySeparator())
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
	case transcriptStatus:
		return m.renderStatusEntry(entry), true
	case transcriptTool:
		return m.renderToolEntry(entry), true
	case transcriptToolGroup:
		return m.renderToolGroupEntry(entry), true
	default:
		return entry.text, true
	}
}

func (m model) renderAgentMessage(text string) string {
	return m.harnessMessage().RenderAgent(text)
}

func (m model) renderSpinnerMessage() string {
	responding := m.spinner.View() + " " + m.thinking.view()
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

	block := m.styles.userBlock
	if !m.showBackground {
		block = block.Background(tuiBackground)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.userLabel.Render("You"),
		block.Width(m.messageWidth()).Render(message),
	)
}

func (m model) renderEvent(text string) string {
	if m.density == densityCompact {
		return m.styles.muted.Render(text)
	}

	return m.styles.eventBlock.Width(m.messageWidth()).Render(text)
}

func (m model) renderStatusEntry(entry transcriptEntry) string {
	line := m.styles.statusLine.Render(entry.text)
	if m.density == densityCompact {
		return line
	}
	return m.styles.statusLine.Width(m.messageWidth()).Render(line)
}

func (m model) messageWidth() int {
	return max(20, m.viewport.Width()-2)
}

func (m model) harnessMessageWidth() int {
	return max(20, m.viewport.Width())
}

func (m model) harnessMessage() harnessMessage {
	message := newHarnessMessage(
		m.harnessMessageWidth(),
		m.density,
		m.styles.agentLabel,
		m.styles.muted,
	)
	message.showBackground = m.showBackground
	return message
}

func (m model) entrySeparator() string {
	if m.density == densityCompact {
		return "\n"
	}

	return "\n\n"
}

func (m model) densityCompact() bool {
	return m.density == densityCompact
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
		if key == "diff" || key == "command" {
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

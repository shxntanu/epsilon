package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/events"
)

const maxSessionPickerRows = 8

type sessionPicker struct {
	sessions []events.SessionInfo
	filtered []int
	query    string
	cursor   int
	loading  bool
	err      string
}

func (m *model) openSessionPicker() tea.Cmd {
	if m.listSessions == nil {
		m.appendPlain(m.styles.error.Render("session picker is not available"))
		m.status = "ready"
		return nil
	}
	if m.busy {
		m.appendPlain(m.styles.error.Render("cannot resume while model is thinking"))
		return nil
	}

	m.sessionPicker = &sessionPicker{loading: true}
	m.status = "sessions:loading"
	m.resize()
	m.dirty = true

	listSessions := m.listSessions
	ctx := m.ctx
	return func() tea.Msg {
		sessions, err := listSessions(ctx)
		return sessionListMsg{sessions: sessions, err: err}
	}
}

func (m *model) applySessionList(msg sessionListMsg) {
	if m.sessionPicker == nil {
		return
	}

	m.sessionPicker.loading = false
	if msg.err != nil {
		m.sessionPicker.err = msg.err.Error()
		m.status = "sessions:error"
		m.resize()
		m.dirty = true
		return
	}

	m.sessionPicker.sessions = msg.sessions
	m.sessionPicker.filter(m.session.ID())
	if len(msg.sessions) == 0 {
		m.sessionPicker.err = "no persisted sessions"
		m.status = "sessions:empty"
	} else {
		m.status = "sessions"
	}
	m.resize()
	m.dirty = true
}

func (m *model) updateSessionPicker(msg tea.KeyPressMsg) tea.Cmd {
	if m.sessionPicker == nil {
		return nil
	}

	switch msg.Keystroke() {
	case "ctrl+c":
		return m.confirmQuit()
	case "esc":
		m.quitArmed = false
		m.sessionPicker = nil
		m.status = "ready"
		m.resize()
	case "up", "ctrl+p":
		m.quitArmed = false
		m.moveSessionSelection(-1)
	case "down", "ctrl+n":
		m.quitArmed = false
		m.moveSessionSelection(1)
	case "backspace", "ctrl+h":
		m.quitArmed = false
		m.sessionPicker.deleteQueryRune(m.session.ID())
	case "ctrl+u":
		m.quitArmed = false
		m.sessionPicker.query = ""
		m.sessionPicker.filter(m.session.ID())
	case "enter":
		m.quitArmed = false
		return m.applySelectedSession()
	default:
		m.quitArmed = false
		if m.sessionPicker.appendQueryText(msg.Key().Text, m.session.ID()) {
			m.status = "sessions"
		}
	}

	m.dirty = true
	return nil
}

func (m *model) moveSessionSelection(delta int) {
	if m.sessionPicker == nil || m.sessionPicker.loading || m.sessionPicker.err != "" {
		return
	}
	count := len(m.sessionPicker.filtered)
	if count == 0 {
		return
	}
	m.sessionPicker.cursor = (m.sessionPicker.cursor + delta + count) % count
}

func (m *model) applySelectedSession() tea.Cmd {
	if m.sessionPicker == nil || m.sessionPicker.loading || m.sessionPicker.err != "" {
		return nil
	}
	visible := m.sessionPicker.visibleSessions()
	if len(visible) == 0 {
		return nil
	}
	selected := visible[m.clampedSessionCursor()]
	m.sessionPicker = nil
	m.resize()
	return m.resumeChat(selected)
}

func (m *model) resumeChat(sessionInfo events.SessionInfo) tea.Cmd {
	sessionID := strings.TrimSpace(sessionInfo.ID)
	if sessionID == "" {
		m.appendPlain(m.styles.error.Render("session ID is empty"))
		return nil
	}
	if m.busy {
		m.appendPlain(m.styles.error.Render("cannot resume while model is thinking"))
		return nil
	}
	if m.resumeSession == nil {
		m.appendPlain(m.styles.error.Render("resume is not available"))
		return nil
	}

	m.status = "resuming"
	m.dirty = true
	resumeSession := m.resumeSession
	ctx := m.ctx
	return func() tea.Msg {
		sess, err := resumeSession(ctx, sessionID)
		if err != nil {
			return resumeSessionMsg{sessionID: sessionID, err: err}
		}
		history := sess.History()
		sub, err := sess.SubscribeLive()
		if err != nil {
			return resumeSessionMsg{sessionID: sessionID, err: err}
		}
		return resumeSessionMsg{
			sessionID: sessionID,
			title:     strings.TrimSpace(sessionInfo.Title),
			history:   history,
			session:   sess,
			sub:       sub,
		}
	}
}

func (m *model) startNewChat(discardCurrent bool) tea.Cmd {
	if m.busy {
		m.appendPlain(m.styles.error.Render("cannot start a new session while model is thinking"))
		return nil
	}
	if m.startSession == nil {
		m.appendPlain(m.styles.error.Render("new sessions are not available"))
		return nil
	}

	currentSessionID := m.session.ID()
	m.status = "starting"
	m.dirty = true
	startSession := m.startSession
	closeSession := m.closeSession
	deleteSession := m.deleteSession
	ctx := m.ctx
	return func() tea.Msg {
		if discardCurrent {
			if deleteSession == nil {
				return resumeSessionMsg{err: fmt.Errorf("discard is not available")}
			}
			if _, err := deleteSession(ctx, currentSessionID); err != nil {
				return resumeSessionMsg{err: err}
			}
		} else if closeSession != nil {
			if err := closeSession(ctx, currentSessionID); err != nil {
				return resumeSessionMsg{err: err}
			}
		}

		sess, err := startSession(ctx)
		if err != nil {
			return resumeSessionMsg{err: err}
		}
		sub, err := sess.Subscribe()
		if err != nil {
			return resumeSessionMsg{sessionID: sess.ID(), err: err}
		}
		return resumeSessionMsg{
			sessionID: sess.ID(),
			session:   sess,
			sub:       sub,
		}
	}
}

func (m *model) applyResumeSession(msg resumeSessionMsg) tea.Cmd {
	if msg.err != nil {
		m.status = "ready"
		m.appendPlain(m.styles.error.Render("resume: " + msg.err.Error()))
		return nil
	}
	if msg.session == nil || msg.sub == nil {
		m.status = "ready"
		m.appendPlain(m.styles.error.Render("resume: session missing"))
		return nil
	}

	if m.subscription != nil {
		m.subscription.Close()
	}

	m.session = msg.session
	m.subscription = msg.sub
	m.events = msg.sub.Events()
	m.entries = hydrateTranscriptEntries(msg.history)
	m.pendingUsers = nil
	m.streaming = -1
	m.permission = nil
	m.modelPicker = nil
	m.sessionPicker = nil
	m.showSpinner = false
	m.busy = false
	m.status = "ready"
	m.sessionTitle = strings.TrimSpace(msg.title)
	m.followOutput = true
	m.scrollback.Reset()
	m.scrollPrinting = false
	m.contextView = func() contextwindow.Summary {
		return msg.session.ContextSummary(nil)
	}
	m.resize()
	m.dirty = true
	return m.waitForEvent()
}

func (m model) clampedSessionCursor() int {
	if m.sessionPicker == nil || len(m.sessionPicker.filtered) == 0 || m.sessionPicker.cursor < 0 {
		return 0
	}
	if m.sessionPicker.cursor >= len(m.sessionPicker.filtered) {
		return len(m.sessionPicker.filtered) - 1
	}
	return m.sessionPicker.cursor
}

func (m model) sessionPickerHeight() int {
	if m.sessionPicker == nil {
		return 0
	}
	if m.sessionPicker.loading || m.sessionPicker.err != "" || len(m.sessionPicker.filtered) == 0 {
		return 4
	}
	return min(maxSessionPickerRows, len(m.sessionPicker.filtered)) + 4
}

func (m model) renderSessionPicker() (string, bool) {
	if m.sessionPicker == nil {
		return "", false
	}

	width := max(20, m.width-4)
	title := "Sessions"
	if m.sessionPicker.query != "" {
		title += " / " + m.sessionPicker.query
	}
	lines := []string{m.renderSelectorHeader(title, sessionPickerMeta(m.sessionPicker))}
	switch {
	case m.sessionPicker.loading:
		lines = append(lines, m.styles.muted.Render("Loading sessions..."))
	case m.sessionPicker.err != "":
		lines = append(lines, m.styles.error.Render(m.sessionPicker.err))
		lines = append(lines, m.renderSelectorHint("esc closes"))
	case len(m.sessionPicker.sessions) == 0:
		lines = append(lines, m.styles.muted.Render("No sessions available"))
	case len(m.sessionPicker.filtered) == 0:
		lines = append(lines, m.styles.muted.Render("No sessions match"))
		lines = append(lines, m.renderSelectorHint("type to search", "esc closes"))
	default:
		lines = append(lines, m.renderSessionRows(width-4)...)
		lines = append(lines, m.renderSelectorHint("type to search", "enter resumes", "esc closes"))
	}

	return m.styles.selector.Width(width).Render(strings.Join(lines, "\n")), true
}

func (m model) renderSessionRows(width int) []string {
	picker := m.sessionPicker
	if picker == nil {
		return nil
	}

	visible := picker.visibleSessions()
	start, end := modelWindow(picker.cursor, len(visible), maxSessionPickerRows)
	rows := make([]string, 0, end-start)
	current := m.session.ID()
	for i := start; i < end; i++ {
		sessionInfo := visible[i]
		line := formatSessionRow(sessionInfo, current, width)
		if i == picker.cursor {
			line = activeSelectorMarker(m.headerFrame) + " " + line
			line = m.styles.selectorActive.Width(max(1, width)).Render(line)
		} else {
			line = "  " + line
		}
		rows = append(rows, line)
	}
	return rows
}

func sessionPickerMeta(picker *sessionPicker) string {
	if picker == nil {
		return ""
	}
	if picker.loading {
		return "loading"
	}
	if picker.err != "" {
		return "needs attention"
	}
	if picker.query != "" {
		return fmt.Sprintf("%d matches", len(picker.filtered))
	}
	return fmt.Sprintf("%d saved", len(picker.sessions))
}

func (p *sessionPicker) appendQueryText(text string, current string) bool {
	if text == "" {
		return false
	}
	p.query += text
	p.filter(current)
	return true
}

func (p *sessionPicker) deleteQueryRune(current string) {
	if p.query == "" {
		return
	}
	runes := []rune(p.query)
	p.query = string(runes[:len(runes)-1])
	p.filter(current)
}

func (p *sessionPicker) filter(current string) {
	type scoredSession struct {
		index int
		score int
	}

	query := strings.ToLower(strings.TrimSpace(p.query))
	if query == "" {
		p.filtered = make([]int, len(p.sessions))
		for i := range p.sessions {
			p.filtered[i] = i
		}
		p.cursor = selectedSessionCursor(p.visibleSessions(), current)
		return
	}

	matches := make([]scoredSession, 0, len(p.sessions))
	for i, sess := range p.sessions {
		score := fuzzyTextScore(strings.ToLower(sessionSearchText(sess)), query)
		if score <= 0 {
			continue
		}
		if sess.ID == current {
			score += 100
		}
		matches = append(matches, scoredSession{index: i, score: score})
	}
	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return p.sessions[matches[i].index].ID < p.sessions[matches[j].index].ID
	})

	p.filtered = make([]int, 0, len(matches))
	for _, match := range matches {
		p.filtered = append(p.filtered, match.index)
	}
	p.cursor = 0
}

func (p sessionPicker) visibleSessions() []events.SessionInfo {
	sessions := make([]events.SessionInfo, 0, len(p.filtered))
	for _, index := range p.filtered {
		if index >= 0 && index < len(p.sessions) {
			sessions = append(sessions, p.sessions[index])
		}
	}
	return sessions
}

func selectedSessionCursor(sessions []events.SessionInfo, current string) int {
	for i, sess := range sessions {
		if sess.ID == current {
			return i
		}
	}
	return 0
}

func formatSessionRow(sessionInfo events.SessionInfo, current string, width int) string {
	parts := []string{sessionRowLabel(sessionInfo)}
	if sessionInfo.ID == current {
		parts = append(parts, "current")
	}
	return fitModelRow(strings.Join(parts, "  "), width)
}

func sessionSearchText(sessionInfo events.SessionInfo) string {
	if title := strings.TrimSpace(sessionInfo.Title); title != "" {
		return title + " " + sessionInfo.ID
	}
	return sessionInfo.ID
}

func sessionRowLabel(sessionInfo events.SessionInfo) string {
	if title := strings.TrimSpace(sessionInfo.Title); title != "" {
		return title + " (" + sessionInfo.ID + ")"
	}
	return sessionInfo.ID
}

package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/skills"
	"github.com/shxntanu/epsilon/core/slash"
)

const maxSlashSelectorMatches = 6

func (m *model) applySlashResult(result slash.Result, err error) tea.Cmd {
	m.followOutput = true
	m.dirty = true
	m.showSpinner = false
	m.resize()

	if err != nil {
		message := err.Error()
		if errors.Is(err, slash.ErrUnknownCommand) {
			message += ". Type /help for commands."
		}
		m.status = "ready"
		m.appendPlain(m.styles.error.Render(message))
		return nil
	}

	switch result.Action {
	case slash.ActionClearTranscript:
		m.entries = nil
		m.streaming = -1
		m.scrollback.Reset()
		m.scrollPrinting = false
		m.status = "ready"
		if result.Message != "" {
			m.appendPlain(m.styles.muted.Render(result.Message))
		}
	case slash.ActionSetEvents:
		m.showEvents = result.Bool
		m.status = "details:" + onOff(m.showEvents)
		m.appendSlashMessage(result.Message)
	case slash.ActionSetDensity:
		m.density = densityModeFromSlash(result.Density)
		m.resize()
		m.status = "density:" + m.density.Label()
		m.appendSlashMessage(result.Message)
	case slash.ActionSetBackground:
		m.showBackground = result.Bool
		m.status = "background:" + onOff(m.showBackground)
		m.appendSlashMessage(result.Message)
	case slash.ActionPickModel:
		return m.openModelPicker()
	case slash.ActionPickSession:
		return m.openSessionPicker()
	case slash.ActionSetModel:
		if m.setModel == nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("model changes are not available"))
			return nil
		}
		if err := m.setModel(m.ctx, result.Model); err != nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render(err.Error()))
			return nil
		}
		m.status = "model:" + result.Model
		m.appendSlashMessage(result.Message)
	case slash.ActionSetEffort:
		if m.setEffort == nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("effort changes are not available"))
			return nil
		}
		if err := m.setEffort(result.Effort); err != nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render(err.Error()))
			return nil
		}
		statusEffort := result.Effort
		if statusEffort == "" {
			statusEffort = "default"
		}
		m.status = "effort:" + statusEffort
		m.appendSlashMessage(result.Message)
	case slash.ActionSetPlanMode:
		if m.setPlanMode == nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("plan mode changes are not available"))
			return nil
		}
		if err := m.setPlanMode(result.Bool); err != nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render(err.Error()))
			return nil
		}
		m.status = "plan:" + onOff(currentBool(m.currentPlanMode))
		m.appendSlashMessage(result.Message)
	case slash.ActionNewSession:
		return m.startNewChat(false)
	case slash.ActionDiscardSession:
		return m.startNewChat(true)
	case slash.ActionResumeChat:
		return m.resumeChat(events.SessionInfo{ID: result.Session})
	case slash.ActionRenameSession:
		if m.renameSession == nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("rename is not available"))
			return nil
		}
		title := strings.TrimSpace(result.Model)
		sessionID := strings.TrimSpace(result.Session)
		if title == "" || sessionID == "" {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("usage: /rename <title...>"))
			return nil
		}
		ok, err := m.renameSession(m.ctx, sessionID, title)
		m.status = "ready"
		if err != nil {
			m.appendPlain(m.styles.error.Render(err.Error()))
			return nil
		}
		if !ok {
			m.appendPlain(m.styles.error.Render("session not found"))
			return nil
		}
		if sessionID == m.session.ID() {
			m.sessionTitle = title
		}
		m.appendPlain(m.styles.muted.Render("renamed current session: " + title))
	case slash.ActionRefreshSkills:
		if m.refreshSkills == nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render("skill refresh is not available"))
			return nil
		}
		count, err := m.refreshSkills()
		m.status = "skills:refreshed"
		if err != nil {
			m.status = "ready"
			m.appendPlain(m.styles.error.Render(err.Error()))
			return nil
		}
		m.appendSlashMessage(fmt.Sprintf("refreshed %d skills", count))
	case slash.ActionQuit:
		return quitCmd()
	default:
		m.status = "ready"
		m.appendSlashMessage(result.Message)
	}

	return nil
}

func (m *model) appendSlashMessage(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}

	m.appendPlain(message)
}

func (m model) slashExecution() slash.Execution {
	return slash.Execution{
		SessionID: m.session.ID(),
		State: slash.State{
			Busy:             m.busy,
			AwaitingApproval: m.permission != nil,
			EventsVisible:    m.showEvents,
			Density:          slashDensity(m.density),
			Background:       m.showBackground,
			MessageCount:     len(m.session.Messages()),
			Model:            currentString(m.currentModel),
			Effort:           currentString(m.currentEffort),
			PlanMode:         currentBool(m.currentPlanMode),
		},
	}
}

func currentString(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

func currentBool(fn func() bool) bool {
	return fn != nil && fn()
}

func slashDensity(density densityMode) slash.Density {
	if density == densityCompact {
		return slash.DensityCompact
	}

	return slash.DensityComfortable
}

func densityModeFromSlash(density slash.Density) densityMode {
	if density == slash.DensityCompact {
		return densityCompact
	}

	return densityComfortable
}

func (m model) slashSelectorState() (string, []slash.Match, bool) {
	query, ok := slashSelectorQuery(m.composer.Value())
	if !ok || m.slash == nil {
		return "", nil, false
	}

	matches := m.slash.Match(query, maxSlashSelectorMatches)
	if len(matches) == 0 {
		return query, nil, true
	}
	return query, matches, true
}

func (m model) slashSelectorActive() bool {
	_, _, ok := m.slashSelectorState()
	return ok
}

func (m model) skillSelectorState() (string, []skills.Skill, bool) {
	query, ok := skillSelectorQuery(m.composer.Value())
	if !ok || m.listSkills == nil {
		return "", nil, false
	}

	matches := matchSkills(m.listSkills(), query, maxSlashSelectorMatches)
	return query, matches, true
}

func (m model) skillSelectorActive() bool {
	_, _, ok := m.skillSelectorState()
	return ok
}

func slashSelectorQuery(input string) (string, bool) {
	text := strings.TrimLeft(input, " \t")
	if text == "" || !strings.HasPrefix(text, "/") || strings.HasPrefix(text, "//") {
		return "", false
	}
	if strings.Contains(text, "\n") {
		return "", false
	}

	body := strings.TrimPrefix(text, "/")
	if strings.ContainsAny(body, " \t") {
		return "", false
	}
	return body, true
}

func skillSelectorQuery(input string) (string, bool) {
	token, _, ok := leadingSkillToken(input)
	if !ok {
		return "", false
	}
	if strings.ContainsAny(token, " \t\n") {
		return "", false
	}
	text := strings.TrimLeft(input, " \t")
	if len(text) > len(token)+1 && strings.ContainsAny(text[len(token)+1:], " \t\n") {
		return "", false
	}
	return token, true
}

func (m model) slashSelectorHeight() int {
	_, matches, ok := m.slashSelectorState()
	if !ok {
		return 0
	}
	if len(matches) == 0 {
		return 5
	}
	return len(matches) + 4
}

func (m model) skillSelectorHeight() int {
	_, matches, ok := m.skillSelectorState()
	if !ok {
		return 0
	}
	if len(matches) == 0 {
		return 5
	}
	return len(matches) + 4
}

func (m model) renderSlashSelector() (string, bool) {
	query, matches, ok := m.slashSelectorState()
	if !ok {
		return "", false
	}

	width := max(20, m.width-4)
	lines := []string{m.renderSelectorHeader("Slash commands", slashSelectorMeta(query, len(matches)))}
	if len(matches) == 0 {
		lines = append(lines, m.styles.muted.Render("No commands match /"+query))
		lines = append(lines, m.renderSelectorHint("keep typing", "esc closes"))
		return m.styles.selector.Width(width).Render(strings.Join(lines, "\n")), true
	}

	selected := m.clampedSlashCursor(len(matches))
	for i, match := range matches {
		lines = append(lines, m.renderSlashSelectorRow(match, i == selected, width-4))
	}
	lines = append(lines, m.renderSelectorHint("tab completes", "enter runs", "esc closes"))

	return m.styles.selector.Width(width).Render(strings.Join(lines, "\n")), true
}

func (m model) renderSkillSelector() (string, bool) {
	query, matches, ok := m.skillSelectorState()
	if !ok {
		return "", false
	}

	width := max(20, m.width-4)
	lines := []string{m.renderSelectorHeader("Skills", skillSelectorMeta(query, len(matches)))}
	if len(matches) == 0 {
		lines = append(lines, m.styles.muted.Render("No skills match !"+query))
		lines = append(lines, m.renderSelectorHint("keep typing", "esc closes"))
		return m.skillSelectorStyle().Width(width).Render(strings.Join(lines, "\n")), true
	}

	selected := m.clampedSkillCursor(len(matches))
	for i, skill := range matches {
		line := m.formatSkillRow(skill, width-4)
		if i == selected {
			line = activeSelectorMarker(m.headerFrame) + " " + line
			line = m.styles.selectorActive.Width(max(1, width-4)).Render(line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, m.renderSelectorHint("tab completes", "enter selects", "esc closes"))

	return m.skillSelectorStyle().Width(width).Render(strings.Join(lines, "\n")), true
}

func slashSelectorMeta(query string, matches int) string {
	if query == "" {
		return fmt.Sprintf("%d commands", matches)
	}
	return fmt.Sprintf("/%s · %d matches", query, matches)
}

func skillSelectorMeta(query string, matches int) string {
	if query == "" {
		return fmt.Sprintf("%d skills", matches)
	}
	return fmt.Sprintf("!%s · %d matches", query, matches)
}

func (m model) renderSlashSelectorRow(match slash.Match, selected bool, width int) string {
	name := m.styles.selectorKey.Render("/" + match.Command.Name)
	description := strings.TrimSpace(match.Command.Description)
	if description != "" {
		description = m.styles.muted.Render(description)
	}
	if match.Alias != "" && match.Alias != match.Command.Name {
		alias := m.styles.selectorMeta.Render("alias /" + match.Alias)
		if description == "" {
			description = alias
		} else {
			description += " " + alias
		}
	}

	line := fitSelectorRow(name, description, width-2)
	if selected {
		line = activeSelectorMarker(m.headerFrame) + " " + line
		return m.styles.selectorActive.Width(max(1, width)).Render(line)
	}
	return "  " + line
}

func (m model) formatSkillRow(skill skills.Skill, width int) string {
	name := m.styles.selectorKey.Render("!" + skill.Name)
	description := strings.TrimSpace(skill.Description)
	if description != "" {
		description = m.styles.muted.Render(description)
	}
	return fitSelectorRow(name, description, width-2)
}

func (m *model) moveSlashSelection(delta int) bool {
	_, matches, ok := m.slashSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	m.slashCursor = (m.clampedSlashCursor(len(matches)) + delta + len(matches)) % len(matches)
	return true
}

func (m *model) moveSkillSelection(delta int) bool {
	_, matches, ok := m.skillSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	m.skillCursor = (m.clampedSkillCursor(len(matches)) + delta + len(matches)) % len(matches)
	return true
}

func (m *model) completeSlashSelection() bool {
	_, matches, ok := m.slashSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	selected := m.clampedSlashCursor(len(matches))
	m.composer.SetValue("/" + matches[selected].Command.Name + " ")
	m.slashCursor = 0
	m.resize()
	return true
}

func (m *model) completeSkillSelection() bool {
	_, matches, ok := m.skillSelectorState()
	if !ok || len(matches) == 0 {
		return false
	}

	selected := m.clampedSkillCursor(len(matches))
	skill := matches[selected]
	m.composer.SetValue("!" + skill.Name + " ")
	m.skillCursor = 0
	m.resize()
	return true
}

func (m *model) completeIncompleteSlashInput() bool {
	query, _, ok := m.slashSelectorState()
	if !ok {
		return false
	}

	if _, exact := m.slash.Get(query); exact {
		return false
	}

	return m.completeSlashSelection()
}

func (m model) clampedSlashCursor(length int) int {
	if length <= 0 || m.slashCursor < 0 {
		return 0
	}
	if m.slashCursor >= length {
		return length - 1
	}
	return m.slashCursor
}

func (m model) clampedSkillCursor(length int) int {
	if length <= 0 || m.skillCursor < 0 {
		return 0
	}
	if m.skillCursor >= length {
		return length - 1
	}
	return m.skillCursor
}

func (m model) skillComposerMode() bool {
	return m.skillPicker != nil || m.skillSelectorActive()
}

func (m model) composerSkillPrefix() string {
	token, _, ok := leadingSkillToken(m.composer.Value())
	if !ok {
		return ""
	}
	return token
}

func (m model) skillSelectorStyle() lipgloss.Style {
	return m.styles.selector.BorderForeground(tuiAccentTool)
}

func matchSkills(skillList []skills.Skill, query string, limit int) []skills.Skill {
	type scoredSkill struct {
		skill skills.Skill
		score int
	}

	query = strings.ToLower(strings.TrimSpace(query))
	matches := make([]scoredSkill, 0, len(skillList))
	for _, skill := range skillList {
		score := skillSearchScore(skill, query)
		if score <= 0 {
			continue
		}
		matches = append(matches, scoredSkill{skill: skill, score: score})
	}
	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].skill.Name < matches[j].skill.Name
	})

	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	results := make([]skills.Skill, 0, len(matches))
	for _, match := range matches {
		results = append(results, match.skill)
	}
	return results
}

func leadingSkillToken(input string) (string, string, bool) {
	text := strings.TrimLeft(input, " \t")
	if text == "" || !strings.HasPrefix(text, "!") || strings.HasPrefix(text, "!!") {
		return "", "", false
	}

	body := strings.TrimPrefix(text, "!")
	if body == "" {
		return "", "", true
	}
	tokenEnd := len(body)
	for i, r := range body {
		if r == ' ' || r == '\t' || r == '\n' {
			tokenEnd = i
			break
		}
	}
	return body[:tokenEnd], strings.TrimSpace(body[tokenEnd:]), true
}

func selectorStyles(base styles) styles {
	base.selector = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tuiAccentCommand).
		Background(tuiSurface).
		Foreground(tuiInk).
		Padding(0, 1)
	base.selectorActive = lipgloss.NewStyle().
		Background(tuiSurface3).
		Foreground(tuiInkStrong).
		Bold(true)
	base.selectorTitle = lipgloss.NewStyle().
		Background(tuiSurface).
		Foreground(tuiAccentCommand).
		Bold(true)
	base.selectorMeta = lipgloss.NewStyle().
		Background(tuiSurface2).
		Foreground(tuiAccentInfo).
		Padding(0, 1)
	base.selectorKey = lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiInkStrong).
		Bold(true)
	base.selectorHint = lipgloss.NewStyle().
		Background(tuiSurface).
		Foreground(tuiFaint)
	return base
}

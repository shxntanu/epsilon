package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shxntanu/epsilon/core/skills"
)

const maxSkillPickerRows = 8

type skillPicker struct {
	skills   []skills.Skill
	filtered []int
	query    string
	cursor   int
	err      string
}

func (m *model) openSkillPicker() tea.Cmd {
	if m.listSkills == nil {
		m.appendPlain(m.styles.error.Render("skill picker is not available"))
		m.status = "ready"
		return nil
	}

	m.skillPicker = &skillPicker{skills: m.listSkills()}
	m.skillPicker.filter(activeSkillName(currentSkill(m.currentSkill)))
	if len(m.skillPicker.skills) == 0 {
		m.skillPicker.err = "no skills installed"
		m.status = "skills:empty"
	} else {
		m.status = "skills"
	}
	m.resize()
	m.dirty = true
	return nil
}

func (m *model) updateSkillPicker(msg tea.KeyPressMsg) tea.Cmd {
	if m.skillPicker == nil {
		return nil
	}

	switch msg.Keystroke() {
	case "ctrl+c":
		return m.confirmQuit()
	case "esc":
		m.quitArmed = false
		m.skillPicker = nil
		m.status = "ready"
		m.resize()
	case "up", "ctrl+p":
		m.quitArmed = false
		m.moveSkillPickerSelection(-1)
	case "down", "ctrl+n":
		m.quitArmed = false
		m.moveSkillPickerSelection(1)
	case "backspace", "ctrl+h":
		m.quitArmed = false
		m.skillPicker.deleteQueryRune(activeSkillName(currentSkill(m.currentSkill)))
	case "ctrl+u":
		m.quitArmed = false
		m.skillPicker.query = ""
		m.skillPicker.filter(activeSkillName(currentSkill(m.currentSkill)))
	case "tab", "enter":
		m.quitArmed = false
		m.applySelectedSkill()
	default:
		m.quitArmed = false
		if m.skillPicker.appendQueryText(msg.Key().Text,
			activeSkillName(currentSkill(m.currentSkill))) {
			m.status = "skills"
		}
	}

	m.dirty = true
	return nil
}

func (m *model) moveSkillPickerSelection(delta int) {
	if m.skillPicker == nil || m.skillPicker.err != "" {
		return
	}
	skillCount := len(m.skillPicker.filtered)
	if skillCount == 0 {
		return
	}
	m.skillPicker.cursor = (m.skillPicker.cursor + delta + skillCount) % skillCount
}

func (m *model) applySelectedSkill() {
	if m.skillPicker == nil || m.skillPicker.err != "" {
		return
	}
	visible := m.skillPicker.visibleSkills()
	if len(visible) == 0 {
		return
	}
	selected := visible[m.clampedSkillPickerCursor()]
	m.skillPicker = nil
	m.composer.SetValue("!" + selected.Name + " ")
	m.status = "ready"
	m.resize()
}

func (m *model) clearSkillSelection() {
	if m.clearSkill != nil {
		m.clearSkill()
	}
	m.status = "ready"
	m.resize()
	m.dirty = true
}

func (m model) clampedSkillPickerCursor() int {
	if m.skillPicker == nil || len(m.skillPicker.filtered) == 0 || m.skillPicker.cursor < 0 {
		return 0
	}
	if m.skillPicker.cursor >= len(m.skillPicker.filtered) {
		return len(m.skillPicker.filtered) - 1
	}
	return m.skillPicker.cursor
}

func (m model) skillPickerHeight() int {
	if m.skillPicker == nil {
		return 0
	}
	if m.skillPicker.err != "" || len(m.skillPicker.filtered) == 0 {
		return 4
	}
	return min(maxSkillPickerRows, len(m.skillPicker.filtered)) + 4
}

func (m model) renderSkillPicker() (string, bool) {
	if m.skillPicker == nil {
		return "", false
	}

	width := max(20, m.width-4)
	title := "Skills"
	if m.skillPicker.query != "" {
		title += " / " + m.skillPicker.query
	}
	lines := []string{m.renderSelectorHeader(title, skillPickerMeta(m.skillPicker))}
	switch {
	case m.skillPicker.err != "":
		lines = append(lines, m.styles.error.Render(m.skillPicker.err))
		lines = append(lines, m.renderSelectorHint("esc closes"))
	case len(m.skillPicker.skills) == 0:
		lines = append(lines, m.styles.muted.Render("No skills installed"))
	case len(m.skillPicker.filtered) == 0:
		lines = append(lines, m.styles.muted.Render("No skills match"))
		lines = append(lines, m.renderSelectorHint("type to search", "esc closes"))
	default:
		lines = append(lines, m.renderSkillRows(width-4)...)
		lines = append(lines, m.renderSelectorHint("type to search", "enter selects", "esc closes"))
	}

	return m.skillSelectorStyle().Width(width).Render(strings.Join(lines, "\n")), true
}

func (m model) renderSkillRows(width int) []string {
	picker := m.skillPicker
	if picker == nil {
		return nil
	}

	visible := picker.visibleSkills()
	start, end := modelWindow(picker.cursor, len(visible), maxSkillPickerRows)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		skill := visible[i]
		line := m.formatSkillRow(skill, width)
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

func skillPickerMeta(picker *skillPicker) string {
	if picker == nil {
		return ""
	}
	if picker.err != "" {
		return "needs attention"
	}
	if picker.query != "" {
		return fmt.Sprintf("%d matches", len(picker.filtered))
	}
	return fmt.Sprintf("%d available", len(picker.skills))
}

func (p *skillPicker) appendQueryText(text string, current string) bool {
	if text == "" {
		return false
	}
	p.query += text
	p.filter(current)
	return true
}

func (p *skillPicker) deleteQueryRune(current string) {
	if p.query == "" {
		return
	}
	runes := []rune(p.query)
	p.query = string(runes[:len(runes)-1])
	p.filter(current)
}

func (p *skillPicker) filter(current string) {
	type scoredSkill struct {
		index int
		score int
	}

	query := strings.ToLower(strings.TrimSpace(p.query))
	if query == "" {
		p.filtered = make([]int, len(p.skills))
		for i := range p.skills {
			p.filtered[i] = i
		}
		p.cursor = selectedSkillCursor(p.visibleSkills(), current)
		return
	}

	matches := make([]scoredSkill, 0, len(p.skills))
	for i, skill := range p.skills {
		score := skillSearchScore(skill, query)
		if score <= 0 {
			continue
		}
		if skill.Name == current {
			score += 100
		}
		matches = append(matches, scoredSkill{index: i, score: score})
	}
	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return p.skills[matches[i].index].Name < p.skills[matches[j].index].Name
	})

	p.filtered = make([]int, 0, len(matches))
	for _, match := range matches {
		p.filtered = append(p.filtered, match.index)
	}
	p.cursor = 0
}

func (p skillPicker) visibleSkills() []skills.Skill {
	skillList := make([]skills.Skill, 0, len(p.filtered))
	for _, index := range p.filtered {
		if index >= 0 && index < len(p.skills) {
			skillList = append(skillList, p.skills[index])
		}
	}
	return skillList
}

func selectedSkillCursor(skillList []skills.Skill, current string) int {
	for i, skill := range skillList {
		if skill.Name == current {
			return i
		}
	}
	return 0
}

func skillSearchScore(skill skills.Skill, query string) int {
	if query == "" {
		return 1
	}
	best := 0
	for _, candidate := range []string{skill.Name, skill.Source, skill.Description} {
		score := fuzzyTextScore(strings.ToLower(strings.TrimSpace(candidate)), query)
		if score > best {
			best = score
		}
	}
	return best
}

func currentSkill(fn func() *skills.Skill) *skills.Skill {
	if fn == nil {
		return nil
	}
	return fn()
}

func activeSkillName(skill *skills.Skill) string {
	if skill == nil {
		return ""
	}
	return strings.TrimSpace(skill.Name)
}

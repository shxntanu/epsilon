package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shxntanu/epsilon/core/types"
)

const maxModelPickerRows = 8

type modelPicker struct {
	models   []types.ModelInfo
	filtered []int
	query    string
	cursor   int
	loading  bool
	err      string
}

func (m *model) openModelPicker() tea.Cmd {
	if m.listModels == nil {
		m.appendPlain(m.styles.error.Render("model picker is not available"))
		m.status = "ready"
		return nil
	}

	m.modelPicker = &modelPicker{loading: true}
	m.status = "models:loading"
	m.resize()
	m.dirty = true

	listModels := m.listModels
	ctx := m.ctx
	return func() tea.Msg {
		models, err := listModels(ctx)
		return modelListMsg{models: models, err: err}
	}
}

func (m *model) applyModelList(msg modelListMsg) {
	if m.modelPicker == nil {
		return
	}

	m.modelPicker.loading = false
	if msg.err != nil {
		m.modelPicker.err = msg.err.Error()
		m.status = "models:error"
		m.resize()
		m.dirty = true
		return
	}

	m.modelPicker.models = msg.models
	m.modelPicker.filter(currentString(m.currentModel))
	if len(msg.models) == 0 {
		m.modelPicker.err = "provider returned no models"
		m.status = "models:empty"
	} else {
		m.status = "models"
	}
	m.resize()
	m.dirty = true
}

func (m *model) updateModelPicker(msg tea.KeyPressMsg) tea.Cmd {
	if m.modelPicker == nil {
		return nil
	}

	switch msg.Keystroke() {
	case "ctrl+c":
		return m.confirmQuit()
	case "esc":
		m.quitArmed = false
		m.modelPicker = nil
		m.status = "ready"
		m.resize()
	case "up", "ctrl+p":
		m.quitArmed = false
		m.moveModelSelection(-1)
	case "down", "ctrl+n":
		m.quitArmed = false
		m.moveModelSelection(1)
	case "backspace", "ctrl+h":
		m.quitArmed = false
		m.modelPicker.deleteQueryRune(currentString(m.currentModel))
	case "ctrl+u":
		m.quitArmed = false
		m.modelPicker.query = ""
		m.modelPicker.filter(currentString(m.currentModel))
	case "enter":
		m.quitArmed = false
		m.applySelectedModel()
	default:
		m.quitArmed = false
		if m.modelPicker.appendQueryText(msg.Key().Text, currentString(m.currentModel)) {
			m.status = "models"
		}
	}

	m.dirty = true
	return nil
}

func (m *model) moveModelSelection(delta int) {
	if m.modelPicker == nil || m.modelPicker.loading || m.modelPicker.err != "" {
		return
	}
	modelCount := len(m.modelPicker.filtered)
	if modelCount == 0 {
		return
	}
	m.modelPicker.cursor = (m.modelPicker.cursor + delta + modelCount) % modelCount
}

func (m *model) applySelectedModel() {
	if m.modelPicker == nil || m.modelPicker.loading || m.modelPicker.err != "" {
		return
	}
	visible := m.modelPicker.visibleModels()
	if len(visible) == 0 {
		return
	}
	if m.setModel == nil {
		m.modelPicker.err = "model changes are not available"
		m.status = "models:error"
		m.resize()
		return
	}

	selected := visible[m.clampedModelCursor()]
	model := modelSelectionID(selected)
	if err := m.setModel(m.ctx, model); err != nil {
		m.modelPicker.err = err.Error()
		m.status = "models:error"
		m.resize()
		return
	}

	m.modelPicker = nil
	m.status = "model:" + model
	m.appendSlashMessage("model " + model)
	m.resize()
}

func (m model) clampedModelCursor() int {
	if m.modelPicker == nil || len(m.modelPicker.filtered) == 0 || m.modelPicker.cursor < 0 {
		return 0
	}
	if m.modelPicker.cursor >= len(m.modelPicker.filtered) {
		return len(m.modelPicker.filtered) - 1
	}
	return m.modelPicker.cursor
}

func (m model) modelPickerHeight() int {
	if m.modelPicker == nil {
		return 0
	}
	if m.modelPicker.loading || m.modelPicker.err != "" || len(m.modelPicker.filtered) == 0 {
		return 4
	}
	return min(maxModelPickerRows, len(m.modelPicker.filtered)) + 4
}

func (m model) renderModelPicker() (string, bool) {
	if m.modelPicker == nil {
		return "", false
	}

	width := max(20, m.width-4)
	title := "Models"
	if m.modelPicker.query != "" {
		title += " / " + m.modelPicker.query
	}
	lines := []string{m.styles.selectorTitle.Render(title)}
	switch {
	case m.modelPicker.loading:
		lines = append(lines, m.styles.muted.Render("Loading models..."))
	case m.modelPicker.err != "":
		lines = append(lines, m.styles.error.Render(m.modelPicker.err))
		lines = append(lines, m.styles.muted.Render("Esc closes"))
	case len(m.modelPicker.models) == 0:
		lines = append(lines, m.styles.muted.Render("No models available"))
	case len(m.modelPicker.filtered) == 0:
		lines = append(lines, m.styles.muted.Render("No models match"))
		lines = append(lines, m.styles.muted.Render("Type to search, Esc closes"))
	default:
		lines = append(lines, m.renderModelRows(width-4)...)
		lines = append(lines, m.styles.muted.Render("Type to search, Enter selects, Esc closes"))
	}

	return m.styles.selector.Width(width).Render(strings.Join(lines, "\n")), true
}

func (m model) renderModelRows(width int) []string {
	picker := m.modelPicker
	if picker == nil {
		return nil
	}

	visible := picker.visibleModels()
	start, end := modelWindow(picker.cursor, len(visible), maxModelPickerRows)
	rows := make([]string, 0, end-start)
	current := currentString(m.currentModel)
	for i := start; i < end; i++ {
		model := visible[i]
		line := formatModelRow(model, current, width)
		if i == picker.cursor {
			line = m.styles.selectorActive.Render(line)
		}
		rows = append(rows, line)
	}
	return rows
}

func (p *modelPicker) appendQueryText(text string, current string) bool {
	if text == "" {
		return false
	}
	p.query += text
	p.filter(current)
	return true
}

func (p *modelPicker) deleteQueryRune(current string) {
	if p.query == "" {
		return
	}
	runes := []rune(p.query)
	p.query = string(runes[:len(runes)-1])
	p.filter(current)
}

func (p *modelPicker) filter(current string) {
	type scoredModel struct {
		index int
		score int
	}

	query := strings.ToLower(strings.TrimSpace(p.query))
	if query == "" {
		p.filtered = make([]int, len(p.models))
		for i := range p.models {
			p.filtered[i] = i
		}
		p.cursor = selectedModelCursor(p.visibleModels(), current)
		return
	}

	matches := make([]scoredModel, 0, len(p.models))
	for i, model := range p.models {
		score := modelSearchScore(model, query)
		if score <= 0 {
			continue
		}
		if modelMatchesCurrent(model, current) {
			score += 100
		}
		matches = append(matches, scoredModel{index: i, score: score})
	}
	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return modelDisplayName(p.models[matches[i].index]) <
			modelDisplayName(p.models[matches[j].index])
	})

	p.filtered = make([]int, 0, len(matches))
	for _, match := range matches {
		p.filtered = append(p.filtered, match.index)
	}
	p.cursor = 0
}

func (p modelPicker) visibleModels() []types.ModelInfo {
	models := make([]types.ModelInfo, 0, len(p.filtered))
	for _, index := range p.filtered {
		if index >= 0 && index < len(p.models) {
			models = append(models, p.models[index])
		}
	}
	return models
}

func selectedModelCursor(models []types.ModelInfo, current string) int {
	for i, model := range models {
		if modelMatchesCurrent(model, current) {
			return i
		}
	}
	return 0
}

func modelWindow(cursor int, count int, limit int) (int, int) {
	if count <= limit {
		return 0, count
	}
	half := limit / 2
	start := cursor - half
	if start < 0 {
		start = 0
	}
	if start+limit > count {
		start = count - limit
	}
	return start, start + limit
}

func modelSelectionID(model types.ModelInfo) string {
	if strings.TrimSpace(model.ID) != "" {
		return strings.TrimSpace(model.ID)
	}
	if strings.TrimSpace(model.ProviderModel) != "" {
		return strings.TrimSpace(model.ProviderModel)
	}
	return strings.TrimSpace(model.Name)
}

func modelDisplayName(model types.ModelInfo) string {
	if strings.TrimSpace(model.Name) != "" {
		return strings.TrimSpace(model.Name)
	}
	return modelSelectionID(model)
}

func modelMatchesCurrent(model types.ModelInfo, current string) bool {
	current = strings.TrimSpace(current)
	if current == "" {
		return false
	}
	values := []string{model.ID, model.Name, model.ProviderModel}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == current || strings.TrimPrefix(value, model.Provider+"/") == current ||
			strings.TrimPrefix(current, model.Provider+"/") == value {
			return true
		}
	}
	return false
}

func formatModelRow(model types.ModelInfo, current string, width int) string {
	name := modelDisplayName(model)
	if name == "" {
		name = "(unnamed)"
	}

	parts := []string{name}
	if modelMatchesCurrent(model, current) {
		parts = append(parts, "current")
	}

	return fitModelRow(strings.Join(parts, "  "), width)
}

func fitModelRow(text string, width int) string {
	if width <= 1 || len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return strings.TrimSpace(text[:max(0, width-3)]) + "..."
}

func modelSearchScore(model types.ModelInfo, query string) int {
	if query == "" {
		return 1
	}
	best := 0
	for _, candidate := range []string{model.ID, model.Name, model.ProviderModel} {
		score := fuzzyTextScore(strings.ToLower(strings.TrimSpace(candidate)), query)
		if score > best {
			best = score
		}
	}
	return best
}

func fuzzyTextScore(candidate string, query string) int {
	if candidate == "" {
		return 0
	}
	if candidate == query {
		return 1000 + len(candidate)
	}
	if strings.HasPrefix(candidate, query) {
		return 800 - (len(candidate) - len(query))
	}
	if strings.Contains(candidate, query) {
		return 600 - strings.Index(candidate, query)
	}

	score := 400
	last := -1
	for _, ch := range query {
		idx := strings.IndexRune(candidate[last+1:], ch)
		if idx < 0 {
			return 0
		}
		next := last + 1 + idx
		score -= idx
		last = next
	}
	return score - len(candidate)
}

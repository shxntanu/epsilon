package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shxntanu/epsilon/core/types"
)

const maxModelPickerRows = 8

type modelPicker struct {
	models  []types.ModelInfo
	cursor  int
	loading bool
	err     string
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
	m.modelPicker.cursor = selectedModelCursor(msg.models, currentString(m.currentModel))
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
		return quitCmd()
	case "esc":
		m.modelPicker = nil
		m.status = "ready"
		m.resize()
	case "up", "ctrl+p":
		m.moveModelSelection(-1)
	case "down", "ctrl+n":
		m.moveModelSelection(1)
	case "enter":
		m.applySelectedModel()
	}

	m.dirty = true
	return nil
}

func (m *model) moveModelSelection(delta int) {
	if m.modelPicker == nil || m.modelPicker.loading || m.modelPicker.err != "" {
		return
	}
	modelCount := len(m.modelPicker.models)
	if modelCount == 0 {
		return
	}
	m.modelPicker.cursor = (m.modelPicker.cursor + delta + modelCount) % modelCount
}

func (m *model) applySelectedModel() {
	if m.modelPicker == nil || m.modelPicker.loading || m.modelPicker.err != "" {
		return
	}
	if len(m.modelPicker.models) == 0 {
		return
	}
	if m.setModel == nil {
		m.modelPicker.err = "model changes are not available"
		m.status = "models:error"
		m.resize()
		return
	}

	selected := m.modelPicker.models[m.clampedModelCursor()]
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
	if m.modelPicker == nil || len(m.modelPicker.models) == 0 || m.modelPicker.cursor < 0 {
		return 0
	}
	if m.modelPicker.cursor >= len(m.modelPicker.models) {
		return len(m.modelPicker.models) - 1
	}
	return m.modelPicker.cursor
}

func (m model) modelPickerHeight() int {
	if m.modelPicker == nil {
		return 0
	}
	if m.modelPicker.loading || m.modelPicker.err != "" || len(m.modelPicker.models) == 0 {
		return 4
	}
	return min(maxModelPickerRows, len(m.modelPicker.models)) + 3
}

func (m model) renderModelPicker() (string, bool) {
	if m.modelPicker == nil {
		return "", false
	}

	width := max(20, m.width-4)
	lines := []string{m.styles.selectorTitle.Render("Models")}
	switch {
	case m.modelPicker.loading:
		lines = append(lines, m.styles.muted.Render("Loading models..."))
	case m.modelPicker.err != "":
		lines = append(lines, m.styles.error.Render(m.modelPicker.err))
		lines = append(lines, m.styles.muted.Render("Esc closes"))
	case len(m.modelPicker.models) == 0:
		lines = append(lines, m.styles.muted.Render("No models available"))
	default:
		lines = append(lines, m.renderModelRows(width-4)...)
		lines = append(lines, m.styles.muted.Render("Enter selects, Esc closes"))
	}

	return m.styles.selector.Width(width).Render(strings.Join(lines, "\n")), true
}

func (m model) renderModelRows(width int) []string {
	picker := m.modelPicker
	if picker == nil {
		return nil
	}

	start, end := modelWindow(picker.cursor, len(picker.models), maxModelPickerRows)
	rows := make([]string, 0, end-start)
	current := currentString(m.currentModel)
	for i := start; i < end; i++ {
		model := picker.models[i]
		line := formatModelRow(model, current, width)
		if i == picker.cursor {
			line = m.styles.selectorActive.Render(line)
		}
		rows = append(rows, line)
	}
	return rows
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
	name := modelSelectionID(model)
	if name == "" {
		name = "(unnamed)"
	}

	parts := []string{name}
	if model.Provider != "" {
		parts = append(parts, model.Provider)
	}
	if model.ProviderModel != "" && model.ProviderModel != name {
		parts = append(parts, model.ProviderModel)
	}
	if model.MaxInputTokens > 0 {
		parts = append(parts, fmt.Sprintf("ctx:%d", model.MaxInputTokens))
	}
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

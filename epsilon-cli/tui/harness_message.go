package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type harnessMessage struct {
	width      int
	density    densityMode
	label      lipgloss.Style
	muted      lipgloss.Style
	block      lipgloss.Style
	agentBlock lipgloss.Style
}

func newHarnessMessage(width int, density densityMode, label lipgloss.Style,
	muted lipgloss.Style) harnessMessage {
	return harnessMessage{
		width:   max(20, width),
		density: density,
		label:   label,
		muted:   muted,
		block: lipgloss.NewStyle().
			Background(tuiBackground).
			Foreground(tuiInk),
		agentBlock: lipgloss.NewStyle().
			Background(tuiSurface).
			Foreground(tuiInk).
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(tuiAccentAgent).
			PaddingLeft(1),
	}
}

func (h harnessMessage) Render(text string) string {
	message := strings.TrimSpace(text)
	if message == "" {
		return ""
	}

	return h.block.Width(h.width).Render(message)
}

func (h harnessMessage) RenderAgent(text string) string {
	message := strings.TrimSpace(text)
	if message != "" {
		message = newMarkdownRenderer(max(20, h.width-2)).Render(message)
	}
	return h.renderLabeled("Agent", message)
}

func (h harnessMessage) RenderSpinner(text string) string {
	return h.renderLabeled("Agent", strings.TrimSpace(text))
}

func (h harnessMessage) renderLabeled(label string, text string) string {
	renderedLabel := h.label.Render(label)
	if h.density == densityCompact {
		return h.block.Width(h.width).Render(
			renderedLabel + " " + h.muted.Render("-") + " " + text)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		renderedLabel,
		h.agentBlock.Width(h.width).Render(text),
	)
}

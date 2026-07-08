package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/types"
)

const (
	contextPanelMinWidth = 20
)

type contextWidget struct {
	summary contextwindow.Summary
	model   *types.ModelInfo
	usage   types.Usage
	width   int
	height  int
}

func newContextWidget(summary contextwindow.Summary, width int, model *types.ModelInfo,
	usage types.Usage) contextWidget {
	return contextWidget{
		summary: summary,
		model:   model,
		usage:   usage,
		width:   max(20, width),
	}
}

func newContextPanel(summary contextwindow.Summary, width int, height int,
	model *types.ModelInfo, usage types.Usage) contextWidget {
	return contextWidget{
		summary: summary,
		model:   model,
		usage:   usage,
		width:   max(contextPanelMinWidth, width),
		height:  max(3, height),
	}
}

func (w contextWidget) View() string {
	label := fmt.Sprintf("context %.1f%%", w.summary.PercentUsed*100)
	details := fmt.Sprintf("%d/%d tokens", w.summary.UsedTokens, w.summary.MaxTokens)
	until := fmt.Sprintf("%d until compact", w.summary.TokensUntilCompact)
	if w.summary.NeedsCompaction {
		until = "compaction due"
	}
	parts := []string{label, details, until}
	if cost := sessionCostLine(w.model, w.usage); cost != "" {
		parts = append(parts, cost)
	}
	line := strings.Join(parts, " | ")

	style := lipgloss.NewStyle().
		Foreground(tuiInkStrong).
		Background(contextColor(w.summary)).
		Padding(0, 1).
		Width(max(0, w.width-2))
	return style.Render(line)
}

func (w contextWidget) Panel() string {
	width := max(contextPanelMinWidth, w.width)
	innerWidth := max(20, width-4)
	headingStyle := lipgloss.NewStyle().
		Background(tuiBackground).
		Bold(true).
		Foreground(tuiInkStrong)
	subheadingStyle := lipgloss.NewStyle().
		Background(tuiBackground).
		Foreground(tuiAccentAgent).
		Bold(true)

	titleRight := fmt.Sprintf("%d / %d tokens", w.summary.UsedTokens, w.summary.MaxTokens)
	titleGap := max(1, innerWidth-lipgloss.Width("Context")-lipgloss.Width(titleRight))
	lines := []string{
		headingStyle.Render("Context") +
			strings.Repeat(" ", titleGap) +
			lipgloss.NewStyle().Background(tuiBackground).Foreground(tuiMuted).Render(titleRight),
		contextBar(w.summary.PercentUsed, innerWidth),
		contextSummaryLine(w.summary, innerWidth),
	}

	if w.model != nil {
		modelLine := strings.Join(modelLines(*w.model), " | ")
		lines = append(lines, "", subheadingStyle.Render("Model"))
		lines = append(lines, wrapLine(modelLine, innerWidth)...)
	}

	if cost := sessionCostLine(w.model, w.usage); cost != "" {
		lines = append(lines, "", subheadingStyle.Render("Session"))
		lines = append(lines, wrapLine(cost, innerWidth)...)
	}

	lines = append(lines,
		"",
		subheadingStyle.Render("Breakdown"),
	)

	for _, bucket := range w.summary.Buckets {
		lines = append(lines, bucketLine(bucket, innerWidth))
	}

	if len(w.summary.Segments) > 0 {
		lines = append(lines, "")
		lines = append(lines, subheadingStyle.Render("Recent"))
		for _, segment := range recentSegments(w.summary.Segments, recentSegmentCount(innerWidth)) {
			lines = append(lines, segmentLine(segment, innerWidth))
		}
	}

	body := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(max(1, width-2)).
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(tuiLine).
		Background(tuiBackground).
		Foreground(tuiInk).
		Padding(0, 1).
		Render(body)
}

func contextSummaryLine(summary contextwindow.Summary, width int) string {
	until := fmt.Sprintf("%d until compact", summary.TokensUntilCompact)
	if summary.NeedsCompaction {
		until = "compaction due"
	}
	parts := []string{
		fmt.Sprintf("%.1f%% used", summary.PercentUsed*100),
		fmt.Sprintf("%d left", summary.RemainingTokens),
		until,
	}
	return truncate(strings.Join(parts, " | "), width)
}

func modelLines(model types.ModelInfo) []string {
	lines := []string{
		"id: " + model.ID,
	}
	if model.Provider != "" {
		lines = append(lines, "provider: "+model.Provider)
	}
	if model.ProviderModel != "" && model.ProviderModel != model.ID {
		lines = append(lines, "model: "+model.ProviderModel)
	}
	if model.MaxInputTokens > 0 {
		lines = append(lines, fmt.Sprintf("context: %d", model.MaxInputTokens))
	}
	if model.MaxOutputTokens > 0 {
		lines = append(lines, fmt.Sprintf("output: %d", model.MaxOutputTokens))
	}
	if price := pricingLine(model.Pricing.InputCostPer1KTokens,
		model.Pricing.OutputCostPer1KTokens); price != "" {
		lines = append(lines, price)
	}
	return lines
}

func pricingLine(input float64, output float64) string {
	if input == 0 && output == 0 {
		return ""
	}
	return fmt.Sprintf("price/1M: $%.4g in / $%.4g out", input*1000, output*1000)
}

func sessionCostLine(model *types.ModelInfo, usage types.Usage) string {
	cost := sessionCost(model, usage)
	if cost <= 0 {
		return ""
	}
	return "cost: " + formatCost(cost)
}

func sessionCost(model *types.ModelInfo, usage types.Usage) float64 {
	if model == nil {
		return 0
	}
	inputCost := tokenCost(model.Pricing.InputCostPerToken,
		model.Pricing.InputCostPer1KTokens)
	outputCost := tokenCost(model.Pricing.OutputCostPerToken,
		model.Pricing.OutputCostPer1KTokens)
	if inputCost == 0 && outputCost == 0 {
		return 0
	}
	return float64(usage.InputTokens)*inputCost + float64(usage.OutputTokens)*outputCost
}

func tokenCost(perToken float64, per1K float64) float64 {
	if perToken > 0 {
		return perToken
	}
	if per1K > 0 {
		return per1K / 1000
	}
	return 0
}

func formatCost(cost float64) string {
	switch {
	case cost >= 1:
		return fmt.Sprintf("$%.2f", cost)
	case cost >= 0.01:
		return fmt.Sprintf("$%.4f", cost)
	case cost >= 0.0001:
		return fmt.Sprintf("$%.6f", cost)
	default:
		return fmt.Sprintf("$%.8f", cost)
	}
}

func bucketLine(bucket contextwindow.Bucket, width int) string {
	labelWidth := min(16, max(10, width/2))
	label := truncate(bucket.Label, labelWidth)
	share := fmt.Sprintf("%4.0f%%", bucket.ShareOfUsed*100)
	tokens := fmt.Sprintf("%d", bucket.Tokens)
	return fmt.Sprintf("%-*s %s %s", labelWidth, label, share, tokens)
}

func segmentLine(segment contextwindow.Segment, width int) string {
	tokens := fmt.Sprintf("%d", segment.Tokens)
	labelWidth := max(8, width-len(tokens)-1)
	return truncate(segment.Label, labelWidth) + " " + tokens
}

func recentSegments(segments []contextwindow.Segment, count int) []contextwindow.Segment {
	if len(segments) <= count {
		return segments
	}
	return segments[len(segments)-count:]
}

func recentSegmentCount(width int) int {
	if width < 48 {
		return 3
	}
	return 5
}

func wrapLine(text string, width int) []string {
	if width <= 0 {
		return nil
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func contextBar(percent float64, width int) string {
	width = max(8, width)
	fill := min(int(percent*float64(width)), width)
	return lipgloss.NewStyle().Foreground(tuiAccentAgent).Render(strings.Repeat("=", fill)) +
		lipgloss.NewStyle().Foreground(tuiFaint).Render(strings.Repeat("-", width-fill))
}

func contextColor(summary contextwindow.Summary) color.Color {
	switch {
	case summary.NeedsCompaction:
		return tuiStatusDanger
	case summary.PercentUsed >= summary.CompactionThreshold*0.8:
		return tuiStatusWarn
	default:
		return tuiSurface2
	}
}

func truncate(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "."
}

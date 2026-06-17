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
	contextPanelMinWidth = 32
	contextPanelMaxWidth = 44
)

type contextWidget struct {
	summary contextwindow.Summary
	model   *types.ModelInfo
	width   int
	height  int
}

func newContextWidget(summary contextwindow.Summary, width int, model *types.ModelInfo) contextWidget {
	return contextWidget{
		summary: summary,
		model:   model,
		width:   max(20, width),
	}
}

func newContextPanel(summary contextwindow.Summary, width int, height int,
	model *types.ModelInfo) contextWidget {
	return contextWidget{
		summary: summary,
		model:   model,
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
	line := label + " | " + details + " | " + until

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Background(contextColor(w.summary)).
		Padding(0, 1).
		Width(max(0, w.width-2))
	return style.Render(line)
}

func (w contextWidget) Panel() string {
	width := max(contextPanelMinWidth, w.width)
	innerWidth := max(20, width-4)
	headingStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Bold(true).
		Foreground(lipgloss.Color("250"))
	subheadingStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Foreground(lipgloss.Color("146")).
		Bold(true)
	lines := []string{
		headingStyle.Render("Context"),
		fmt.Sprintf("%.1f%% used", w.summary.PercentUsed*100),
		fmt.Sprintf("%d / %d tokens", w.summary.UsedTokens, w.summary.MaxTokens),
		fmt.Sprintf("%d tokens left", w.summary.RemainingTokens),
		fmt.Sprintf("%d until compact", w.summary.TokensUntilCompact),
		contextBar(w.summary.PercentUsed, innerWidth),
	}

	if w.model != nil {
		lines = append(lines, "", subheadingStyle.Render("Model"))
		lines = append(lines, modelLines(*w.model)...)
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
		lines = append(lines,
			subheadingStyle.Render("Recent"))
		for _, segment := range recentSegments(w.summary.Segments, 5) {
			lines = append(lines, segmentLine(segment, innerWidth))
		}
	}

	body := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(max(1, width-3)).
		Height(w.height).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("238")).
		Background(lipgloss.Color("234")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(body)
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
	return fmt.Sprintf("price/1k: $%.4g in / $%.4g out", input, output)
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

func contextBar(percent float64, width int) string {
	width = max(8, width)
	fill := min(int(percent*float64(width)), width)
	return lipgloss.NewStyle().Foreground(lipgloss.Color("146")).Render(strings.Repeat("=", fill)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("-", width-fill))
}

func contextColor(summary contextwindow.Summary) color.Color {
	switch {
	case summary.NeedsCompaction:
		return lipgloss.Color("52")
	case summary.PercentUsed >= summary.CompactionThreshold*0.8:
		return lipgloss.Color("58")
	default:
		return lipgloss.Color("235")
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

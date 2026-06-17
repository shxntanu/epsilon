package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

const tuiBackgroundANSI = "\x1b[48;2;0;0;0m"

var (
	tuiBackground      = lipgloss.Color("#000000")
	tuiBackgroundColor = color.RGBA{R: 0, G: 0, B: 0, A: 255}
)

func paintScreenBackground(content string, width int, height int) string {
	width = max(0, width)
	height = max(0, height)
	if width == 0 || height == 0 {
		return content
	}

	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	painted := make([]string, len(lines))
	for i, line := range lines {
		line = keepBackgroundAfterReset(line)
		if visible := lipgloss.Width(line); visible < width {
			line += strings.Repeat(" ", width-visible)
		}
		painted[i] = tuiBackgroundANSI + line
	}
	return strings.Join(painted, "\n")
}

func keepBackgroundAfterReset(text string) string {
	text = strings.ReplaceAll(text, "\x1b[0m", "\x1b[0m"+tuiBackgroundANSI)
	text = strings.ReplaceAll(text, "\x1b[m", "\x1b[m"+tuiBackgroundANSI)
	text = strings.ReplaceAll(text, "\x1b[49m", tuiBackgroundANSI)
	return text
}

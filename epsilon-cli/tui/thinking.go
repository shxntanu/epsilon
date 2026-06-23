package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type thinkingTickMsg time.Time

type RGB struct {
	R uint8
	G uint8
	B uint8
}

type Thinking struct {
	Text string

	BaseColor      RGB
	HighlightColor RGB

	Frame int
	Speed float64 // chars/sec

	lastWidth int
}

func NewThinking(text string) Thinking {
	return Thinking{
		Text: text,
		BaseColor: RGB{
			R: 21,
			G: 151,
			B: 187,
		},
		HighlightColor: RGB{
			R: 143,
			G: 214,
			B: 225,
		},
		Speed: 8,
	}
}

func thinkingTick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
		return thinkingTickMsg(t)
	})
}

func (m Thinking) Update(msg tea.Msg) (Thinking, tea.Cmd) {
	switch msg.(type) {
	case thinkingTickMsg:
		m.Frame++
		return m, thinkingTick()
	}

	return m, nil
}

func (m Thinking) View() string {
	runes := []rune(m.Text)

	if len(runes) == 0 {
		return ""
	}

	width := len(runes)

	// Highlight travels beyond the ends so it fades in/out naturally.
	position :=
		(float64(m.Frame) / 30.0) * m.Speed

	position = math.Mod(position, float64(width)+12) - 6

	var out strings.Builder

	for i, r := range runes {
		dist := math.Abs(float64(i) - position)

		// Gaussian falloff.
		//
		// Smaller = tighter highlight.
		// Larger = softer shimmer.
		intensity := math.Exp(-(dist * dist) / 4.5)

		c := lerpColor(
			m.BaseColor,
			m.HighlightColor,
			intensity,
		)

		out.WriteString(ansiRGB(c))
		out.WriteString(string(r))
	}

	out.WriteString("\033[0m")

	return out.String()
}

func ansiRGB(c RGB) string {
	return fmt.Sprintf(
		"\033[38;2;%d;%d;%dm",
		c.R,
		c.G,
		c.B,
	)
}

func lerpColor(a, b RGB, t float64) RGB {
	return RGB{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
	}
}

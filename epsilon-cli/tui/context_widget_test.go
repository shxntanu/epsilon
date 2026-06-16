package tui

import (
	"strings"
	"testing"

	"github.com/shxntanu/epsilon/core/contextwindow"
)

func TestContextWidgetView(t *testing.T) {
	view := newContextWidget(contextwindow.Summary{
		MaxTokens:          100,
		UsedTokens:         25,
		RemainingTokens:    75,
		PercentUsed:        0.25,
		TokensUntilCompact: 60,
	}, 80).View()

	for _, want := range []string{"context 25.0%", "25/100 tokens", "60 until compact"} {
		if !strings.Contains(view, want) {
			t.Fatalf("context widget missing %q in %q", want, view)
		}
	}
}

func TestContextPanelView(t *testing.T) {
	view := newContextPanel(contextwindow.Summary{
		MaxTokens:          100,
		UsedTokens:         50,
		RemainingTokens:    50,
		PercentUsed:        0.5,
		TokensUntilCompact: 35,
		Buckets: []contextwindow.Bucket{
			{Label: "User messages", Tokens: 20, ShareOfUsed: 0.4},
			{Label: "Tool calls", Tokens: 30, ShareOfUsed: 0.6},
		},
		Segments: []contextwindow.Segment{
			{Label: "user", Tokens: 20},
			{Label: "tool call: read_file", Tokens: 30},
		},
	}, 36, 12).Panel()

	for _, want := range []string{"Context", "Breakdown", "User messages", "Tool calls", "Recent"} {
		if !strings.Contains(view, want) {
			t.Fatalf("context panel missing %q in %q", want, view)
		}
	}
}

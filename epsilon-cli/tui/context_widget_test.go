package tui

import (
	"strings"
	"testing"

	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/types"
)

func TestContextWidgetView(t *testing.T) {
	view := newContextWidget(contextwindow.Summary{
		MaxTokens:          100,
		UsedTokens:         25,
		RemainingTokens:    75,
		PercentUsed:        0.25,
		TokensUntilCompact: 60,
	}, 80, nil, types.Usage{}).View()

	for _, want := range []string{"context 25.0%", "25/100 tokens", "60 until compact"} {
		if !strings.Contains(view, want) {
			t.Fatalf("context widget missing %q in %q", want, view)
		}
	}
}

func TestContextPanelView(t *testing.T) {
	model := &types.ModelInfo{
		ID:              "gpt-4o",
		Provider:        "openai",
		ProviderModel:   "openai/gpt-4o",
		MaxInputTokens:  128000,
		MaxOutputTokens: 16384,
		Pricing: types.ModelPricing{
			InputCostPer1KTokens:  0.0025,
			OutputCostPer1KTokens: 0.01,
		},
	}
	usage := types.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}
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
	}, 36, 12, model, usage).Panel()

	for _, want := range []string{"Context", "Model", "gpt-4o", "128000", "Session", "cost: $0.000750", "Breakdown", "User messages", "Tool calls", "Recent"} {
		if !strings.Contains(view, want) {
			t.Fatalf("context panel missing %q in %q", want, view)
		}
	}
}

func TestContextWidgetShowsSessionCost(t *testing.T) {
	model := &types.ModelInfo{
		ID: "gpt-4o",
		Pricing: types.ModelPricing{
			InputCostPerToken:  0.0000025,
			OutputCostPerToken: 0.00001,
		},
	}
	view := newContextWidget(contextwindow.Summary{
		MaxTokens:          100,
		UsedTokens:         25,
		RemainingTokens:    75,
		PercentUsed:        0.25,
		TokensUntilCompact: 60,
	}, 80, model, types.Usage{InputTokens: 100, OutputTokens: 50}).View()

	if !strings.Contains(view, "cost: $0.000750") {
		t.Fatalf("context widget missing cost in %q", view)
	}
}

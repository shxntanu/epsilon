package tui

import (
	"testing"

	"github.com/shxntanu/epsilon/core/slash"
	"github.com/shxntanu/epsilon/core/types"
)

func TestSlashSelectorQuery(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantQuery string
		wantOK    bool
	}{
		{
			name:      "slash prefix",
			input:     "/sta",
			wantQuery: "sta",
			wantOK:    true,
		},
		{
			name:      "bare slash",
			input:     "/",
			wantQuery: "",
			wantOK:    true,
		},
		{
			name:  "escaped slash",
			input: "//help",
		},
		{
			name:  "command with args",
			input: "/events off",
		},
		{
			name:  "plain message",
			input: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotOK := slashSelectorQuery(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotQuery != tt.wantQuery {
				t.Fatalf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}

func TestDensityModeFromSlash(t *testing.T) {
	if got := densityModeFromSlash(slash.DensityCompact); got != densityCompact {
		t.Fatalf("compact density = %v, want compact", got)
	}
	if got := slashDensity(densityComfortable); got != slash.DensityComfortable {
		t.Fatalf("comfortable density = %v, want comfortable", got)
	}
}

func TestModelPickerSelectsCurrentModel(t *testing.T) {
	models := []types.ModelInfo{
		{ID: "gpt-4o"},
		{ID: "gpt-5.4", Provider: "openai", ProviderModel: "openai/gpt-5.4"},
	}

	if got := selectedModelCursor(models, "openai/gpt-5.4"); got != 1 {
		t.Fatalf("cursor = %d, want current model index", got)
	}
	if got := modelSelectionID(models[1]); got != "gpt-5.4" {
		t.Fatalf("selection id = %q, want gpt-5.4", got)
	}
}

func TestModelPickerFiltersFuzzily(t *testing.T) {
	picker := modelPicker{
		models: []types.ModelInfo{
			{ID: "claude-sonnet-4"},
			{ID: "gpt-4o"},
			{ID: "gpt-5.4"},
		},
		query: "g54",
	}
	picker.filter("")

	visible := picker.visibleModels()
	if len(visible) != 1 || visible[0].ID != "gpt-5.4" {
		t.Fatalf("visible = %#v, want only gpt-5.4", visible)
	}
}

func TestFormatModelRowOmitsMetadata(t *testing.T) {
	row := formatModelRow(types.ModelInfo{
		ID:             "gpt-5.4",
		Name:           "GPT 5.4",
		Provider:       "openai",
		ProviderModel:  "openai/gpt-5.4",
		MaxInputTokens: 128000,
	}, "", 80)

	if row != "GPT 5.4" {
		t.Fatalf("row = %q, want display name only", row)
	}
}

func TestModelWindowKeepsCursorVisible(t *testing.T) {
	start, end := modelWindow(9, 12, 5)
	if start != 7 || end != 12 {
		t.Fatalf("window = %d:%d, want 7:12", start, end)
	}
}

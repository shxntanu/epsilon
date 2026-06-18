package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core/events"
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

func TestRenderToolEntryHidesDetailsUntilExpanded(t *testing.T) {
	m := model{
		width:   80,
		density: densityComfortable,
		styles: styles{
			tool:      lipgloss.NewStyle(),
			error:     lipgloss.NewStyle(),
			muted:     lipgloss.NewStyle(),
			toolBlock: lipgloss.NewStyle(),
		},
	}
	entry := transcriptEntry{
		kind:       transcriptTool,
		text:       "Used read_file",
		toolName:   "read_file",
		toolInput:  `{"path":"README.md"}`,
		toolResult: "project docs",
		toolMeta:   map[string]string{"path": "README.md"},
	}

	collapsed := m.renderToolEntry(entry)
	if strings.Contains(collapsed, "project docs") || strings.Contains(collapsed, `"path"`) {
		t.Fatalf("collapsed tool entry leaked details:\n%s", collapsed)
	}

	m.showEvents = true
	expanded := m.renderToolEntry(entry)
	for _, want := range []string{"Used read_file", `{"path":"README.md"}`, "project docs"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded tool entry missing %q:\n%s", want, expanded)
		}
	}
}

func TestRenderToolGroupHidesNestedDetailsUntilExpanded(t *testing.T) {
	m := model{
		width:   80,
		density: densityCompact,
		styles: styles{
			tool:      lipgloss.NewStyle(),
			error:     lipgloss.NewStyle(),
			muted:     lipgloss.NewStyle(),
			toolBlock: lipgloss.NewStyle(),
		},
	}
	entry := transcriptEntry{
		kind: transcriptToolGroup,
		text: "Explored 2 items: read_file, ripgrep",
		tools: []transcriptEntry{
			{
				kind:       transcriptTool,
				text:       "Used read_file",
				toolName:   "read_file",
				toolInput:  `{"path":"README.md"}`,
				toolResult: "project docs",
			},
			{
				kind:       transcriptTool,
				text:       "Used ripgrep",
				toolName:   "ripgrep",
				toolInput:  `{"pattern":"TODO"}`,
				toolResult: "No matches found.",
			},
		},
	}

	collapsed := m.renderToolGroupEntry(entry)
	if strings.Contains(collapsed, "project docs") || strings.Contains(collapsed, "No matches found") {
		t.Fatalf("collapsed tool group leaked details:\n%s", collapsed)
	}

	m.showEvents = true
	expanded := m.renderToolGroupEntry(entry)
	for _, want := range []string{"Explored 2 items", "Used read_file", `{"path":"README.md"}`, "No matches found."} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded tool group missing %q:\n%s", want, expanded)
		}
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

func TestSessionPickerFiltersFuzzily(t *testing.T) {
	picker := sessionPicker{
		sessions: []events.SessionInfo{
			{ID: "session_alpha"},
			{ID: "session_beta"},
			{ID: "worktree_fix"},
		},
		query: "wfx",
	}
	picker.filter("")

	visible := picker.visibleSessions()
	if len(visible) != 1 || visible[0].ID != "worktree_fix" {
		t.Fatalf("visible = %#v, want only worktree_fix", visible)
	}
}

func TestFormatSessionRowMarksCurrent(t *testing.T) {
	row := formatSessionRow(events.SessionInfo{ID: "session_123"}, "session_123", 80)
	if row != "session_123  current" {
		t.Fatalf("row = %q, want current marker", row)
	}
}

func TestModelWindowKeepsCursorVisible(t *testing.T) {
	start, end := modelWindow(9, 12, 5)
	if start != 7 || end != 12 {
		t.Fatalf("window = %d:%d, want 7:12", start, end)
	}
}

package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/skills"
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

func TestRenderSlashSelectorUsesCommandPaletteTreatment(t *testing.T) {
	composer := newComposer()
	composer.SetValue("/")
	m := model{
		layoutState: layoutState{width: 100},
		inputState: inputState{
			composer: composer,
			slash:    slash.NewDefaultRegistry(),
		},
		visualState: visualState{
			styles: selectorStyles(styles{
				muted: lipgloss.NewStyle(),
			}),
		},
	}

	rendered, ok := m.renderSlashSelector()
	if !ok {
		t.Fatal("selector did not render")
	}
	plain := plainANSI(rendered)
	for _, want := range []string{"✦ Slash commands", "commands", "▸ /clear", "tab completes"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("selector missing %q:\n%s", want, plain)
		}
	}
}

func TestComposerViewShowsCommandMode(t *testing.T) {
	composer := newComposer()
	composer.SetWidth(80)

	view := plainANSI(composer.View(true, false, false, "", 1, "test session"))
	for _, want := range []string{"command", "tab completes"} {
		if !strings.Contains(view, want) {
			t.Fatalf("composer command mode missing %q:\n%s", want, view)
		}
	}
}

func TestSkillSelectorQuery(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantQuery string
		wantOK    bool
	}{
		{name: "skill prefix", input: "!imp", wantQuery: "imp", wantOK: true},
		{name: "bare skill prefix", input: "!", wantQuery: "", wantOK: true},
		{name: "escaped bang", input: "!!literal"},
		{name: "skill with args", input: "!imp now"},
		{name: "plain message", input: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotOK := skillSelectorQuery(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotQuery != tt.wantQuery {
				t.Fatalf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}

func TestRenderSkillSelectorUsesToolPaletteTreatment(t *testing.T) {
	composer := newComposer()
	composer.SetValue("!")
	m := model{
		layoutState: layoutState{width: 100},
		inputState: inputState{
			composer: composer,
		},
		providerState: providerState{
			listSkills: func() []skills.Skill {
				return []skills.Skill{{
					Name:        "impeccable",
					Description: "Build polished frontend interfaces.",
				}}
			},
		},
		visualState: visualState{
			styles: selectorStyles(styles{
				muted: lipgloss.NewStyle(),
			}),
		},
	}

	rendered, ok := m.renderSkillSelector()
	if !ok {
		t.Fatal("selector did not render")
	}
	plain := plainANSI(rendered)
	for _, want := range []string{"✦ Skills", "skills", "▸ !impeccable", "tab completes"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("selector missing %q:\n%s", want, plain)
		}
	}
}

func TestComposerViewHighlightsSkillPrefixInline(t *testing.T) {
	composer := newComposer()
	composer.SetWidth(80)
	composer.SetValue("!impeccable improve the composer")

	view := plainANSI(composer.View(false, false, false, "impeccable", 1, "test session"))
	for _, want := range []string{"$impeccable", "improve the composer"} {
		if !strings.Contains(view, want) {
			t.Fatalf("composer skill prefix missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "!impeccable") {
		t.Fatalf("composer should render highlighted skill prefix, got:\n%s", view)
	}
}

func TestCompleteSkillSelectionInsertsRequiredSkillPrefix(t *testing.T) {
	composer := newComposer()
	composer.SetValue("!imp")
	m := model{
		layoutState: layoutState{width: 100},
		inputState: inputState{
			composer: composer,
		},
		providerState: providerState{
			listSkills: func() []skills.Skill {
				return []skills.Skill{{Name: "impeccable"}}
			},
		},
		visualState: visualState{
			styles: selectorStyles(styles{
				muted: lipgloss.NewStyle(),
			}),
		},
	}

	if !m.completeSkillSelection() {
		t.Fatal("completeSkillSelection returned false")
	}
	if got := m.composer.Value(); got != "!impeccable " {
		t.Fatalf("composer value = %q, want required skill prefix", got)
	}
}

func TestFileSelectorQuery(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantQuery string
		wantOK    bool
	}{
		{name: "file prefix", input: "@tui", wantQuery: "tui", wantOK: true},
		{name: "bare file prefix", input: "read @", wantQuery: "", wantOK: true},
		{name: "file in prompt", input: "explain @composer", wantQuery: "composer", wantOK: true},
		{name: "escaped at", input: "@@literal"},
		{name: "closed token", input: "read @composer now"},
		{name: "trailing space closes", input: "read @composer "},
		{name: "plain message", input: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotOK := fileSelectorQuery(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotQuery != tt.wantQuery {
				t.Fatalf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}

func TestRenderFileSelectorUsesSelectorTreatment(t *testing.T) {
	composer := newComposer()
	composer.SetValue("@comp")
	m := model{
		layoutState: layoutState{width: 100},
		inputState: inputState{
			composer:  composer,
			filePaths: []string{"epsilon-cli/tui/composer.go", "epsilon-cli/tui/tui.go"},
		},
		visualState: visualState{
			styles: selectorStyles(styles{
				muted: lipgloss.NewStyle(),
			}),
		},
	}

	rendered, ok := m.renderFileSelector()
	if !ok {
		t.Fatal("selector did not render")
	}
	plain := plainANSI(rendered)
	for _, want := range []string{"✦ Files", "matches", "▸ epsilon-cli/tui/composer.go", "enter pastes"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("selector missing %q:\n%s", want, plain)
		}
	}
}

func TestCompleteFileSelectionPastesRelativePath(t *testing.T) {
	composer := newComposer()
	composer.SetValue("explain @comp")
	m := model{
		layoutState: layoutState{width: 100},
		inputState: inputState{
			composer:  composer,
			filePaths: []string{"epsilon-cli/tui/composer.go"},
		},
		visualState: visualState{
			styles: selectorStyles(styles{
				muted: lipgloss.NewStyle(),
			}),
		},
	}

	if !m.completeFileSelection() {
		t.Fatal("completeFileSelection returned false")
	}
	if got := m.composer.Value(); got != "explain epsilon-cli/tui/composer.go " {
		t.Fatalf("composer value = %q, want selected relative path pasted", got)
	}
}

func TestCompleteFileSelectionPreservesTextAfterCursor(t *testing.T) {
	composer := newComposer()
	composer.SetValue("explain @comp please")
	for range len(" please") {
		composer, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	m := model{
		layoutState: layoutState{width: 100},
		inputState: inputState{
			composer:  composer,
			filePaths: []string{"epsilon-cli/tui/composer.go"},
		},
		visualState: visualState{
			styles: selectorStyles(styles{
				muted: lipgloss.NewStyle(),
			}),
		},
	}

	if !m.completeFileSelection() {
		t.Fatal("completeFileSelection returned false")
	}
	if got := m.composer.Value(); got != "explain epsilon-cli/tui/composer.go please" {
		t.Fatalf("composer value = %q, want selected path before trailing text", got)
	}
}

func TestShiftEnterInComposerInsertsNewline(t *testing.T) {
	composer := newComposer()
	composer.SetValue("first line")
	m := model{
		inputState: inputState{
			composer: composer,
		},
		visualState: visualState{
			styles: selectorStyles(styles{
				muted: lipgloss.NewStyle(),
			}),
		},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	got := updated.(model).composer.Value()
	if got != "first line\n" {
		t.Fatalf("composer value = %q, want newline appended", got)
	}
	view := plainANSI(updated.(model).composer.View(false, false, false, "", 1, "test session"))
	if strings.Contains(view, "\\") {
		t.Fatalf("composer view rendered literal backslash after newline:\n%s", view)
	}
}

func TestPromptHistoryCyclesAndRestoresDraft(t *testing.T) {
	composer := newComposer()
	composer.SetValue("current draft")
	m := model{
		inputState: inputState{
			composer:      composer,
			promptHistory: []string{"first prompt", "second prompt", "third prompt"},
		},
	}

	if !m.recallPromptHistory(-1) {
		t.Fatal("recallPromptHistory up returned false, want true")
	}
	if got := m.composer.Value(); got != "third prompt" {
		t.Fatalf("composer value = %q, want newest prompt", got)
	}
	if !m.recallPromptHistory(-1) {
		t.Fatal("second history up returned false, want true")
	}
	if got := m.composer.Value(); got != "second prompt" {
		t.Fatalf("composer value = %q, want previous prompt", got)
	}
	if !m.recallPromptHistory(1) {
		t.Fatal("history down returned false, want true")
	}
	if got := m.composer.Value(); got != "third prompt" {
		t.Fatalf("composer value = %q, want newer prompt", got)
	}
	if !m.recallPromptHistory(1) {
		t.Fatal("final history down returned false, want true")
	}
	if got := m.composer.Value(); got != "current draft" {
		t.Fatalf("composer value = %q, want restored draft", got)
	}
}

func TestPromptHistoryRestoresEmptyDraft(t *testing.T) {
	composer := newComposer()
	m := model{
		inputState: inputState{
			composer:      composer,
			promptHistory: []string{"previous prompt"},
		},
	}

	if !m.recallPromptHistory(-1) {
		t.Fatal("history up returned false, want true")
	}
	if got := m.composer.Value(); got != "previous prompt" {
		t.Fatalf("composer value = %q, want previous prompt", got)
	}
	if !m.recallPromptHistory(1) {
		t.Fatal("history down returned false, want true")
	}
	if got := m.composer.Value(); got != "" {
		t.Fatalf("composer value = %q, want empty draft restored", got)
	}
}

func TestUpAndDownKeysNavigatePromptHistory(t *testing.T) {
	composer := newComposer()
	m := model{
		inputState: inputState{
			composer:      composer,
			promptHistory: []string{"summarize this repo", "explain the tests"},
		},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	m = updated.(model)
	if got := m.composer.Value(); got != "explain the tests" {
		t.Fatalf("composer value = %q, want newest prompt", got)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	m = updated.(model)
	if got := m.composer.Value(); got != "summarize this repo" {
		t.Fatalf("composer value = %q, want older prompt", got)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	m = updated.(model)
	if got := m.composer.Value(); got != "explain the tests" {
		t.Fatalf("composer value = %q, want newer prompt", got)
	}
}

func TestUpAndDownKeysNavigateCursorInMultilineComposer(t *testing.T) {
	composer := newComposer()
	composer.SetValue("first line\nsecond line")
	m := model{
		inputState: inputState{
			composer:      composer,
			promptHistory: []string{"previous prompt"},
		},
	}
	if m.composer.CursorLine() != 1 {
		t.Fatalf("cursor line = %d, want final line", m.composer.CursorLine())
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(model)
	if got := m.composer.Value(); got != "first line\nsecond line" {
		t.Fatalf("composer value = %q, want multiline prompt preserved", got)
	}
	if m.composer.CursorLine() != 0 {
		t.Fatalf("cursor line = %d, want previous line", m.composer.CursorLine())
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(model)
	if m.composer.CursorLine() != 1 {
		t.Fatalf("cursor line = %d, want next line", m.composer.CursorLine())
	}
	_ = cmd
}

func TestComposerCommandArrowsMoveToLineEdges(t *testing.T) {
	composer := newComposer()
	composer.SetValue("first line\nsecond line")
	if got := composer.CursorColumn(); got != len("second line") {
		t.Fatalf("cursor column = %d, want end of second line", got)
	}

	var cmd tea.Cmd
	composer, cmd = composer.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModMeta})
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if got := composer.CursorColumn(); got != 0 {
		t.Fatalf("cursor column = %d, want start of line", got)
	}

	composer, cmd = composer.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModMeta})
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if got := composer.CursorColumn(); got != len("second line") {
		t.Fatalf("cursor column = %d, want end of line", got)
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
		layoutState: layoutState{
			width:   80,
			density: densityComfortable,
		},
		visualState: visualState{
			styles: styles{
				tool:      lipgloss.NewStyle(),
				error:     lipgloss.NewStyle(),
				muted:     lipgloss.NewStyle(),
				toolBlock: lipgloss.NewStyle(),
			},
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
	if strings.Contains(collapsed, "ctrl+o") {
		t.Fatalf("collapsed tool entry should not render persistent hint:\n%s", collapsed)
	}
	collapsedPlain := plainANSI(collapsed)
	if strings.Contains(collapsedPlain, "Used read_file") || !strings.Contains(collapsedPlain, "Read README.md") ||
		!strings.Contains(collapsedPlain, "└  1 line") {
		t.Fatalf("collapsed tool entry = %q, want pretty action and summary", collapsedPlain)
	}

	m.showEvents = true
	expanded := m.renderToolEntry(entry)
	for _, want := range []string{"Read README.md", `{"path":"README.md"}`, "project docs"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded tool entry missing %q:\n%s", want, expanded)
		}
	}
}

func TestRenderBashToolEntryUsesCodexLikeDetailTreatment(t *testing.T) {
	m := model{
		layoutState: layoutState{
			width:      80,
			density:    densityComfortable,
			showEvents: true,
		},
		visualState: visualState{
			styles: styles{
				tool:      lipgloss.NewStyle(),
				error:     lipgloss.NewStyle(),
				muted:     lipgloss.NewStyle(),
				toolBlock: lipgloss.NewStyle(),
			},
		},
	}
	entry := transcriptEntry{
		kind:       transcriptTool,
		text:       "Used bash",
		toolName:   "bash",
		toolInput:  `{"command":"git diff --stat"}`,
		toolResult: "epsilon-cli/tui/tui.go | 12 +++++\n1 file changed",
	}

	rendered := plainANSI(m.renderToolEntry(entry))
	for _, want := range []string{"• Ran git diff --stat", "  $ git diff --stat", "result", "1 file changed"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("bash tool entry missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderToolGroupHidesNestedDetailsUntilExpanded(t *testing.T) {
	m := model{
		layoutState: layoutState{
			width:   80,
			density: densityCompact,
		},
		visualState: visualState{
			styles: styles{
				tool:      lipgloss.NewStyle(),
				error:     lipgloss.NewStyle(),
				muted:     lipgloss.NewStyle(),
				toolBlock: lipgloss.NewStyle(),
			},
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
	if strings.Contains(collapsed, "ctrl+o") {
		t.Fatalf("collapsed tool group should not render persistent hint:\n%s", collapsed)
	}

	m.showEvents = true
	expanded := m.renderToolGroupEntry(entry)
	for _, want := range []string{"Explored 2 items", "read_file", `{"path":"README.md"}`, "No matches found."} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded tool group missing %q:\n%s", want, expanded)
		}
	}
}

func TestDefaultSessionTitle(t *testing.T) {
	got := defaultSessionTitle("  please   trace\nthis conversation clearly  ", 18)
	if got != "please trace this" {
		t.Fatalf("title = %q, want %q", got, "please trace this")
	}

	got = defaultSessionTitle("hello 世界 from epsilon", 8)
	if got != "hello 世界" {
		t.Fatalf("unicode title = %q, want %q", got, "hello 世界")
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

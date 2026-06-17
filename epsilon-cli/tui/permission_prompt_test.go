package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/shxntanu/epsilon/core/types"
)

func TestPermissionPromptCyclesThroughChoices(t *testing.T) {
	prompt := newPermissionPrompt(types.PermissionRequest{ToolName: "read_file"})

	if prompt.Decision() != types.PermissionDecisionAllow {
		t.Fatalf("initial decision = %q, want allow", prompt.Decision())
	}

	prompt = prompt.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if prompt.Decision() != types.PermissionDecisionAllowSession {
		t.Fatalf("second decision = %q, want allow_session", prompt.Decision())
	}

	prompt = prompt.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if prompt.Decision() != types.PermissionDecisionDeny {
		t.Fatalf("third decision = %q, want deny", prompt.Decision())
	}

	prompt = prompt.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if prompt.Decision() != types.PermissionDecisionAllowSession {
		t.Fatalf("after shift+tab decision = %q, want allow_session", prompt.Decision())
	}
}

func TestPermissionPromptViewShowsSessionOption(t *testing.T) {
	prompt := newPermissionPrompt(types.PermissionRequest{
		ToolName: "read_file",
		Input:    []byte(`{"path":"README.md"}`),
	})
	prompt.SetWidth(120)

	view := prompt.View()
	for _, want := range []string{"Approve tool call?", "read_file", "Allow once", "Allow session", "Deny"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q in %q", want, view)
		}
	}
}

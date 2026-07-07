package core

import (
	"context"
	"strings"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestDefaultToolsExcludeEcho(t *testing.T) {
	provider := &recordingProvider{}
	harness, err := New(
		WithProvider(provider),
		WithDefaultTools(t.TempDir()),
		WithSessionIDGenerator(func() (string, error) { return "session_default_tools", nil }),
	)
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	sess, err := harness.StartSession(context.Background())
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if err := sess.Send(context.Background(), types.UserMessage("hello")); err != nil {
		t.Fatalf("send message: %v", err)
	}
	if err := sess.Step(context.Background()); err != nil {
		t.Fatalf("step: %v", err)
	}

	found := false
	hasRipgrep := false
	for _, tool := range provider.request.Tools {
		if tool.Name == "echo" {
			found = true
		}
		if tool.Name == "ripgrep" {
			hasRipgrep = true
		}
	}
	if found {
		t.Fatalf("default tools unexpectedly included echo")
	}
	if !hasRipgrep {
		t.Fatalf("default tools did not include ripgrep")
	}
	if len(provider.request.Tools) == 0 {
		t.Fatalf("default tools were not sent")
	}
}

func TestPlanModeExposesOnlyReadOnlyToolsAndPrompt(t *testing.T) {
	provider := &recordingProvider{}
	harness, err := New(
		WithProvider(provider),
		WithDefaultTools(t.TempDir()),
		WithPlanMode(true),
		WithSessionIDGenerator(func() (string, error) { return "session_plan_mode", nil }),
	)
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	sess, err := harness.StartSession(context.Background())
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if err := sess.Send(context.Background(), types.UserMessage("change the code")); err != nil {
		t.Fatalf("send message: %v", err)
	}
	if err := sess.Step(context.Background()); err != nil {
		t.Fatalf("step: %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range provider.request.Tools {
		toolNames[tool.Name] = true
	}
	for _, want := range []string{"read_file", "list_dir", "file_tree", "grep_search", "ripgrep", "git_status", "git_diff"} {
		if !toolNames[want] {
			t.Fatalf("plan mode missing read-only tool %q in %#v", want, toolNames)
		}
	}
	for _, forbidden := range []string{"write_file", "apply_patch", "bash"} {
		if toolNames[forbidden] {
			t.Fatalf("plan mode exposed mutating tool %q", forbidden)
		}
	}
	if len(provider.request.Messages) == 0 || provider.request.Messages[0].Role != types.RoleSystem {
		t.Fatalf("plan mode did not send a system prompt first")
	}
	if !strings.Contains(provider.request.Messages[0].Content[0].Text, "Plan mode is active") {
		t.Fatalf("plan mode prompt missing instructions: %q", provider.request.Messages[0].Content[0].Text)
	}
}

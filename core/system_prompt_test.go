package core

import (
	"context"
	"strings"
	"testing"

	"github.com/shxntanu/epsilon/core/prompts"
	"github.com/shxntanu/epsilon/core/types"
)

func TestDefaultSystemPromptIsInjectedIntoModelRequest(t *testing.T) {
	provider := &recordingProvider{}
	harness, err := New(
		WithProvider(provider),
		WithSessionIDGenerator(func() (string, error) { return "session_default_system_prompt", nil }),
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

	if len(provider.request.Messages) != 2 {
		t.Fatalf("request messages = %d, want 2", len(provider.request.Messages))
	}
	if provider.request.Messages[0].Role != types.RoleSystem {
		t.Fatalf("first role = %q, want system", provider.request.Messages[0].Role)
	}
	if provider.request.Messages[0].Content[0].Text != prompts.DefaultAgent().Render() {
		t.Fatalf("system prompt = %q, want default prompt", provider.request.Messages[0].Content[0].Text)
	}
}

func TestSystemPromptIsInjectedIntoModelRequest(t *testing.T) {
	provider := &recordingProvider{}
	harness, err := New(
		WithProvider(provider),
		WithSessionIDGenerator(func() (string, error) { return "session_system_prompt", nil }),
		WithSystemPrompt("follow the harness policy"),
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

	if len(provider.request.Messages) != 2 {
		t.Fatalf("request messages = %d, want 2", len(provider.request.Messages))
	}
	if provider.request.Messages[0].Role != types.RoleSystem {
		t.Fatalf("first role = %q, want system", provider.request.Messages[0].Role)
	}
	prompt := provider.request.Messages[0].Content[0].Text
	if !strings.HasPrefix(prompt, prompts.DefaultAgent().Render()) {
		t.Fatalf("system prompt does not start with default prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "follow the harness policy") {
		t.Fatalf("system prompt does not include configured prompt: %q", prompt)
	}
	if provider.request.Messages[1].Role != types.RoleUser {
		t.Fatalf("second role = %q, want user", provider.request.Messages[1].Role)
	}

	for _, msg := range sess.Messages() {
		if msg.Role == types.RoleSystem {
			t.Fatalf("system prompt should not be persisted in session messages")
		}
	}
}

func TestHarnessCanRenderOtherPromptKinds(t *testing.T) {
	harness, err := New()
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	prompt, ok := harness.RenderPrompt(prompts.Title, "Use at most five words.")
	if !ok {
		t.Fatalf("expected title prompt")
	}
	if !strings.Contains(prompt, prompts.DefaultTitle().Render()) {
		t.Fatalf("prompt does not include default title prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "Use at most five words.") {
		t.Fatalf("prompt does not include extension: %q", prompt)
	}
}

type recordingProvider struct {
	request types.ModelRequest
}

func (p *recordingProvider) Respond(_ context.Context,
	req types.ModelRequest) (*types.ModelResponse, error) {
	p.request = cloneModelRequest(req)
	return &types.ModelResponse{
		Message: types.AssistantMessage("ok"),
	}, nil
}

func cloneModelRequest(req types.ModelRequest) types.ModelRequest {
	req.Messages = append([]types.Message(nil), req.Messages...)
	req.Tools = append([]types.ToolDefinition(nil), req.Tools...)
	return req
}

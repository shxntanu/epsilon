package core

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

func TestSessionStepAllowsMoreThanTwentyToolTurns(t *testing.T) {
	provider := &manyToolTurnsProvider{turns: 25}
	harness, err := New(
		WithProvider(provider),
		WithTool(tools.NewEchoTool()),
		WithSessionIDGenerator(func() (string, error) { return "session_many_tools", nil }),
	)
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	sess, err := harness.StartSession(context.Background())
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if err := sess.Send(context.Background(), types.UserMessage("explore freely")); err != nil {
		t.Fatalf("send: %v", err)
	}

	if err := sess.Step(context.Background()); err != nil {
		t.Fatalf("step: %v", err)
	}

	if provider.calls != provider.turns+1 {
		t.Fatalf("provider calls = %d, want %d", provider.calls, provider.turns+1)
	}
	messages := sess.Messages()
	if got := messages[len(messages)-1].Content[0].Text; got != "done after 25 tools" {
		t.Fatalf("final message = %q, want completion after 25 tools", got)
	}
}

type manyToolTurnsProvider struct {
	turns int
	calls int
}

func (p *manyToolTurnsProvider) Respond(_ context.Context,
	req types.ModelRequest) (*types.ModelResponse, error) {
	p.calls++
	completedTools := 0
	for _, msg := range req.Messages {
		if msg.Role == types.RoleTool {
			completedTools++
		}
	}
	if completedTools >= p.turns {
		return &types.ModelResponse{
			Message: types.AssistantMessage(fmt.Sprintf("done after %d tools", completedTools)),
		}, nil
	}

	input, err := json.Marshal(map[string]string{
		"text": fmt.Sprintf("turn %d", completedTools+1),
	})
	if err != nil {
		return nil, err
	}
	return &types.ModelResponse{
		Message: types.Message{
			Role: types.RoleAssistant,
			ToolCalls: []types.ToolCall{{
				ID:    fmt.Sprintf("call_%d", completedTools+1),
				Name:  "echo",
				Input: input,
			}},
		},
	}, nil
}

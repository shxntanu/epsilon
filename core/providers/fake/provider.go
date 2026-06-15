package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Respond(ctx context.Context,
	req types.ModelRequest) (*types.ModelResponse, error) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role == types.RoleUser && len(msg.Content) > 0 {
			text := msg.Content[0].Text
			if strings.HasPrefix(text, "tool:") {
				message, err := toolMessageFromPrompt(text)
				if err != nil {
					return nil, err
				}

				return &types.ModelResponse{
					Message: message,
				}, nil
			}

			return &types.ModelResponse{
				Message: types.AssistantMessage("Echo: " + text),
			}, nil
		}
	}

	return &types.ModelResponse{
		Message: types.AssistantMessage("Echo:"),
	}, nil
}

func toolMessageFromPrompt(prompt string) (types.Message, error) {
	command := strings.TrimSpace(strings.TrimPrefix(prompt, "tool:"))
	name, rest, _ := strings.Cut(command, " ")

	switch name {
	case "echo":
		input, err := json.Marshal(map[string]string{
			"text": strings.TrimSpace(rest),
		})
		if err != nil {
			return types.Message{}, fmt.Errorf("encode echo tool input: %w", err)
		}
		return toolCallMessage("fake_call_echo", "echo", input), nil
	case "read":
		path := strings.TrimSpace(rest)
		input, err := json.Marshal(map[string]string{
			"path": path,
		})
		if err != nil {
			return types.Message{}, fmt.Errorf("encode read_file tool input: %w", err)
		}
		return toolCallMessage("fake_call_read", "read_file", input), nil
	case "write":
		path, content, ok := strings.Cut(strings.TrimSpace(rest), " ")
		if !ok {
			return types.Message{}, fmt.Errorf("tool:write requires path and content")
		}
		input, err := json.Marshal(map[string]any{
			"path":      path,
			"content":   content,
			"overwrite": true,
		})
		if err != nil {
			return types.Message{}, fmt.Errorf("encode write_file tool input: %w", err)
		}
		return toolCallMessage("fake_call_write", "write_file", input), nil
	default:
		return types.Message{}, fmt.Errorf("unknown fake tool command: %q", name)
	}
}

func toolCallMessage(id string, name string, input json.RawMessage) types.Message {
	return types.Message{
		Role: types.RoleAssistant,
		ToolCalls: []types.ToolCall{
			{
				ID:    id,
				Name:  name,
				Input: input,
			},
		},
	}
}

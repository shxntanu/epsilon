package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
		input, err := readToolInputFromPrompt(rest)
		if err != nil {
			return types.Message{}, err
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

func readToolInputFromPrompt(prompt string) (json.RawMessage, error) {
	fields := strings.Fields(strings.TrimSpace(prompt))
	if len(fields) == 0 {
		return nil, fmt.Errorf("tool:read requires path")
	}
	if len(fields) > 3 {
		return nil, fmt.Errorf("tool:read accepts path and optional start_line end_line")
	}

	input := map[string]any{
		"path": fields[0],
	}
	if len(fields) >= 2 {
		startLine, err := parsePositiveLineNumber(fields[1], "start_line")
		if err != nil {
			return nil, err
		}
		input["start_line"] = startLine
	}
	if len(fields) == 3 {
		endLine, err := parsePositiveLineNumber(fields[2], "end_line")
		if err != nil {
			return nil, err
		}
		input["end_line"] = endLine
	}

	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode read_file tool input: %w", err)
	}
	return data, nil
}

func parsePositiveLineNumber(value string, name string) (int, error) {
	line, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("tool:read %s must be a positive integer", name)
	}
	if line < 1 {
		return 0, fmt.Errorf("tool:read %s must be >= 1", name)
	}
	return line, nil
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

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shxntanu/epsilon/core/types"
)

// EchoTool returns the text it receives.
type EchoTool struct{}

// NewEchoTool returns a tool useful for smoke-testing tool execution.
func NewEchoTool() *EchoTool {
	return &EchoTool{}
}

// Name returns the tool name exposed to the model.
func (t *EchoTool) Name() string {
	return "echo"
}

// Description returns the tool description exposed to the model.
func (t *EchoTool) Description() string {
	return "Return the provided text unchanged. Useful for testing tool calls."
}

// InputSchema returns the tool input schema.
func (t *EchoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","description"` +
		`:"Text to echo back."}},"required":["text"],"additionalProperties":false}`)
}

// Run executes the tool.
func (t *EchoTool) Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("echo cancelled: %w", ctx.Err())
	default:
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode echo input: %w", err)
	}

	result := types.TextToolResult(req.Text)
	return &result, nil
}

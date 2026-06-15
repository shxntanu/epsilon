package tools

import (
	"context"
	"encoding/json"

	"github.com/shxntanu/epsilon/core/types"
)

// Tool is an executable capability that can be offered to a model.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Run(ctx context.Context, input json.RawMessage) (*types.ToolResult, error)
}

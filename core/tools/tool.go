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

// Permissioner can be implemented by tools that want to declare their default permission mode.
type Permissioner interface {
	Permission() types.PermissionMode
}

// Permission returns the permission mode for a tool.
func Permission(tool Tool) types.PermissionMode {
	permissioner, ok := tool.(Permissioner)
	if !ok {
		return types.PermissionAsk
	}

	mode := permissioner.Permission()
	if mode == "" {
		return types.PermissionAsk
	}

	return mode
}

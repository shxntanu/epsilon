package tools

import (
	"errors"
	"sort"
	"sync"

	"github.com/shxntanu/epsilon/core/types"
)

var (
	ErrToolNil               = errors.New("tool is nil")
	ErrToolNameEmpty         = errors.New("tool name is empty")
	ErrToolAlreadyRegistered = errors.New("tool already registered")
	ErrToolNotFound          = errors.New("tool not found")
)

// Registry stores the tools available to a session.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return ErrToolNil
	}
	if tool.Name() == "" {
		return ErrToolNameEmpty
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[tool.Name()]; exists {
		return ErrToolAlreadyRegistered
	}

	r.tools[tool.Name()] = tool
	return nil
}

// RegisterAll adds multiple tools to the registry in order.
func (r *Registry) RegisterAll(toolList ...Tool) error {
	for _, tool := range toolList {
		if err := r.Register(tool); err != nil {
			return err
		}
	}
	return nil
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	if name == "" {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// Definitions returns provider-facing tool definitions in a stable order.
func (r *Registry) Definitions() []types.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]types.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		defs = append(defs, types.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}
	return defs
}

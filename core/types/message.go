package types

import "encoding/json"

// Role identifies who produced a message in a session.
type Role string

const (
	// RoleSystem is reserved for harness and application instructions.
	RoleSystem Role = "system"
	// RoleUser represents input from the human or calling application.
	RoleUser Role = "user"
	// RoleAssistant represents output from the model.
	RoleAssistant Role = "assistant"
	// RoleTool represents the result of a tool call.
	RoleTool Role = "tool"
)

// ContentKind identifies the shape of a message content part.
type ContentKind string

const (
	// ContentText is plain UTF-8 text.
	ContentText ContentKind = "text"
)

// ContentPart is one piece of message content.
type ContentPart struct {
	Kind ContentKind `json:"kind"`
	Text string      `json:"text,omitempty"`
}

// TextPart returns a text content part.
func TextPart(text string) ContentPart {
	return ContentPart{
		Kind: ContentText,
		Text: text,
	}
}

// Message is the provider-neutral conversation unit used by the harness.
type Message struct {
	Role       Role          `json:"role"`
	Content    []ContentPart `json:"content,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolName   string        `json:"tool_name,omitempty"`
}

// SystemMessage returns a system message with text content.
func SystemMessage(text string) Message {
	return Message{
		Role:    RoleSystem,
		Content: []ContentPart{TextPart(text)},
	}
}

// UserMessage returns a user message with text content.
func UserMessage(text string) Message {
	return Message{
		Role:    RoleUser,
		Content: []ContentPart{TextPart(text)},
	}
}

// AssistantMessage returns an assistant message with text content.
func AssistantMessage(text string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: []ContentPart{TextPart(text)},
	}
}

// ToolMessage returns a tool-result message tied to a previous tool call.
func ToolMessage(callID string, name string, result ToolResult) Message {
	return Message{
		Role:       RoleTool,
		Content:    result.Content,
		ToolCallID: callID,
		ToolName:   name,
	}
}

// ToolCall is a model request to execute a named tool with JSON input.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

// ToolResult is the structured output returned by a tool.
type ToolResult struct {
	Content  []ContentPart     `json:"content,omitempty"`
	IsError  bool              `json:"is_error,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TextToolResult returns a successful text tool result.
func TextToolResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentPart{TextPart(text)},
	}
}

// ErrorToolResult returns a text tool result that represents tool failure.
func ErrorToolResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentPart{TextPart(text)},
		IsError: true,
	}
}

// ModelRequest is the provider-neutral input sent to an LLM provider.
type ModelRequest struct {
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

// ModelResponse is the provider-neutral output returned by an LLM provider.
type ModelResponse struct {
	Message Message `json:"message"`
	Usage   Usage   `json:"usage,omitempty"`
}

// ToolDefinition describes a tool that a provider can expose to a model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// Usage captures token accounting reported by a provider.
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}

// ModelInfo is provider-neutral metadata about an available model.
type ModelInfo struct {
	ID              string       `json:"id"`
	Name            string       `json:"name,omitempty"`
	Provider        string       `json:"provider,omitempty"`
	ProviderModel   string       `json:"provider_model,omitempty"`
	Mode            string       `json:"mode,omitempty"`
	MaxInputTokens  int          `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int          `json:"max_output_tokens,omitempty"`
	MaxTokens       int          `json:"max_tokens,omitempty"`
	Pricing         ModelPricing `json:"pricing,omitempty"`
}

// ModelPricing captures token pricing normalized to per-token cost.
type ModelPricing struct {
	InputCostPerToken        float64 `json:"input_cost_per_token,omitempty"`
	OutputCostPerToken       float64 `json:"output_cost_per_token,omitempty"`
	CacheReadCostPerToken    float64 `json:"cache_read_cost_per_token,omitempty"`
	CacheWriteCostPerToken   float64 `json:"cache_write_cost_per_token,omitempty"`
	ReasoningCostPerToken    float64 `json:"reasoning_cost_per_token,omitempty"`
	InputCostPer1KTokens     float64 `json:"input_cost_per_1k_tokens,omitempty"`
	OutputCostPer1KTokens    float64 `json:"output_cost_per_1k_tokens,omitempty"`
	CacheReadCostPer1KToken  float64 `json:"cache_read_cost_per_1k_tokens,omitempty"`
	CacheWriteCostPer1KToken float64 `json:"cache_write_cost_per_1k_tokens,omitempty"`
}

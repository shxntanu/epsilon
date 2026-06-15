package types

import "encoding/json"

// PermissionMode describes how a tool call should be authorized.
type PermissionMode string

const (
	// PermissionAllow runs the tool without asking a broker.
	PermissionAllow PermissionMode = "allow"
	// PermissionAsk asks the configured permission broker before running the tool.
	PermissionAsk PermissionMode = "ask"
	// PermissionDeny refuses the tool without asking a broker.
	PermissionDeny PermissionMode = "deny"
)

// PermissionDecision describes the outcome of a permission request.
type PermissionDecision string

const (
	// PermissionDecisionAllow allows the requested action.
	PermissionDecisionAllow PermissionDecision = "allow"
	// PermissionDecisionDeny denies the requested action.
	PermissionDecisionDeny PermissionDecision = "deny"
)

// PermissionRequest describes a tool action that may need approval.
type PermissionRequest struct {
	RunID      string          `json:"run_id,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name"`
	Input      json.RawMessage `json:"input,omitempty"`
	Mode       PermissionMode  `json:"mode"`
	Reason     string          `json:"reason,omitempty"`
}

// PermissionResult records the outcome of a permission request.
type PermissionResult struct {
	Request  PermissionRequest  `json:"request"`
	Decision PermissionDecision `json:"decision"`
	Reason   string             `json:"reason,omitempty"`
}

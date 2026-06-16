package types

import "time"

// EventKind identifies what happened during a session.
type EventKind string

const (
	EventSessionStarted        EventKind = "session_started"
	EventUserMessageAdded      EventKind = "user_message_added"
	EventModelStarted          EventKind = "model_started"
	EventModelTextDelta        EventKind = "model_text_delta"
	EventModelMessageCompleted EventKind = "model_message_completed"
	EventToolCallRequested     EventKind = "tool_call_requested"
	EventPermissionRequested   EventKind = "permission_requested"
	EventPermissionGranted     EventKind = "permission_granted"
	EventPermissionDenied      EventKind = "permission_denied"
	EventToolCallStarted       EventKind = "tool_call_started"
	EventToolCallCompleted     EventKind = "tool_call_completed"
	EventSessionCompleted      EventKind = "session_completed"
	EventSessionError          EventKind = "session_error"
)

// Event is an append-only fact emitted by the harness.
type Event struct {
	ID        string    `json:"id,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Sequence  int64     `json:"sequence,omitempty"`
	Kind      EventKind `json:"kind"`
	CreatedAt time.Time `json:"created_at"`

	Message    *Message          `json:"message,omitempty"`
	TextDelta  string            `json:"text_delta,omitempty"`
	ToolCall   *ToolCall         `json:"tool_call,omitempty"`
	ToolResult *ToolResult       `json:"tool_result,omitempty"`
	Permission *PermissionResult `json:"permission,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// NewSessionStartedEvent returns an event for the beginning of a session.
func NewSessionStartedEvent(now time.Time) Event {
	return Event{
		Kind:      EventSessionStarted,
		CreatedAt: now,
	}
}

// NewUserMessageAddedEvent returns an event for accepted user input.
func NewUserMessageAddedEvent(now time.Time, msg Message) Event {
	return Event{
		Kind:      EventUserMessageAdded,
		CreatedAt: now,
		Message:   &msg,
	}
}

// NewModelTextDeltaEvent returns a streaming text delta event.
func NewModelTextDeltaEvent(now time.Time, delta string) Event {
	return Event{
		Kind:      EventModelTextDelta,
		CreatedAt: now,
		TextDelta: delta,
	}
}

// NewToolCallRequestedEvent returns an event for a tool call requested by the model.
func NewToolCallRequestedEvent(now time.Time, call ToolCall) Event {
	return Event{
		Kind:      EventToolCallRequested,
		CreatedAt: now,
		ToolCall:  &call,
	}
}

// NewPermissionRequestedEvent returns an event for a permission request.
func NewPermissionRequestedEvent(now time.Time, request PermissionRequest) Event {
	return Event{
		Kind:      EventPermissionRequested,
		CreatedAt: now,
		Permission: &PermissionResult{
			Request: request,
		},
	}
}

// NewPermissionGrantedEvent returns an event for an allowed permission decision.
func NewPermissionGrantedEvent(now time.Time, result PermissionResult) Event {
	return Event{
		Kind:       EventPermissionGranted,
		CreatedAt:  now,
		Permission: &result,
	}
}

// NewPermissionDeniedEvent returns an event for a denied permission decision.
func NewPermissionDeniedEvent(now time.Time, result PermissionResult) Event {
	return Event{
		Kind:       EventPermissionDenied,
		CreatedAt:  now,
		Permission: &result,
	}
}

// NewToolCallStartedEvent returns an event for a tool call beginning execution.
func NewToolCallStartedEvent(now time.Time, call ToolCall) Event {
	return Event{
		Kind:      EventToolCallStarted,
		CreatedAt: now,
		ToolCall:  &call,
	}
}

// NewToolCallCompletedEvent returns an event for a completed tool call.
func NewToolCallCompletedEvent(now time.Time, call ToolCall, result ToolResult) Event {
	return Event{
		Kind:       EventToolCallCompleted,
		CreatedAt:  now,
		ToolCall:   &call,
		ToolResult: &result,
	}
}

// NewSessionErrorEvent returns an event for a session-level error.
func NewSessionErrorEvent(now time.Time, err error) Event {
	return Event{
		Kind:      EventSessionError,
		CreatedAt: now,
		Error:     err.Error(),
	}
}

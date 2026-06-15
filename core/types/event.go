package types

import "time"

// EventKind identifies what happened during a run.
type EventKind string

const (
	EventRunStarted            EventKind = "run_started"
	EventUserMessageAdded      EventKind = "user_message_added"
	EventModelStarted          EventKind = "model_started"
	EventModelTextDelta        EventKind = "model_text_delta"
	EventModelMessageCompleted EventKind = "model_message_completed"
	EventToolCallRequested     EventKind = "tool_call_requested"
	EventToolCallStarted       EventKind = "tool_call_started"
	EventToolCallCompleted     EventKind = "tool_call_completed"
	EventRunCompleted          EventKind = "run_completed"
	EventRunError              EventKind = "run_error"
)

// Event is an append-only fact emitted by the harness.
type Event struct {
	ID        string    `json:"id,omitempty"`
	RunID     string    `json:"run_id,omitempty"`
	Sequence  int64     `json:"sequence,omitempty"`
	Kind      EventKind `json:"kind"`
	CreatedAt time.Time `json:"created_at"`

	Message    *Message    `json:"message,omitempty"`
	TextDelta  string      `json:"text_delta,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// NewRunStartedEvent returns an event for the beginning of a run.
func NewRunStartedEvent(now time.Time) Event {
	return Event{
		Kind:      EventRunStarted,
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

// NewRunErrorEvent returns an event for a run-level error.
func NewRunErrorEvent(now time.Time, err error) Event {
	return Event{
		Kind:      EventRunError,
		CreatedAt: now,
		Error:     err.Error(),
	}
}

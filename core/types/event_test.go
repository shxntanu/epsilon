package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUserMessageAddedEvent(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	msg := UserMessage("hello")

	event := NewUserMessageAddedEvent(now, msg)

	if event.Kind != EventUserMessageAdded {
		t.Fatalf("kind = %q, want %q", event.Kind, EventUserMessageAdded)
	}
	if event.Message == nil {
		t.Fatal("message is nil")
	}
	if event.Message.Role != RoleUser {
		t.Fatalf("role = %q, want %q", event.Message.Role, RoleUser)
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	event := NewModelTextDeltaEvent(now, "hello")

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if got.Kind != EventModelTextDelta {
		t.Fatalf("kind = %q, want %q", got.Kind, EventModelTextDelta)
	}
	if got.TextDelta != "hello" {
		t.Fatalf("text delta = %q, want hello", got.TextDelta)
	}
}

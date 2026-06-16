package events

import (
	"context"
	"testing"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

func TestJSONLStoreListSessions(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.Append(context.Background(), types.Event{
		SessionID: "session_a",
		Kind:      types.EventSessionStarted,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append session a: %v", err)
	}
	if err := store.Append(context.Background(), types.Event{
		SessionID: "session_b",
		Kind:      types.EventSessionStarted,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append session b: %v", err)
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}

	seen := map[string]bool{}
	for _, sess := range sessions {
		seen[sess.ID] = true
	}
	if !seen["session_a"] || !seen["session_b"] {
		t.Fatalf("sessions = %#v, want session_a and session_b", sessions)
	}
}

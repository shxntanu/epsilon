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
		Kind:      types.EventUserMessageAdded,
		CreatedAt: time.Now().UTC(),
		Message:   &types.Message{Role: types.RoleUser, Content: []types.ContentPart{types.TextPart("hello")}},
	}); err != nil {
		t.Fatalf("append session a: %v", err)
	}
	if err := store.Append(context.Background(), types.Event{
		SessionID: "session_b",
		Kind:      types.EventModelMessageCompleted,
		CreatedAt: time.Now().UTC(),
		Message:   &types.Message{Role: types.RoleAssistant, Content: []types.ContentPart{types.TextPart("hi")}},
	}); err != nil {
		t.Fatalf("append session b: %v", err)
	}
	if err := store.Append(context.Background(), types.Event{
		SessionID: "session_empty",
		Kind:      types.EventSessionStarted,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append empty session: %v", err)
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

func TestJSONLStoreListSessionsIncludesTitle(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.Append(context.Background(), types.Event{
		SessionID: "session_a",
		Kind:      types.EventUserMessageAdded,
		CreatedAt: time.Now().UTC(),
		Message:   &types.Message{Role: types.RoleUser, Content: []types.ContentPart{types.TextPart("hello")}},
	}); err != nil {
		t.Fatalf("append session a: %v", err)
	}

	ok, err := store.RenameSession(context.Background(), "session_a", "Planning work")
	if err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if !ok {
		t.Fatalf("rename session returned false")
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Title != "Planning work" {
		t.Fatalf("title = %q, want %q", sessions[0].Title, "Planning work")
	}
}

func TestJSONLStoreRenameSessionWithoutEvents(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ok, err := store.RenameSession(context.Background(), "session_a", "Planning work")
	if err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if !ok {
		t.Fatalf("rename session returned false")
	}

	exists, err := store.SessionExists(context.Background(), "session_a")
	if err != nil {
		t.Fatalf("session exists: %v", err)
	}
	if !exists {
		t.Fatalf("session should exist after rename")
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].ID != "session_a" || sessions[0].Title != "Planning work" {
		t.Fatalf("session = %#v, want session_a with title", sessions[0])
	}
}

func TestJSONLStoreDeleteSession(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.Append(context.Background(), types.Event{
		SessionID: "session_a",
		Kind:      types.EventUserMessageAdded,
		CreatedAt: time.Now().UTC(),
		Message:   &types.Message{Role: types.RoleUser, Content: []types.ContentPart{types.TextPart("hello")}},
	}); err != nil {
		t.Fatalf("append session: %v", err)
	}
	if ok, err := store.RenameSession(context.Background(), "session_a", "Planning work"); err != nil || !ok {
		t.Fatalf("rename session: ok=%v err=%v", ok, err)
	}

	ok, err := store.DeleteSession(context.Background(), "session_a")
	if err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if !ok {
		t.Fatalf("delete session returned false")
	}

	exists, err := store.SessionExists(context.Background(), "session_a")
	if err != nil {
		t.Fatalf("session exists: %v", err)
	}
	if exists {
		t.Fatalf("session should not exist after delete")
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want none", sessions)
	}

	ok, err = store.DeleteSession(context.Background(), "session_a")
	if err != nil {
		t.Fatalf("delete missing session: %v", err)
	}
	if ok {
		t.Fatalf("delete missing session returned true")
	}
}

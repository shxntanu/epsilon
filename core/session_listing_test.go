package core

import (
	"context"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestListSessionsOmitsEmptyInMemorySessions(t *testing.T) {
	harness, err := New(
		WithSessionIDGenerator(func() (string, error) { return "session_empty", nil }),
	)
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	sess, err := harness.StartSession(context.Background())
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	sessions, err := harness.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want none for empty session", sessions)
	}

	if err := sess.Send(context.Background(), types.UserMessage("hello")); err != nil {
		t.Fatalf("send message: %v", err)
	}

	sessions, err = harness.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions after message: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "session_empty" {
		t.Fatalf("sessions = %#v, want session_empty", sessions)
	}
}

func TestListSessionsIncludesRenamedEmptySession(t *testing.T) {
	dir := t.TempDir()
	harness, err := New(
		WithSessionDir(dir),
		WithSessionIDGenerator(func() (string, error) { return "session_empty", nil }),
	)
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	sess, err := harness.StartSession(context.Background())
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	ok, err := harness.RenameSession(context.Background(), sess.ID(), "Planning work")
	if err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if !ok {
		t.Fatalf("rename session returned false")
	}

	sessions, err := harness.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want one renamed session", sessions)
	}
	if sessions[0].ID != "session_empty" || sessions[0].Title != "Planning work" {
		t.Fatalf("session = %#v, want renamed empty session", sessions[0])
	}

	resumed, err := harness.ResumeSession(context.Background(), "session_empty")
	if err != nil {
		t.Fatalf("resume renamed empty session: %v", err)
	}
	if resumed.ID() != "session_empty" {
		t.Fatalf("resumed ID = %q, want session_empty", resumed.ID())
	}
}

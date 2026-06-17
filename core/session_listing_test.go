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

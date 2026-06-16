package core

import (
	"context"
	"testing"

	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/types"
)

func TestHarnessContextSummary(t *testing.T) {
	harness, err := New(
		WithSessionIDGenerator(func() (string, error) { return "session_context", nil }),
		WithContextWindow(contextwindow.Config{
			MaxTokens:           100,
			CompactionThreshold: 0.8,
			Estimator: contextwindow.EstimatorFunc(func(string) int {
				return 10
			}),
		}),
	)
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	sess, err := harness.StartSession(context.Background())
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if err := sess.Send(context.Background(), types.UserMessage("hello")); err != nil {
		t.Fatalf("send message: %v", err)
	}

	summary, ok := harness.ContextSummary(sess.ID())
	if !ok {
		t.Fatalf("expected context summary for session")
	}
	if summary.MaxTokens != 100 {
		t.Fatalf("max tokens = %d, want 100", summary.MaxTokens)
	}
	if summary.UsedTokens == 0 {
		t.Fatalf("expected used tokens")
	}

	if _, ok := harness.ContextSummary("missing"); ok {
		t.Fatalf("expected missing session context lookup to fail")
	}
}

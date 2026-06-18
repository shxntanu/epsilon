package core

import (
	"context"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestDefaultToolsExcludeEcho(t *testing.T) {
	provider := &recordingProvider{}
	harness, err := New(
		WithProvider(provider),
		WithDefaultTools(t.TempDir()),
		WithSessionIDGenerator(func() (string, error) { return "session_default_tools", nil }),
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
	if err := sess.Step(context.Background()); err != nil {
		t.Fatalf("step: %v", err)
	}

	found := false
	hasRipgrep := false
	for _, tool := range provider.request.Tools {
		if tool.Name == "echo" {
			found = true
		}
		if tool.Name == "ripgrep" {
			hasRipgrep = true
		}
	}
	if found {
		t.Fatalf("default tools unexpectedly included echo")
	}
	if !hasRipgrep {
		t.Fatalf("default tools did not include ripgrep")
	}
	if len(provider.request.Tools) == 0 {
		t.Fatalf("default tools were not sent")
	}
}

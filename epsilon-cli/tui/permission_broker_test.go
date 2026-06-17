package tui

import (
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestPermissionBrokerCachesSessionApprovalByTool(t *testing.T) {
	broker := NewPermissionBroker()
	request := types.PermissionRequest{
		SessionID: "session_123",
		ToolName:  "read_file",
		Input:     []byte(`{"path":"README.md"}`),
	}

	broker.Resolve(request, types.PermissionDecisionAllowSession, "allowed for this session from TUI")

	cached, ok := broker.CachedDecision(types.PermissionRequest{
		SessionID: "session_123",
		ToolName:  "read_file",
		Input:     []byte(`{"path":"go.mod"}`),
	})
	if !ok {
		t.Fatalf("expected cached session approval")
	}
	if cached.Decision != types.PermissionDecisionAllowSession {
		t.Fatalf("decision = %q, want allow_session", cached.Decision)
	}
	if string(cached.Request.Input) != `{"path":"go.mod"}` {
		t.Fatalf("request input = %s, want new request input", cached.Request.Input)
	}

	if _, ok := broker.CachedDecision(types.PermissionRequest{
		SessionID: "session_123",
		ToolName:  "write_file",
		Input:     []byte(`{"path":"go.mod"}`),
	}); ok {
		t.Fatalf("unexpected cached decision for different tool")
	}
}

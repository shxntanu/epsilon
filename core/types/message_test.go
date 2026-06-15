package types

import (
	"encoding/json"
	"testing"
)

func TestMessageConstructors(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		role Role
		text string
	}{
		{
			name: "system",
			msg:  SystemMessage("follow project instructions"),
			role: RoleSystem,
			text: "follow project instructions",
		},
		{
			name: "user",
			msg:  UserMessage("hello"),
			role: RoleUser,
			text: "hello",
		},
		{
			name: "assistant",
			msg:  AssistantMessage("hi there"),
			role: RoleAssistant,
			text: "hi there",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.Role != tt.role {
				t.Fatalf("role = %q, want %q", tt.msg.Role, tt.role)
			}
			if len(tt.msg.Content) != 1 {
				t.Fatalf("content length = %d, want 1", len(tt.msg.Content))
			}
			if tt.msg.Content[0].Kind != ContentText {
				t.Fatalf("content kind = %q, want %q", tt.msg.Content[0].Kind, ContentText)
			}
			if tt.msg.Content[0].Text != tt.text {
				t.Fatalf("content text = %q, want %q", tt.msg.Content[0].Text, tt.text)
			}
		})
	}
}

func TestToolCallJSONRoundTrip(t *testing.T) {
	input := json.RawMessage(`{"path":"README.md"}`)
	msg := Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{
			{
				ID:    "call_123",
				Name:  "read_file",
				Input: input,
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}

	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool calls length = %d, want 1", len(got.ToolCalls))
	}
	if got.ToolCalls[0].ID != "call_123" {
		t.Fatalf("tool call id = %q, want call_123", got.ToolCalls[0].ID)
	}
	if got.ToolCalls[0].Name != "read_file" {
		t.Fatalf("tool call name = %q, want read_file", got.ToolCalls[0].Name)
	}
	if string(got.ToolCalls[0].Input) != string(input) {
		t.Fatalf("tool call input = %s, want %s", got.ToolCalls[0].Input, input)
	}
}

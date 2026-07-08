package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

func TestHydrateTranscriptEntriesBuildsMessagesWithoutReplay(t *testing.T) {
	history := []types.Event{
		types.NewUserMessageAddedEvent(time.Now().UTC(), types.UserMessage("hello")),
		{
			Kind:      types.EventModelMessageCompleted,
			CreatedAt: time.Now().UTC(),
			Message:   &types.Message{Role: types.RoleAssistant, Content: []types.ContentPart{types.TextPart("hi there")}},
		},
	}

	entries := hydrateTranscriptEntries(history)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].kind != transcriptUser || entries[0].text != "hello" {
		t.Fatalf("first entry = %#v, want user hello", entries[0])
	}
	if entries[1].kind != transcriptAgent || entries[1].text != "hi there" {
		t.Fatalf("second entry = %#v, want agent hi there", entries[1])
	}
}

func TestHydrateTranscriptEntriesPreservesCompletedToolState(t *testing.T) {
	call := types.ToolCall{ID: "call_123", Name: "read_file"}
	history := []types.Event{
		{
			Kind:      types.EventModelMessageCompleted,
			CreatedAt: time.Now().UTC(),
			Message:   &types.Message{Role: types.RoleAssistant, ToolCalls: []types.ToolCall{call}},
		},
		types.NewToolCallCompletedEvent(time.Now().UTC(), call, types.TextToolResult("README contents")),
	}

	entries := hydrateTranscriptEntries(history)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].kind != transcriptTool {
		t.Fatalf("entry kind = %v, want transcriptTool", entries[0].kind)
	}
	if !strings.Contains(entries[0].text, "Used read_file") {
		t.Fatalf("tool text = %q, want completed tool text", entries[0].text)
	}
	if entries[0].toolResult != "README contents" {
		t.Fatalf("tool result = %q, want README contents", entries[0].toolResult)
	}
}

func TestHydrateTranscriptEntriesGroupsExplorationTools(t *testing.T) {
	read := types.ToolCall{ID: "call_read", Name: "read_file"}
	search := types.ToolCall{ID: "call_search", Name: "ripgrep"}
	history := []types.Event{
		{
			Kind:      types.EventModelMessageCompleted,
			CreatedAt: time.Now().UTC(),
			Message: &types.Message{Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
				read,
				search,
			}},
		},
		types.NewToolCallCompletedEvent(time.Now().UTC(), read, types.TextToolResult("README contents")),
		types.NewToolCallCompletedEvent(time.Now().UTC(), search, types.TextToolResult("README.md:1:epsilon")),
	}

	entries := hydrateTranscriptEntries(history)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 grouped exploration entry", len(entries))
	}
	if entries[0].kind != transcriptToolGroup {
		t.Fatalf("entry kind = %v, want transcriptToolGroup", entries[0].kind)
	}
	if len(entries[0].tools) != 2 {
		t.Fatalf("group tools = %d, want 2", len(entries[0].tools))
	}
	if !strings.Contains(entries[0].text, "Explored 2 items") {
		t.Fatalf("group text = %q, want aggregate explored text", entries[0].text)
	}
	if entries[0].tools[0].toolResult != "README contents" {
		t.Fatalf("first tool result = %q, want README contents", entries[0].tools[0].toolResult)
	}
	if entries[0].tools[1].toolResult != "README.md:1:epsilon" {
		t.Fatalf("second tool result = %q, want ripgrep result", entries[0].tools[1].toolResult)
	}
}

func TestModelStartedDoesNotAppendThinkingStatus(t *testing.T) {
	m := model{transcriptState: transcriptState{streaming: -1}}
	m.applyEvent(types.Event{
		Kind:      types.EventModelStarted,
		CreatedAt: time.Now().UTC(),
	}, false)

	if len(m.entries) != 0 {
		t.Fatalf("entries = %#v, want no transcript status for model start", m.entries)
	}
	if m.showSpinner {
		t.Fatalf("showSpinner = true, want false when not busy")
	}
}

func TestUsageFromEventsAccumulatesCompletedModelUsage(t *testing.T) {
	history := []types.Event{
		{
			Kind:      types.EventModelMessageCompleted,
			CreatedAt: time.Now().UTC(),
			Usage:     &types.Usage{InputTokens: 10, OutputTokens: 3, TotalTokens: 13},
		},
		types.NewUserMessageAddedEvent(time.Now().UTC(), types.UserMessage("hello")),
		{
			Kind:      types.EventModelMessageCompleted,
			CreatedAt: time.Now().UTC(),
			Usage:     &types.Usage{InputTokens: 7, OutputTokens: 5, TotalTokens: 12},
		},
	}

	got := usageFromEvents(history)
	want := types.Usage{InputTokens: 17, OutputTokens: 8, TotalTokens: 25}
	if got != want {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}
}

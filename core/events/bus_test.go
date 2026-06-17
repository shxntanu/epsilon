package events

import (
	"context"
	"testing"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

func TestSubscribeLiveSkipsHistory(t *testing.T) {
	history := []types.Event{
		{
			SessionID: "session_123",
			Sequence:  1,
			Kind:      types.EventUserMessageAdded,
			CreatedAt: time.Now().UTC(),
			Message:   &types.Message{Role: types.RoleUser, Content: []types.ContentPart{types.TextPart("earlier")}},
		},
	}
	bus := NewBusFromHistory("session_123", 4, nil, history)

	sub, err := bus.SubscribeLive()
	if err != nil {
		t.Fatalf("subscribe live: %v", err)
	}
	defer sub.Close()

	select {
	case event := <-sub.Events():
		t.Fatalf("unexpected replayed event: %#v", event)
	default:
	}

	if _, err := bus.Publish(context.Background(), types.NewUserMessageAddedEvent(
		time.Now().UTC(),
		types.UserMessage("new message"),
	)); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	select {
	case event := <-sub.Events():
		if event.Message == nil || event.Message.Content[0].Text != "new message" {
			t.Fatalf("event = %#v, want published event", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for live event")
	}
}

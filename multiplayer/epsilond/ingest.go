package epsilond

import (
	"context"
	"fmt"

	"github.com/shxntanu/epsilon/multiplayer/chat"
	"github.com/shxntanu/epsilon/multiplayer/chat/googlechat"
)

// BootstrapIngestor normalizes chat events and delegates mentions to an orchestrator.
type BootstrapIngestor struct {
	Orchestrator ChatOrchestrator
	Parser       chat.Parser
}

// IngestGoogleChatEvent accepts a raw Google Chat event body.
func (i BootstrapIngestor) IngestGoogleChatEvent(ctx context.Context, event RawGoogleChatEvent) (IngestResult, error) {
	select {
	case <-ctx.Done():
		return IngestResult{}, fmt.Errorf("ingest cancelled: %w", ctx.Err())
	default:
	}
	parser := i.Parser
	if parser == nil {
		parser = chat.DecodeReaderParser(chat.PlatformGoogleChat, googlechat.DecodeEvent)
	}
	normalized, err := parser.Parse(ctx, chat.RawEvent{
		Platform:   chat.PlatformGoogleChat,
		Body:       event.Body,
		Headers:    event.Headers,
		ReceivedAt: event.ReceivedAt,
	})
	if err != nil {
		return IngestResult{}, err
	}
	if !normalized.IsMention {
		return IngestResult{
			Accepted: false,
			Message:  "ignored: message does not mention Epsilon",
		}, nil
	}
	orchestrator := i.Orchestrator
	if orchestrator == nil {
		orchestrator = BootstrapOrchestrator{}
	}
	handle, err := orchestrator.HandleMention(ctx, normalized)
	if err != nil {
		return IngestResult{}, err
	}
	message := handle.Message
	if message == "" {
		message = "Epsilon accepted the task and will report back in this thread."
	}
	return IngestResult{
		Accepted: true,
		Message:  message,
		TaskID:   handle.TaskID,
	}, nil
}

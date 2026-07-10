package epsilond

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shxntanu/epsilon/multiplayer/chat"
	"github.com/shxntanu/epsilon/multiplayer/chat/googlechat"
	"github.com/shxntanu/epsilon/multiplayer/store"
)

// BootstrapIngestor normalizes chat events and delegates mentions to an orchestrator.
type BootstrapIngestor struct {
	Orchestrator ChatOrchestrator
	Parser       chat.Parser
	ReplyClient  chat.Client
	Store        store.Store
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
	inserted := true
	if i.Store != nil {
		var err error
		inserted, err = i.persistMention(ctx, normalized)
		if err != nil {
			return IngestResult{}, err
		}
		if !inserted {
			return IngestResult{
				Accepted: false,
				Message:  "ignored: duplicate chat message delivery",
			}, nil
		}
	}
	orchestrator := i.Orchestrator
	if orchestrator == nil {
		orchestrator = BootstrapOrchestrator{}
	}
	handle, err := orchestrator.HandleMention(ctx, normalized)
	if err != nil {
		return IngestResult{}, err
	}
	if i.Store != nil {
		if err := i.Store.CreateTask(ctx, store.Task{
			ID:        handle.TaskID,
			ThreadID:  threadID(normalized),
			SpaceID:   spaceID(normalized),
			Status:    store.TaskStatusPending,
			Goal:      normalized.Text,
			Plan:      json.RawMessage(`{}`),
			Budget:    json.RawMessage(`{}`),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}); err != nil {
			return IngestResult{}, err
		}
	}
	message := handle.Message
	if message == "" {
		message = "Epsilon accepted the task and will report back in this thread."
	}
	if i.ReplyClient != nil {
		if err := i.ReplyClient.Reply(ctx, chat.Reply{
			Platform:   normalized.Platform,
			SpaceName:  normalized.SpaceName,
			ThreadName: normalized.ThreadName,
			Text:       message,
		}); err != nil {
			return IngestResult{}, err
		}
	}
	return IngestResult{
		Accepted: true,
		Message:  message,
		TaskID:   handle.TaskID,
	}, nil
}

func (i BootstrapIngestor) persistMention(ctx context.Context, event *chat.Event) (bool, error) {
	now := time.Now().UTC()
	space := store.Space{
		ID:              spaceID(event),
		GoogleSpaceID:   event.SpaceName,
		GoogleSpaceName: event.SpaceName,
		DisplayName:     event.SpaceDisplayName,
		Enabled:         true,
		BudgetPolicy:    json.RawMessage(`{}`),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := i.Store.UpsertSpace(ctx, space); err != nil {
		return false, err
	}
	thread := store.Thread{
		ID:                 threadID(event),
		SpaceID:            space.ID,
		GoogleThreadID:     event.ThreadName,
		GoogleThreadName:   event.ThreadName,
		Status:             store.ThreadStatusActive,
		PinnedFacts:        json.RawMessage(`[]`),
		WorkspaceTier:      "standard",
		TemporalWorkflowID: workflowID(event),
		Version:            1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := i.Store.UpsertThread(ctx, thread); err != nil {
		return false, err
	}
	message := store.ChatMessage{
		ID:                messageID(event),
		SpaceID:           space.ID,
		ThreadID:          thread.ID,
		GoogleMessageID:   event.MessageName,
		GoogleMessageName: event.MessageName,
		GoogleEventID:     event.MessageName,
		SenderID:          event.SenderName,
		SenderDisplayName: event.SenderDisplayName,
		Text:              event.Text,
		ArgumentText:      event.Text,
		MentionedEpsilon:  event.IsMention,
		RawEvent:          event.Raw,
		GoogleCreateTime:  event.CreateTime,
		ReceivedAt:        event.ReceivedAt,
		CreatedAt:         now,
	}
	inserted, err := i.Store.InsertChatMessage(ctx, message)
	if err != nil || !inserted {
		return inserted, err
	}
	for _, attachment := range event.Attachments {
		metadata, _ := json.Marshal(attachment.Metadata)
		if len(metadata) == 0 || string(metadata) == "null" {
			metadata = []byte(`{}`)
		}
		if err := i.Store.InsertChatArtifact(ctx, store.ChatArtifact{
			ID:               stableID("artifact", message.ID, attachment.Name, attachment.ContentName),
			MessageID:        message.ID,
			SpaceID:          space.ID,
			ThreadID:         thread.ID,
			GoogleAttachment: attachment.Name,
			FileName:         attachment.ContentName,
			MimeType:         attachment.ContentType,
			StoragePath:      attachment.DownloadURI,
			ExtractionStatus: store.ArtifactExtractionPending,
			Metadata:         json.RawMessage(metadata),
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func spaceID(event *chat.Event) string {
	return stableID("space", event.Platform, event.SpaceName)
}

func threadID(event *chat.Event) string {
	return stableID("thread", event.Platform, event.SpaceName, event.ThreadName)
}

func messageID(event *chat.Event) string {
	return stableID("message", event.Platform, event.SpaceName, event.MessageName)
}

func workflowID(event *chat.Event) string {
	return "thread/" + spaceID(event) + "/" + threadID(event)
}

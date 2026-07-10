package epsilond

import (
	"context"
	"fmt"

	"github.com/shxntanu/epsilon/multiplayer/chat"
	"github.com/shxntanu/epsilon/multiplayer/orchestrator"
)

// TemporalOrchestrator adapts the durable orchestrator client to epsilond ingestion.
type TemporalOrchestrator struct {
	Client orchestrator.Client
}

// HandleMention starts or signals a durable thread workflow.
func (o TemporalOrchestrator) HandleMention(ctx context.Context, event *chat.Event) (TaskHandle, error) {
	if o.Client == nil {
		return TaskHandle{}, fmt.Errorf("orchestrator client is nil")
	}
	signal := orchestrator.NewChatMessageSignal{
		Thread: orchestrator.ThreadKey{
			SpaceID:          spaceID(event),
			ThreadID:         threadID(event),
			GoogleSpaceName:  event.SpaceName,
			GoogleThreadName: event.ThreadName,
		},
		MessageID:       messageID(event),
		GoogleMessageID: event.MessageName,
		SenderID:        event.SenderName,
		SenderName:      event.SenderDisplayName,
		Text:            event.Text,
		ArgumentText:    event.Text,
		Artifacts:       artifactRefs(event),
		ReceivedAt:      event.ReceivedAt,
	}
	status, err := o.Client.StartOrSignal(ctx, signal)
	if err != nil {
		return TaskHandle{}, err
	}
	taskID := status.ActiveTaskID
	if taskID == "" {
		taskID = signal.MessageID
	}
	return TaskHandle{
		TaskID:     taskID,
		WorkflowID: workflowID(event),
		Message:    "Epsilon accepted the task and will report back in this thread.",
	}, nil
}

func artifactRefs(event *chat.Event) []orchestrator.ArtifactRef {
	refs := make([]orchestrator.ArtifactRef, 0, len(event.Attachments))
	for _, attachment := range event.Attachments {
		refs = append(refs, orchestrator.ArtifactRef{
			ID:       stableID("artifact", event.MessageName, attachment.Name, attachment.ContentName),
			Kind:     "chat_attachment",
			Name:     attachment.ContentName,
			MimeType: attachment.ContentType,
			URI:      attachment.DownloadURI,
			Metadata: attachment.Metadata,
		})
	}
	return refs
}

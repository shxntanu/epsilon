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
func (o TemporalOrchestrator) HandleMention(ctx context.Context,
	event *chat.Event) (TaskHandle, error) {
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

// HandleFollowup signals the existing thread workflow with additional user input.
func (o TemporalOrchestrator) HandleFollowup(ctx context.Context,
	event *chat.Event) (TaskHandle, error) {
	if o.Client == nil {
		return TaskHandle{}, fmt.Errorf("orchestrator client is nil")
	}
	signal := newMessageSignal(event)
	if err := o.Client.SignalMessage(ctx, signal); err != nil {
		return TaskHandle{}, err
	}
	status, err := o.Client.Status(ctx, orchestrator.StatusQuery{Thread: signal.Thread})
	if err != nil {
		return TaskHandle{}, err
	}
	return TaskHandle{
		TaskID:     firstNonEmpty(status.ActiveTaskID, signal.MessageID),
		WorkflowID: workflowID(event),
		Message:    "Epsilon added that to the active thread context.",
	}, nil
}

// HandleInterrupt sends a steering signal to the existing thread workflow.
func (o TemporalOrchestrator) HandleInterrupt(ctx context.Context, event *chat.Event,
	kind orchestrator.InterruptKind, reason string) (TaskHandle, error) {
	if o.Client == nil {
		return TaskHandle{}, fmt.Errorf("orchestrator client is nil")
	}
	signal := newMessageSignal(event)
	status, err := o.Client.Interrupt(ctx, orchestrator.InterruptSignal{
		Thread:    signal.Thread,
		Kind:      kind,
		Reason:    reason,
		MessageID: signal.MessageID,
		IssuedBy:  event.SenderName,
		IssuedAt:  event.ReceivedAt,
	})
	if err != nil {
		return TaskHandle{}, err
	}
	return TaskHandle{
		TaskID:     firstNonEmpty(status.ActiveTaskID, signal.MessageID),
		WorkflowID: workflowID(event),
		Message:    messageForInterrupt(kind),
	}, nil
}

// HandleStatus queries the current thread workflow status.
func (o TemporalOrchestrator) HandleStatus(ctx context.Context, event *chat.Event) (TaskHandle, error) {
	if o.Client == nil {
		return TaskHandle{}, fmt.Errorf("orchestrator client is nil")
	}
	signal := newMessageSignal(event)
	status, err := o.Client.Status(ctx, orchestrator.StatusQuery{Thread: signal.Thread})
	if err != nil {
		return TaskHandle{}, err
	}
	return TaskHandle{
		TaskID:     firstNonEmpty(status.ActiveTaskID, signal.MessageID),
		WorkflowID: workflowID(event),
		Message:    statusMessage(status),
	}, nil
}

func newMessageSignal(event *chat.Event) orchestrator.NewChatMessageSignal {
	return orchestrator.NewChatMessageSignal{
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
}

func messageForInterrupt(kind orchestrator.InterruptKind) string {
	switch kind {
	case orchestrator.InterruptKindCancel:
		return "Epsilon stopped the active work for this thread."
	case orchestrator.InterruptKindPause:
		return "Epsilon paused the active work for this thread."
	case orchestrator.InterruptKindReplan:
		return "Epsilon will replan using the latest instruction."
	default:
		return "Epsilon updated the active thread plan."
	}
}

func statusMessage(status orchestrator.Status) string {
	if status.Error != "" {
		return "Epsilon status: " + string(status.State) + " (" + status.Error + ")"
	}
	if status.BlockedReason != "" {
		return "Epsilon status: " + string(status.State) + " (" + status.BlockedReason + ")"
	}
	if status.LastMessage != "" {
		return "Epsilon status: " + string(status.State) + ". Current task: " + status.LastMessage
	}
	return "Epsilon status: " + string(status.State)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

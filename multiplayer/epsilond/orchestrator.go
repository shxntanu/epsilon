package epsilond

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/shxntanu/epsilon/multiplayer/chat"
)

// ChatOrchestrator starts or signals durable work for a chat thread.
type ChatOrchestrator interface {
	HandleMention(ctx context.Context, event *chat.Event) (TaskHandle, error)
}

// TaskHandle identifies accepted work for a Google Chat thread.
type TaskHandle struct {
	TaskID     string
	WorkflowID string
	Message    string
}

// BootstrapOrchestrator is a local implementation used before Temporal wiring.
type BootstrapOrchestrator struct{}

// HandleMention accepts a mention and returns deterministic placeholder task metadata.
func (BootstrapOrchestrator) HandleMention(ctx context.Context, event *chat.Event) (TaskHandle, error) {
	select {
	case <-ctx.Done():
		return TaskHandle{}, fmt.Errorf("orchestrator cancelled: %w", ctx.Err())
	default:
	}
	if event == nil {
		return TaskHandle{}, fmt.Errorf("chat event is nil")
	}
	threadName := strings.TrimSpace(event.ThreadName)
	if threadName == "" {
		threadName = event.MessageName
	}
	workflowID := workflowID(event)
	taskID := "task_" + stableID(strings.Join([]string{
		event.SpaceName,
		threadName,
		event.MessageName,
	}, "\n"))[:16]
	return TaskHandle{
		TaskID:     taskID,
		WorkflowID: workflowID,
		Message:    "Epsilon accepted the task and will report back in this thread.",
	}, nil
}

func stableID(parts ...any) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, fmt.Sprint(part))
	}
	value := strings.TrimSpace(strings.Join(values, "\x00"))
	if value == "" {
		value = "unknown"
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}

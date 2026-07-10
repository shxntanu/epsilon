// Package temporal provides Temporal-backed orchestration for epsilond.
package temporal

import (
	"context"
	"fmt"
	"strings"

	"github.com/shxntanu/epsilon/multiplayer/orchestrator"
	"github.com/shxntanu/epsilon/multiplayer/temporal/workflows"
	"go.temporal.io/sdk/client"
)

// Client implements orchestrator.Client with Temporal workflows.
type Client struct {
	client    client.Client
	taskQueue string
}

// NewClient returns a Temporal-backed orchestration client.
func NewClient(temporalClient client.Client, taskQueue string) (*Client, error) {
	if temporalClient == nil {
		return nil, fmt.Errorf("temporal client is nil")
	}
	taskQueue = strings.TrimSpace(taskQueue)
	if taskQueue == "" {
		return nil, fmt.Errorf("temporal task queue is empty")
	}
	return &Client{client: temporalClient, taskQueue: taskQueue}, nil
}

// StartOrSignal starts the thread workflow or signals it if it already exists.
func (c *Client) StartOrSignal(ctx context.Context, signal orchestrator.NewChatMessageSignal) (orchestrator.Status, error) {
	workflowID := WorkflowID(signal.Thread)
	run, err := c.client.SignalWithStartWorkflow(ctx, workflowID, workflows.SignalNewChatMessage, signal,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: c.taskQueue,
		},
		workflows.ThreadWorkflow,
		workflows.ThreadWorkflowInput{InitialMessage: signal},
	)
	if err != nil {
		return orchestrator.Status{}, fmt.Errorf("signal with start thread workflow: %w", err)
	}
	status, err := c.Status(ctx, orchestrator.StatusQuery{Thread: signal.Thread})
	if err != nil {
		return orchestrator.Status{
			Thread:       signal.Thread,
			State:        orchestrator.TaskStateQueued,
			ActiveTaskID: signal.MessageID,
			LastMessage:  signal.Text,
		}, nil
	}
	_ = run
	return status, nil
}

// SignalMessage sends a message signal to an existing workflow.
func (c *Client) SignalMessage(ctx context.Context, signal orchestrator.NewChatMessageSignal) error {
	if err := c.client.SignalWorkflow(ctx, WorkflowID(signal.Thread), "", workflows.SignalNewChatMessage, signal); err != nil {
		return fmt.Errorf("signal thread message: %w", err)
	}
	return nil
}

// Interrupt sends a steering/cancellation signal and returns current status.
func (c *Client) Interrupt(ctx context.Context, signal orchestrator.InterruptSignal) (orchestrator.Status, error) {
	if err := c.client.SignalWorkflow(ctx, WorkflowID(signal.Thread), "", workflows.SignalInterrupt, signal); err != nil {
		return orchestrator.Status{}, fmt.Errorf("signal thread interrupt: %w", err)
	}
	return c.Status(ctx, orchestrator.StatusQuery{Thread: signal.Thread})
}

// UpdateTask sends a task-state patch and returns current status.
func (c *Client) UpdateTask(ctx context.Context, request orchestrator.UpdateTaskRequest) (orchestrator.Status, error) {
	if err := c.client.SignalWorkflow(ctx, WorkflowID(request.Thread), "", workflows.SignalUpdateTask, request); err != nil {
		return orchestrator.Status{}, fmt.Errorf("signal task update: %w", err)
	}
	return c.Status(ctx, orchestrator.StatusQuery{Thread: request.Thread})
}

// Status queries current workflow state.
func (c *Client) Status(ctx context.Context, query orchestrator.StatusQuery) (orchestrator.Status, error) {
	value, err := c.client.QueryWorkflow(ctx, WorkflowID(query.Thread), "", workflows.QueryStatus)
	if err != nil {
		return orchestrator.Status{}, fmt.Errorf("query thread status: %w", err)
	}
	var status orchestrator.Status
	if err := value.Get(&status); err != nil {
		return orchestrator.Status{}, fmt.Errorf("decode thread status: %w", err)
	}
	return status, nil
}

// WorkflowID returns the stable Temporal workflow ID for a thread.
func WorkflowID(thread orchestrator.ThreadKey) string {
	spaceID := strings.TrimSpace(thread.SpaceID)
	if spaceID == "" {
		spaceID = "unknown-space"
	}
	threadID := strings.TrimSpace(thread.ThreadID)
	if threadID == "" {
		threadID = "unknown-thread"
	}
	return "thread/" + sanitizeIDPart(spaceID) + "/" + sanitizeIDPart(threadID)
}

func sanitizeIDPart(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.', r == '/':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "/")
}

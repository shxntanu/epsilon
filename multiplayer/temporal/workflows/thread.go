// Package workflows contains Temporal workflows for epsilond.
package workflows

import (
	"time"

	"github.com/shxntanu/epsilon/multiplayer/orchestrator"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// ThreadWorkflowName is the registered Temporal workflow name for chat threads.
	ThreadWorkflowName = "epsilond.thread"
	// SignalNewChatMessage is sent when a new chat message arrives.
	SignalNewChatMessage = "new_chat_message"
	// SignalInterrupt is sent when a user steers or cancels current work.
	SignalInterrupt = "interrupt"
	// SignalUpdateTask is sent by operators or activities to patch task state.
	SignalUpdateTask = "update_task"
	// QueryStatus returns workflow state for a thread.
	QueryStatus = "status"
)

// ThreadWorkflowInput starts a durable workflow for one chat thread.
type ThreadWorkflowInput struct {
	InitialMessage orchestrator.NewChatMessageSignal
	CarryStatus    orchestrator.Status
}

// ThreadWorkflow owns short-lived orchestration state for one chat thread.
func ThreadWorkflow(ctx workflow.Context, input ThreadWorkflowInput) (orchestrator.Status, error) {
	state := input.CarryStatus
	if state.Thread.ThreadID == "" {
		state = newStatus(input.InitialMessage)
	}
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	if err := workflow.SetQueryHandler(ctx, QueryStatus, func() (orchestrator.Status, error) {
		return state, nil
	}); err != nil {
		return state, err
	}

	state = acceptMessage(state, input.InitialMessage, workflow.Now(ctx))

	messageCh := workflow.GetSignalChannel(ctx, SignalNewChatMessage)
	interruptCh := workflow.GetSignalChannel(ctx, SignalInterrupt)
	updateTaskCh := workflow.GetSignalChannel(ctx, SignalUpdateTask)

	for {
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(messageCh, func(c workflow.ReceiveChannel, more bool) {
			var signal orchestrator.NewChatMessageSignal
			c.Receive(ctx, &signal)
			state = acceptMessage(state, signal, workflow.Now(ctx))
		})
		selector.AddReceive(interruptCh, func(c workflow.ReceiveChannel, more bool) {
			var signal orchestrator.InterruptSignal
			c.Receive(ctx, &signal)
			state = acceptInterrupt(state, signal, workflow.Now(ctx))
		})
		selector.AddReceive(updateTaskCh, func(c workflow.ReceiveChannel, more bool) {
			var request orchestrator.UpdateTaskRequest
			c.Receive(ctx, &request)
			state = acceptTaskUpdate(state, request, workflow.Now(ctx))
		})
		selector.Select(ctx)

		if workflow.GetInfo(ctx).GetCurrentHistoryLength() > 2_000 {
			return state, workflow.NewContinueAsNewError(ctx, ThreadWorkflow, ThreadWorkflowInput{
				CarryStatus: state,
			})
		}
	}
}

func newStatus(signal orchestrator.NewChatMessageSignal) orchestrator.Status {
	return orchestrator.Status{
		Thread:       signal.Thread,
		State:        orchestrator.TaskStateQueued,
		ActiveTaskID: signal.MessageID,
		LastMessage:  signal.Text,
		UpdatedAt:    signal.ReceivedAt,
		Plan: orchestrator.TaskPlan{
			ID:        signal.MessageID,
			Thread:    signal.Thread,
			Goal:      signal.Text,
			State:     orchestrator.TaskStateQueued,
			CreatedAt: signal.ReceivedAt,
			UpdatedAt: signal.ReceivedAt,
		},
	}
}

func acceptMessage(status orchestrator.Status, signal orchestrator.NewChatMessageSignal, now time.Time) orchestrator.Status {
	status.Thread = signal.Thread
	status.State = orchestrator.TaskStateQueued
	status.ActiveTaskID = signal.MessageID
	status.LastMessage = signal.Text
	status.Error = ""
	status.BlockedReason = ""
	status.UpdatedAt = firstTime(signal.ReceivedAt, now)
	status.Plan.ID = signal.MessageID
	status.Plan.Thread = signal.Thread
	status.Plan.Goal = signal.Text
	status.Plan.State = orchestrator.TaskStateQueued
	status.Plan.Subtasks = []orchestrator.Subtask{
		{
			ID:        signal.MessageID,
			Goal:      signal.Text,
			State:     orchestrator.TaskStateQueued,
			Artifacts: signal.Artifacts,
			CreatedAt: status.UpdatedAt,
		},
	}
	if status.Plan.CreatedAt.IsZero() {
		status.Plan.CreatedAt = status.UpdatedAt
	}
	status.Plan.UpdatedAt = status.UpdatedAt
	return status
}

func acceptInterrupt(status orchestrator.Status, signal orchestrator.InterruptSignal, now time.Time) orchestrator.Status {
	status.Thread = signal.Thread
	status.UpdatedAt = firstTime(signal.IssuedAt, now)
	switch signal.Kind {
	case orchestrator.InterruptKindCancel:
		status.State = orchestrator.TaskStateCancelled
	case orchestrator.InterruptKindPause:
		status.State = orchestrator.TaskStateBlocked
		status.BlockedReason = signal.Reason
	case orchestrator.InterruptKindClarify, orchestrator.InterruptKindReplan:
		status.State = orchestrator.TaskStatePlanning
		status.LastMessage = signal.Reason
	default:
		status.State = orchestrator.TaskStatePlanning
	}
	status.Plan.State = status.State
	status.Plan.UpdatedAt = status.UpdatedAt
	return status
}

func acceptTaskUpdate(status orchestrator.Status, request orchestrator.UpdateTaskRequest, now time.Time) orchestrator.Status {
	status.Thread = request.Thread
	status.UpdatedAt = firstTime(request.UpdatedAt, now)
	if request.State != "" {
		status.State = request.State
		status.Plan.State = request.State
	}
	if request.TaskID != "" {
		status.ActiveTaskID = request.TaskID
	}
	status.BlockedReason = request.BlockedReason
	status.Error = request.Error
	for i := range status.Plan.Subtasks {
		if status.Plan.Subtasks[i].ID == request.TaskID {
			status.Plan.Subtasks[i].State = request.State
			status.Plan.Subtasks[i].Result = request.Result
			status.Plan.Subtasks[i].CompletedAt = status.UpdatedAt
			status.Plan.UpdatedAt = status.UpdatedAt
			return status
		}
	}
	if request.TaskID != "" {
		status.Plan.Subtasks = append(status.Plan.Subtasks, orchestrator.Subtask{
			ID:          request.TaskID,
			State:       request.State,
			Result:      request.Result,
			CompletedAt: status.UpdatedAt,
		})
	}
	status.Plan.UpdatedAt = status.UpdatedAt
	return status
}

func firstTime(value time.Time, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

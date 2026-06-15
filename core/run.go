package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

var (
	ErrRunClosed          = errors.New("run closed")
	ErrInvalidMessageRole = errors.New("invalid message role")
	ErrProviderMissing    = errors.New("provider missing")
)

type Run struct {
	id       string
	bus      *events.Bus
	provider Provider
	tools    *tools.Registry
	mu       sync.Mutex
	closed   bool
	messages []types.Message
}

// harness will create a run for the users
func newRun(id string, eventBufferSize int, provider Provider, toolRegistry *tools.Registry) *Run {
	return &Run{
		id:       id,
		bus:      events.NewBus(id, eventBufferSize),
		provider: provider,
		tools:    toolRegistry,
	}
}

func (r *Run) ID() string {
	return r.id
}

func (r *Run) Subscribe() (*events.Subscription, error) {
	return r.bus.Subscribe()
}

// Send is used to send messages to the bus.
// It records state and emits an event.
func (r *Run) Send(ctx context.Context, msg types.Message) error {
	if msg.Role != types.RoleUser {
		return fmt.Errorf("%w: Send only accepts user messages", ErrInvalidMessageRole)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRunClosed
	}

	r.messages = append(r.messages, msg)

	_, err := r.bus.Publish(ctx, types.NewUserMessageAddedEvent(time.Now().UTC(), msg))
	if err != nil {
		return fmt.Errorf("publish user message event: %w", err)
	}

	return nil
}

// Messages returns a snapshot of the messages sent during this run
func (r *Run) Messages() []types.Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	messages := make([]types.Message, len(r.messages))
	copy(messages, r.messages)
	return messages
}

// TODO: Persist a run on disk after a conversation has ended
func (r *Run) Close(ctx context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	defer r.bus.Close()

	_, err := r.bus.Publish(ctx, types.Event{
		Kind:      types.EventRunCompleted,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("publish run completed event: %w", err)
	}

	return nil
}

func (r *Run) Step(ctx context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrRunClosed
	}
	if r.provider == nil {
		r.mu.Unlock()
		return ErrProviderMissing
	}

	messages := make([]types.Message, len(r.messages))
	copy(messages, r.messages)
	provider := r.provider
	toolRegistry := r.tools
	r.mu.Unlock()

	var toolDefs []types.ToolDefinition
	if toolRegistry != nil {
		toolDefs = toolRegistry.Definitions()
	}

	_, err := r.bus.Publish(ctx, types.Event{
		Kind:      types.EventModelStarted,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("publish model started event: %w", err)
	}

	resp, err := provider.Respond(ctx, types.ModelRequest{
		Messages: messages,
		Tools:    toolDefs,
	})
	if err != nil {
		_, _ = r.bus.Publish(ctx, types.NewRunErrorEvent(time.Now().UTC(), err))
		return fmt.Errorf("provider respond: %w", err)
	}

	r.mu.Lock()
	r.messages = append(r.messages, resp.Message)
	r.mu.Unlock()

	_, err = r.bus.Publish(ctx, types.Event{
		Kind:      types.EventModelMessageCompleted,
		CreatedAt: time.Now().UTC(),
		Message:   &resp.Message,
	})
	if err != nil {
		return fmt.Errorf("publish model completed event: %w", err)
	}

	if len(resp.Message.ToolCalls) > 0 {
		if err := r.executeToolCalls(ctx, resp.Message.ToolCalls, toolRegistry); err != nil {
			return err
		}
	}

	return nil
}

func (r *Run) executeToolCalls(ctx context.Context, calls []types.ToolCall, registry *tools.Registry) error {
	for _, call := range calls {
		if _, err := r.bus.Publish(ctx, types.NewToolCallRequestedEvent(time.Now().UTC(), call)); err != nil {
			return fmt.Errorf("publish tool call requested event: %w", err)
		}

		result, err := r.executeToolCall(ctx, call, registry)
		if err != nil {
			return err
		}

		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return ErrRunClosed
		}
		r.messages = append(r.messages, types.ToolMessage(call.ID, call.Name, result))
		r.mu.Unlock()
	}

	return nil
}

func (r *Run) executeToolCall(ctx context.Context, call types.ToolCall,
	registry *tools.Registry) (types.ToolResult, error) {
	if registry == nil {
		result := types.ErrorToolResult("tool registry is not configured")
		if _, err := r.bus.Publish(ctx, types.NewToolCallCompletedEvent(
			time.Now().UTC(), call, result)); err != nil {
			return types.ToolResult{}, fmt.Errorf("publish tool call completed event: %w", err)
		}
		return result, nil
	}

	tool, ok := registry.Get(call.Name)
	if !ok {
		result := types.ErrorToolResult(fmt.Sprintf("tool %q not found", call.Name))
		if _, err := r.bus.Publish(ctx, types.NewToolCallCompletedEvent(
			time.Now().UTC(), call, result)); err != nil {
			return types.ToolResult{}, fmt.Errorf("publish tool call completed event: %w", err)
		}
		return result, nil
	}

	if _, err := r.bus.Publish(ctx, types.NewToolCallStartedEvent(
		time.Now().UTC(), call)); err != nil {
		return types.ToolResult{}, fmt.Errorf("publish tool call started event: %w", err)
	}

	result, err := tool.Run(ctx, call.Input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_, _ = r.bus.Publish(ctx, types.NewRunErrorEvent(time.Now().UTC(), err))
			return types.ToolResult{}, fmt.Errorf("run tool %q: %w", call.Name, err)
		}
		errorResult := types.ErrorToolResult(err.Error())
		result = &errorResult
	}
	if result == nil {
		result = &types.ToolResult{}
	}

	if _, err := r.bus.Publish(ctx, types.NewToolCallCompletedEvent(
		time.Now().UTC(), call, *result)); err != nil {
		return types.ToolResult{}, fmt.Errorf("publish tool call completed event: %w", err)
	}

	return *result, nil
}

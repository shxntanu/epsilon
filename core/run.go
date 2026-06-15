package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/types"
)

var (
	ErrRunClosed          = errors.New("run closed")
	ErrInvalidMessageRole = errors.New("invalid message role")
)

type Run struct {
	id       string
	bus      *events.Bus
	mu       sync.Mutex
	closed   bool
	messages []types.Message
}

// harness will create a run for the users
func newRun(id string, eventBufferSize int) *Run {
	return &Run{
		id:  id,
		bus: events.NewBus(id, eventBufferSize),
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

package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

const defaultBufferSize = 64

var (
	// ErrBusClosed is returned when publishing to or subscribing to a closed bus.
	ErrBusClosed = errors.New("event bus closed")
	// ErrEmptyEventKind is returned when publishing an event without a kind.
	ErrEmptyEventKind = errors.New("event kind is empty")
)

// Bus stamps and broadcasts session events to subscribers.
type Bus struct {
	mu          sync.RWMutex
	sessionID   string
	bufferSize  int
	nextSeq     int64
	nextSubID   uint64
	closed      bool
	subscribers map[uint64]chan types.Event
	store       Store

	// history for replaying events
	history []types.Event
}

// NewBus returns an event bus for one session.
func NewBus(sessionID string, bufferSize int) *Bus {
	return NewBusWithStore(sessionID, bufferSize, nil)
}

// NewBusWithStore returns an event bus for one session with optional persistence.
func NewBusWithStore(sessionID string, bufferSize int, store Store) *Bus {
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}

	return &Bus{
		sessionID:   sessionID,
		bufferSize:  bufferSize,
		subscribers: make(map[uint64]chan types.Event),
		store:       store,
	}
}

// NewBusFromHistory returns an event bus seeded with previously persisted events.
func NewBusFromHistory(sessionID string, bufferSize int, store Store, history []types.Event) *Bus {
	bus := NewBusWithStore(sessionID, bufferSize, store)
	bus.history = make([]types.Event, len(history))
	copy(bus.history, history)

	for _, event := range history {
		if event.Sequence > bus.nextSeq {
			bus.nextSeq = event.Sequence
		}
	}

	return bus
}

// Subscribe registers a new event subscriber.
func (b *Bus) Subscribe() (*Subscription, error) {
	return b.subscribe(true)
}

// SubscribeLive registers a subscriber that only receives events published
// after the subscription is created.
func (b *Bus) SubscribeLive() (*Subscription, error) {
	return b.subscribe(false)
}

func (b *Bus) subscribe(includeHistory bool) (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrBusClosed
	}

	b.nextSubID++
	id := b.nextSubID
	ch := make(chan types.Event, len(b.history)+b.bufferSize)
	if includeHistory {
		for _, event := range b.history {
			ch <- event
		}
	}
	b.subscribers[id] = ch

	return &Subscription{
		bus:    b,
		id:     id,
		events: ch,
	}, nil
}

// History returns a snapshot of the bus history in publish order.
func (b *Bus) History() []types.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	history := make([]types.Event, len(b.history))
	copy(history, b.history)
	return history
}

// Publish stamps an event and broadcasts it to every current subscriber.
func (b *Bus) Publish(ctx context.Context, event types.Event) (types.Event, error) {
	if event.Kind == "" {
		return types.Event{}, ErrEmptyEventKind
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return types.Event{}, ErrBusClosed
	}

	b.nextSeq++
	event.Sequence = b.nextSeq
	event.SessionID = b.sessionID
	if event.ID == "" {
		event.ID = fmt.Sprintf("%s:%d", b.sessionID, event.Sequence)
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	if b.store != nil {
		if err := b.store.Append(ctx, event); err != nil {
			return types.Event{}, fmt.Errorf("persist event: %w", err)
		}
	}

	b.history = append(b.history, event)

	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		case <-ctx.Done():
			return types.Event{}, fmt.Errorf("publish event: %w", ctx.Err())
		}
	}

	return event, nil
}

// Close closes the bus and every subscriber channel.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for id, subscriber := range b.subscribers {
		close(subscriber)
		delete(b.subscribers, id)
	}
}

func (b *Bus) unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subscriber, ok := b.subscribers[id]
	if !ok {
		return
	}

	close(subscriber)
	delete(b.subscribers, id)
}

// Subscription is a caller's view of events published by a bus.
type Subscription struct {
	bus    *Bus
	id     uint64
	events chan types.Event
	once   sync.Once
}

// Events returns the channel of events for this subscription.
func (s *Subscription) Events() <-chan types.Event {
	return s.events
}

// Close unsubscribes from the bus and closes the events channel.
func (s *Subscription) Close() {
	s.once.Do(func() {
		s.bus.unsubscribe(s.id)
	})
}

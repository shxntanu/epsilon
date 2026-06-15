package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

type Harness struct {
	mu               sync.Mutex
	runs             map[string]*Run
	eventBufferSize  int
	newRunID         func() (string, error)
	provider         Provider
	toolRegistry     *tools.Registry
	permissionBroker permissions.Broker
	eventStore       events.Store
}

const defaultEventBufferSize = 64

var ErrEventStoreMissing = errors.New("event store missing")

func New(opts ...Option) (*Harness, error) {
	h := &Harness{
		runs:            make(map[string]*Run),
		eventBufferSize: defaultEventBufferSize,
		newRunID:        randomRunID,
		toolRegistry:    tools.NewRegistry(),
	}

	for _, opt := range opts {
		if err := opt(h); err != nil {
			return nil, fmt.Errorf("apply option: %w", err)
		}
	}

	if h.eventBufferSize <= 0 {
		return nil, fmt.Errorf("event buffer size must be positive")
	}
	if h.newRunID == nil {
		return nil, fmt.Errorf("run ID generator is nil")
	}

	return h, nil
}

func (h *Harness) Start(ctx context.Context) (*Run, error) {
	id, err := h.newRunID()
	if err != nil {
		return nil, fmt.Errorf("create run ID: %w", err)
	}

	run := newRun(id, h.eventBufferSize, h.provider, h.toolRegistry,
		h.permissionBroker, h.eventStore)

	h.mu.Lock()
	h.runs[id] = run
	h.mu.Unlock()

	_, err = run.bus.Publish(ctx, types.NewRunStartedEvent(time.Now().UTC()))
	if err != nil {
		return nil, fmt.Errorf("publish run started event: %w", err)
	}

	return run, nil
}

func (h *Harness) Resume(ctx context.Context, runID string) (*Run, error) {
	h.mu.Lock()
	if run, ok := h.runs[runID]; ok {
		h.mu.Unlock()
		return run, nil
	}
	h.mu.Unlock()

	if h.eventStore == nil {
		return nil, ErrEventStoreMissing
	}

	history, err := h.eventStore.Load(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("load run events: %w", err)
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("run %q has no persisted events", runID)
	}

	run := newRunFromHistory(runID, h.eventBufferSize, h.provider, h.toolRegistry,
		h.permissionBroker, h.eventStore, history)

	h.mu.Lock()
	if existing, ok := h.runs[runID]; ok {
		h.mu.Unlock()
		return existing, nil
	}
	h.runs[runID] = run
	h.mu.Unlock()

	return run, nil
}

func (h *Harness) GetRun(id string) (*Run, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	run, ok := h.runs[id]
	return run, ok
}

func randomRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	return "run_" + hex.EncodeToString(b[:]), nil
}

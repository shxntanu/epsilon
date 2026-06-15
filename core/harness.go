package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

type Harness struct {
	mu              sync.Mutex
	runs            map[string]*Run
	eventBufferSize int
	newRunID        func() (string, error)
	provider        Provider
	toolRegistry    *tools.Registry
}

const defaultEventBufferSize = 64

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

	run := newRun(id, h.eventBufferSize, h.provider, h.toolRegistry)

	h.mu.Lock()
	h.runs[id] = run
	h.mu.Unlock()

	_, err = run.bus.Publish(ctx, types.NewRunStartedEvent(time.Now().UTC()))
	if err != nil {
		return nil, fmt.Errorf("publish run started event: %w", err)
	}

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

package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/session"
	"github.com/shxntanu/epsilon/core/slash"
	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

type Harness struct {
	mu               sync.Mutex
	sessions         map[string]*session.Session
	eventBufferSize  int
	newSessionID     func() (string, error)
	provider         types.Provider
	toolRegistry     *tools.Registry
	slashRegistry    *slash.Registry
	contextTracker   *contextwindow.Tracker
	permissionBroker permissions.Broker
	eventStore       events.Store
}

const defaultEventBufferSize = 64

var ErrEventStoreMissing = errors.New("event store missing")

func New(opts ...Option) (*Harness, error) {
	h := &Harness{
		sessions:        make(map[string]*session.Session),
		eventBufferSize: defaultEventBufferSize,
		newSessionID:    randomSessionID,
		toolRegistry:    tools.NewRegistry(),
		slashRegistry:   slash.NewDefaultRegistry(),
		contextTracker:  contextwindow.DefaultTracker(),
	}

	for _, opt := range opts {
		if err := opt(h); err != nil {
			return nil, fmt.Errorf("apply option: %w", err)
		}
	}

	if h.eventBufferSize <= 0 {
		return nil, fmt.Errorf("event buffer size must be positive")
	}
	if h.newSessionID == nil {
		return nil, fmt.Errorf("session ID generator is nil")
	}

	return h, nil
}

func (h *Harness) StartSession(ctx context.Context) (*session.Session, error) {
	id, err := h.newSessionID()
	if err != nil {
		return nil, fmt.Errorf("create session ID: %w", err)
	}

	sess := session.New(id, h.eventBufferSize, h.provider, h.toolRegistry,
		h.permissionBroker, h.eventStore)

	h.mu.Lock()
	h.sessions[id] = sess
	h.mu.Unlock()

	if err := sess.Start(ctx); err != nil {
		return nil, err
	}

	return sess, nil
}

func (h *Harness) ResumeSession(ctx context.Context, sessionID string) (*session.Session, error) {
	h.mu.Lock()
	if sess, ok := h.sessions[sessionID]; ok {
		h.mu.Unlock()
		return sess, nil
	}
	h.mu.Unlock()

	if h.eventStore == nil {
		return nil, ErrEventStoreMissing
	}

	history, err := h.eventStore.Load(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session events: %w", err)
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("session %q has no persisted events", sessionID)
	}

	sess := session.FromHistory(sessionID, h.eventBufferSize, h.provider, h.toolRegistry,
		h.permissionBroker, h.eventStore, history)

	h.mu.Lock()
	if existing, ok := h.sessions[sessionID]; ok {
		h.mu.Unlock()
		return existing, nil
	}
	h.sessions[sessionID] = sess
	h.mu.Unlock()

	return sess, nil
}

func (h *Harness) GetSession(id string) (*session.Session, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sess, ok := h.sessions[id]
	return sess, ok
}

func (h *Harness) SlashCommands() *slash.Registry {
	return h.slashRegistry
}

func (h *Harness) ContextSummary(sessionID string) (contextwindow.Summary, bool) {
	sess, ok := h.GetSession(sessionID)
	if !ok {
		return contextwindow.Summary{}, false
	}

	return sess.ContextSummary(h.contextTracker), true
}

func randomSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	return "session_" + hex.EncodeToString(b[:]), nil
}

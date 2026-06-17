package tui

import (
	"context"
	"fmt"
	"sync"

	"github.com/shxntanu/epsilon/core/types"
)

// PermissionBroker lets the TUI resolve permission requests from the Bubble Tea loop.
type PermissionBroker struct {
	mu             sync.Mutex
	waiters        map[string]chan types.PermissionResult
	decisions      map[string]types.PermissionResult
	sessionAllows  map[string]types.PermissionResult
}

// NewPermissionBroker returns an interactive broker for TUI sessions.
func NewPermissionBroker() *PermissionBroker {
	return &PermissionBroker{
		waiters:       make(map[string]chan types.PermissionResult),
		decisions:     make(map[string]types.PermissionResult),
		sessionAllows: make(map[string]types.PermissionResult),
	}
}

// CachedDecision returns a session-scoped approval when one has already been
// granted for the requested tool.
func (b *PermissionBroker) CachedDecision(request types.PermissionRequest) (types.PermissionResult, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	result, ok := b.sessionAllows[permissionSessionToolKey(request)]
	if !ok {
		return types.PermissionResult{}, false
	}
	result.Request = request
	return result, true
}

// Decide blocks until the TUI resolves the permission request or ctx is cancelled.
func (b *PermissionBroker) Decide(ctx context.Context,
	request types.PermissionRequest) (types.PermissionResult, error) {
	key := permissionKey(request)

	b.mu.Lock()
	if decision, ok := b.decisions[key]; ok {
		delete(b.decisions, key)
		b.mu.Unlock()
		return decision, nil
	}
	ch := make(chan types.PermissionResult, 1)
	b.waiters[key] = ch
	b.mu.Unlock()

	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		b.mu.Lock()
		delete(b.waiters, key)
		b.mu.Unlock()
		return types.PermissionResult{}, fmt.Errorf("permission decision cancelled: %w", ctx.Err())
	}
}

// Resolve sends a user decision to the pending broker request.
func (b *PermissionBroker) Resolve(request types.PermissionRequest,
	decision types.PermissionDecision, reason string) {
	result := types.PermissionResult{
		Request:  request,
		Decision: decision,
		Reason:   reason,
	}
	key := permissionKey(request)

	b.mu.Lock()
	if decision == types.PermissionDecisionAllowSession {
		b.sessionAllows[permissionSessionToolKey(request)] = result
	}
	waiter, ok := b.waiters[key]
	if ok {
		delete(b.waiters, key)
	}
	if !ok {
		b.decisions[key] = result
	}
	b.mu.Unlock()

	if ok {
		waiter <- result
	}
}

func permissionKey(request types.PermissionRequest) string {
	if request.ToolCallID != "" {
		return request.SessionID + ":" + request.ToolCallID
	}

	return request.SessionID + ":" + request.ToolName + ":" + string(request.Input)
}

func permissionSessionToolKey(request types.PermissionRequest) string {
	return request.SessionID + ":" + request.ToolName
}

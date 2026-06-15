package permissions

import (
	"context"
	"fmt"

	"github.com/shxntanu/epsilon/core/types"
)

// Broker decides whether a permission request should be allowed.
type Broker interface {
	Decide(ctx context.Context, request types.PermissionRequest) (types.PermissionResult, error)
}

// StaticBroker returns the same decision for every request.
type StaticBroker struct {
	decision types.PermissionDecision
	reason   string
}

// NewStaticBroker returns a broker with a fixed decision.
func NewStaticBroker(decision types.PermissionDecision, reason string) *StaticBroker {
	return &StaticBroker{
		decision: decision,
		reason:   reason,
	}
}

// NewAllowBroker returns a broker that allows every request.
func NewAllowBroker() *StaticBroker {
	return NewStaticBroker(types.PermissionDecisionAllow, "allowed by static broker")
}

// NewDenyBroker returns a broker that denies every request.
func NewDenyBroker() *StaticBroker {
	return NewStaticBroker(types.PermissionDecisionDeny, "denied by static broker")
}

// Decide returns the configured static decision.
func (b *StaticBroker) Decide(ctx context.Context, request types.PermissionRequest) (types.PermissionResult, error) {
	select {
	case <-ctx.Done():
		return types.PermissionResult{}, fmt.Errorf("permission decision cancelled: %w", ctx.Err())
	default:
	}

	return types.PermissionResult{
		Request:  request,
		Decision: b.decision,
		Reason:   b.reason,
	}, nil
}

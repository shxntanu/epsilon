package memory

import "context"

// Service searches and writes memories for epsilond.
type Service interface {
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
	Write(ctx context.Context, req WriteRequest) (Memory, error)
}

// Redactor removes secrets or personal data before a memory write reaches storage.
type Redactor interface {
	Redact(ctx context.Context, req WriteRequest) (WriteRequest, error)
}

// RedactorFunc adapts a function into a Redactor.
type RedactorFunc func(ctx context.Context, req WriteRequest) (WriteRequest, error)

// Redact calls f(ctx, req).
func (f RedactorFunc) Redact(ctx context.Context, req WriteRequest) (WriteRequest, error) {
	return f(ctx, req)
}

// PassthroughRedactor returns write requests unchanged.
type PassthroughRedactor struct{}

// Redact returns req without modification.
func (PassthroughRedactor) Redact(ctx context.Context, req WriteRequest) (WriteRequest, error) {
	return req, ctx.Err()
}

// NoopService is a memory service that stores and returns no memories.
type NoopService struct {
	Redactor Redactor
}

// NewNoopService returns a no-op memory service with a passthrough redactor.
func NewNoopService() NoopService {
	return NoopService{Redactor: PassthroughRedactor{}}
}

// Search returns no memory results.
func (s NoopService) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

// Write redacts the request and returns the redacted memory without persisting it.
func (s NoopService) Write(ctx context.Context, req WriteRequest) (Memory, error) {
	if err := ctx.Err(); err != nil {
		return Memory{}, err
	}
	redactor := s.Redactor
	if redactor == nil {
		redactor = PassthroughRedactor{}
	}
	redacted, err := redactor.Redact(ctx, req)
	if err != nil {
		return Memory{}, err
	}
	return memoryFromWriteRequest(redacted), nil
}

func memoryFromWriteRequest(req WriteRequest) Memory {
	mem := req.Memory
	if isZeroScope(mem.Scope) {
		mem.Scope = req.Scope
	}
	if isZeroFact(mem.Fact) {
		mem.Fact = req.Fact
	}
	if mem.Content == "" {
		mem.Content = mem.Fact.Content
	}
	if mem.Content == "" {
		mem.Content = mem.Fact.Object
	}
	if mem.ExpiresAt.IsZero() {
		mem.ExpiresAt = mem.Scope.ExpiresAt
	}
	return mem
}

func isZeroScope(scope Scope) bool {
	return scope.OrgID == "" &&
		scope.SpaceID == "" &&
		scope.TeamID == "" &&
		scope.Repo == "" &&
		scope.UseCase == "" &&
		scope.SourceThreadID == "" &&
		scope.Confidence == 0 &&
		scope.ExpiresAt.IsZero() &&
		scope.Freshness == 0 &&
		len(scope.Labels) == 0
}

func isZeroFact(fact Fact) bool {
	return fact.Subject == "" &&
		fact.Predicate == "" &&
		fact.Object == "" &&
		fact.Content == "" &&
		fact.Confidence == 0 &&
		len(fact.Metadata) == 0
}

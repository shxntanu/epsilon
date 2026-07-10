package memory

import "time"

// Scope identifies where a memory is relevant.
type Scope struct {
	OrgID          string
	SpaceID        string
	TeamID         string
	Repo           string
	UseCase        string
	SourceThreadID string
	Confidence     float64
	ExpiresAt      time.Time
	Freshness      time.Duration
	Labels         map[string]string
}

// Provenance records where a memory came from and when it was observed.
type Provenance struct {
	Source      string
	SourceID    string
	SourceURI   string
	ActorID     string
	ActorName   string
	ObservedAt  time.Time
	ExtractedAt time.Time
}

// Fact is a normalized assertion that can be stored as a memory.
type Fact struct {
	Subject    string
	Predicate  string
	Object     string
	Content    string
	Confidence float64
	Metadata   map[string]string
}

// Memory is a persisted fact with scope, provenance, and lifecycle metadata.
type Memory struct {
	ID         string
	Fact       Fact
	Content    string
	Scope      Scope
	Provenance Provenance
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ExpiresAt  time.Time
	Metadata   map[string]string
}

// SearchRequest describes a scoped memory lookup.
type SearchRequest struct {
	Query          string
	Scope          Scope
	Limit          int
	MinConfidence  float64
	IncludeExpired bool
	Now            time.Time
}

// SearchResult is one matched memory and its retrieval score.
type SearchResult struct {
	Memory Memory
	Score  float64
	Reason string
}

// WriteRequest describes a memory write after the caller has extracted a fact.
type WriteRequest struct {
	Memory Memory
	Scope  Scope
	Fact   Fact
}

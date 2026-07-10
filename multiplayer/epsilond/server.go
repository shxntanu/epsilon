package epsilond

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/shxntanu/epsilon/multiplayer/chat"
)

// Ingestor accepts raw Google Chat events after HTTP-level validation.
type Ingestor interface {
	IngestGoogleChatEvent(ctx context.Context, event RawGoogleChatEvent) (IngestResult, error)
}

// RawGoogleChatEvent is the bounded request body accepted by epsilond.
type RawGoogleChatEvent struct {
	Body       []byte
	Headers    http.Header
	ReceivedAt time.Time
}

// IngestResult describes how epsilond accepted an incoming event.
type IngestResult struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
}

// DependencyChecker reports whether service dependencies are ready.
type DependencyChecker interface {
	Ready(ctx context.Context) error
}

// Server exposes epsilond's HTTP API.
type Server struct {
	cfg      Config
	logger   *slog.Logger
	ingestor Ingestor
	checker  DependencyChecker
	verifier chat.RequestVerifier
}

// ServerOption customizes a Server.
type ServerOption func(*Server)

// WithLogger configures the logger used by the server.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithIngestor configures the Google Chat event ingestor.
func WithIngestor(ingestor Ingestor) ServerOption {
	return func(s *Server) {
		s.ingestor = ingestor
	}
}

// WithDependencyChecker configures readiness checks.
func WithDependencyChecker(checker DependencyChecker) ServerOption {
	return func(s *Server) {
		s.checker = checker
	}
}

// WithRequestVerifier configures platform request verification.
func WithRequestVerifier(verifier chat.RequestVerifier) ServerOption {
	return func(s *Server) {
		s.verifier = verifier
	}
}

// NewServer returns an epsilond HTTP server.
func NewServer(cfg Config, opts ...ServerOption) *Server {
	s := &Server{
		cfg:    cfg,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Handler returns the service HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("POST /google-chat/events", s.handleGoogleChatEvent)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.checker == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.checker.Ready(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGoogleChatEvent(w http.ResponseWriter, r *http.Request) {
	if s.ingestor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "google chat ingestor is not configured",
		})
		return
	}
	if s.verifier != nil {
		if err := s.verifier.Verify(r.Context(), r); err != nil {
			s.logger.Warn("google chat request verification failed", "error", err)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxEventBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": fmt.Sprintf("read request body: %v", err),
		})
		return
	}
	result, err := s.ingestor.IngestGoogleChatEvent(r.Context(), RawGoogleChatEvent{
		Body:       body,
		Headers:    r.Header.Clone(),
		ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		s.logger.Error("ingest google chat event failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !result.Accepted {
		writeJSON(w, http.StatusAccepted, result)
		return
	}
	if result.Message == "" {
		result.Message = "Epsilon accepted the request."
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Default().Warn("write json response failed", "error", err)
	}
}

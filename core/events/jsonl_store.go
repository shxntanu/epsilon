package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

const eventsFileName = "events.jsonl"

// JSONLStore stores each session's events as newline-delimited JSON.
type JSONLStore struct {
	baseDir string
}

// NewJSONLStore returns a file-backed event store rooted at baseDir.
func NewJSONLStore(baseDir string) (*JSONLStore, error) {
	if strings.TrimSpace(baseDir) == "" {
		return nil, fmt.Errorf("event store directory is empty")
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve event store directory: %w", err)
	}

	if err := os.MkdirAll(absBaseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create event store directory: %w", err)
	}

	return &JSONLStore{baseDir: absBaseDir}, nil
}

// Append persists one event.
func (s *JSONLStore) Append(ctx context.Context, event types.Event) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("append event cancelled: %w", ctx.Err())
	default:
	}

	path, err := s.eventsPath(event.SessionID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create session event directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}

	encodeErr := json.NewEncoder(file).Encode(event)
	syncErr := file.Sync()
	closeErr := file.Close()
	if encodeErr != nil {
		if closeErr != nil {
			return fmt.Errorf("encode event: %w; close event log: %w", encodeErr, closeErr)
		}
		return fmt.Errorf("encode event: %w", encodeErr)
	}
	if syncErr != nil {
		if closeErr != nil {
			return fmt.Errorf("sync event log: %w; close event log: %w", syncErr, closeErr)
		}
		return fmt.Errorf("sync event log: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close event log: %w", closeErr)
	}

	return nil
}

// Load returns persisted events for a session in append order.
func (s *JSONLStore) Load(ctx context.Context, sessionID string) ([]types.Event, error) {
	path, err := s.eventsPath(sessionID)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var events []types.Event
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("load events cancelled: %w", ctx.Err())
		default:
		}

		var event types.Event
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return events, nil
			}
			return nil, fmt.Errorf("decode event %d: %w", len(events)+1, err)
		}
		events = append(events, event)
	}
}

// BaseDir returns the absolute event store directory.
func (s *JSONLStore) BaseDir() string {
	return s.baseDir
}

func (s *JSONLStore) eventsPath(sessionID string) (string, error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", err
	}

	return filepath.Join(s.baseDir, sessionID, eventsFileName), nil
}

func validateSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session ID is empty")
	}
	if sessionID == "." || sessionID == ".." {
		return fmt.Errorf("invalid session ID: %q", sessionID)
	}
	if filepath.Base(sessionID) != sessionID {
		return fmt.Errorf("session ID must not contain path separators: %q", sessionID)
	}

	return nil
}

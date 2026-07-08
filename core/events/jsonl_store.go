package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

const eventsFileName = "events.jsonl"
const metadataFileName = "metadata.json"

type sessionMetadata struct {
	Title string `json:"title,omitempty"`
}

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

// ListSessions returns sessions that have persisted event logs.
func (s *JSONLStore) ListSessions(ctx context.Context) ([]SessionInfo, error) {
	entries, err := os.ReadDir(s.baseDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session directory: %w", err)
	}

	sessions := make([]SessionInfo, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("list sessions cancelled: %w", ctx.Err())
		default:
		}
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		if err := validateSessionID(sessionID); err != nil {
			continue
		}

		metadata, err := s.readSessionMetadata(sessionID)
		if err != nil {
			return nil, err
		}
		hasTitle := strings.TrimSpace(metadata.Title) != ""

		path := filepath.Join(s.baseDir, sessionID, eventsFileName)
		info, err := os.Stat(path)
		hasEvents := !errors.Is(err, os.ErrNotExist)
		if err != nil && hasEvents {
			return nil, fmt.Errorf("stat session events: %w", err)
		}
		if hasEvents {
			if ok, err := s.hasConversationEvents(ctx, sessionID); err != nil {
				return nil, err
			} else if !ok && !hasTitle {
				continue
			}
		} else if !hasTitle {
			continue
		}

		updatedAt := time.Now().UTC()
		if hasEvents {
			updatedAt = info.ModTime()
		} else if metaInfo, err := os.Stat(filepath.Join(s.baseDir, sessionID, metadataFileName)); err == nil {
			updatedAt = metaInfo.ModTime()
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat session metadata: %w", err)
		}

		sessions = append(sessions, SessionInfo{
			ID:        sessionID,
			Title:     metadata.Title,
			UpdatedAt: updatedAt,
		})
	}

	sort.SliceStable(sessions, func(i int, j int) bool {
		if !sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
		}
		return sessions[i].ID < sessions[j].ID
	})
	return sessions, nil
}

func (s *JSONLStore) readSessionMetadata(sessionID string) (sessionMetadata, error) {
	metaPath, err := s.metadataPath(sessionID)
	if err != nil {
		return sessionMetadata{}, err
	}

	data, err := os.ReadFile(metaPath)
	if errors.Is(err, os.ErrNotExist) {
		return sessionMetadata{}, nil
	}
	if err != nil {
		return sessionMetadata{}, fmt.Errorf("read session metadata: %w", err)
	}

	var metadata sessionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return sessionMetadata{}, fmt.Errorf("decode session metadata: %w", err)
	}
	return metadata, nil
}

func (s *JSONLStore) hasConversationEvents(ctx context.Context, sessionID string) (bool, error) {
	history, err := s.Load(ctx, sessionID)
	if err != nil {
		return false, err
	}
	for _, event := range history {
		switch event.Kind {
		case types.EventUserMessageAdded, types.EventModelMessageCompleted,
			types.EventToolCallCompleted:
			return true, nil
		}
	}
	return false, nil
}

// SessionExists reports whether a session has events or metadata.
func (s *JSONLStore) SessionExists(ctx context.Context, sessionID string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, fmt.Errorf("check session cancelled: %w", ctx.Err())
	default:
	}

	eventPath, err := s.eventsPath(sessionID)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(eventPath); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat session events: %w", err)
	}

	metaPath, err := s.metadataPath(sessionID)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(metaPath); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat session metadata: %w", err)
	}

	return false, nil
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

func (s *JSONLStore) metadataPath(sessionID string) (string, error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", err
	}
	return filepath.Join(s.baseDir, sessionID, metadataFileName), nil
}

// DeleteSession removes a session directory and all persisted session data.
func (s *JSONLStore) DeleteSession(ctx context.Context, sessionID string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, fmt.Errorf("delete session cancelled: %w", ctx.Err())
	default:
	}

	eventPath, err := s.eventsPath(sessionID)
	if err != nil {
		return false, err
	}
	sessionDir := filepath.Dir(eventPath)
	if _, err := os.Stat(sessionDir); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("stat session directory: %w", err)
	}

	if err := os.RemoveAll(sessionDir); err != nil {
		return false, fmt.Errorf("delete session directory: %w", err)
	}
	return true, nil
}

// RenameSession persists a session title in per-session metadata.
func (s *JSONLStore) RenameSession(ctx context.Context, sessionID string, title string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, fmt.Errorf("rename session cancelled: %w", ctx.Err())
	default:
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return false, fmt.Errorf("title cannot be empty")
	}

	metaPath, err := s.metadataPath(sessionID)
	if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(metaPath), 0o700); err != nil {
		return false, fmt.Errorf("create session metadata dir: %w", err)
	}

	data, err := json.MarshalIndent(sessionMetadata{Title: title}, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode session metadata: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(metaPath), ".metadata-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create metadata temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return false, fmt.Errorf("write metadata temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("close metadata temp file: %w", err)
	}
	if err := os.Rename(tmpName, metaPath); err != nil {
		return false, fmt.Errorf("replace metadata file: %w", err)
	}

	return true, nil
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

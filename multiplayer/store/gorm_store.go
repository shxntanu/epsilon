package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GORMStore persists epsilond multiplayer state using GORM.
type GORMStore struct {
	db *gorm.DB
}

// OpenGORM opens a Postgres-backed GORM store.
func OpenGORM(dsn string) (*GORMStore, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("postgres DSN is empty")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open gorm postgres store: %w", err)
	}
	return NewGORMStore(db), nil
}

// NewGORMStore wraps an existing GORM DB.
func NewGORMStore(db *gorm.DB) *GORMStore {
	return &GORMStore{db: db}
}

// Ready verifies the Postgres connection is reachable.
func (s *GORMStore) Ready(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm store is not configured")
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

// AutoMigrate creates or updates the epsilond multiplayer tables.
func (s *GORMStore) AutoMigrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm store is not configured")
	}
	if err := s.db.WithContext(ctx).AutoMigrate(
		&spaceModel{},
		&threadModel{},
		&chatMessageModel{},
		&chatArtifactModel{},
		&taskModel{},
		&epsilonSessionModel{},
		&workerRunModel{},
		&memoryRecordModel{},
		&usageLedgerModel{},
	); err != nil {
		return fmt.Errorf("auto migrate epsilond store: %w", err)
	}
	return nil
}

// UpsertSpace stores or updates a Google Chat space.
func (s *GORMStore) UpsertSpace(ctx context.Context, space Space) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	model := spaceToModel(space)
	if model.ID == "" {
		model.ID = stableStoreID("space", firstNonEmpty(model.GoogleSpaceID, model.GoogleSpaceName))
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	model.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "google_space_name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"google_space_id", "display_name", "enabled", "budget_policy", "updated_at",
		}),
	}).Create(&model).Error; err != nil {
		return fmt.Errorf("upsert space: %w", err)
	}
	return nil
}

// UpsertThread stores or updates a Google Chat thread.
func (s *GORMStore) UpsertThread(ctx context.Context, thread Thread) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	model := threadToModel(thread)
	if model.ID == "" {
		model.ID = stableStoreID("thread", model.SpaceID, firstNonEmpty(model.GoogleThreadID, model.GoogleThreadName))
	}
	if model.Status == "" {
		model.Status = string(ThreadStatusActive)
	}
	if model.WorkspaceTier == "" {
		model.WorkspaceTier = "standard"
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	model.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "space_id"}, {Name: "google_thread_name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"google_thread_id":     model.GoogleThreadID,
			"status":               model.Status,
			"summary_pointer":      model.SummaryPointer,
			"pinned_facts":         model.PinnedFacts,
			"workspace_tier":       model.WorkspaceTier,
			"temporal_workflow_id": model.TemporalWorkflowID,
			"version":              gorm.Expr("chat_threads.version + 1"),
			"updated_at":           model.UpdatedAt,
		}),
	}).Create(&model).Error; err != nil {
		return fmt.Errorf("upsert thread: %w", err)
	}
	return nil
}

// UpdateThreadState updates thread state only when expectedVersion matches.
func (s *GORMStore) UpdateThreadState(ctx context.Context, thread Thread, expectedVersion int64) (bool, error) {
	if err := s.requireDB(); err != nil {
		return false, err
	}
	if strings.TrimSpace(thread.ID) == "" {
		return false, fmt.Errorf("thread ID is empty")
	}
	if expectedVersion <= 0 {
		return false, fmt.Errorf("expected version must be positive")
	}
	updates := map[string]any{
		"status":          string(thread.Status),
		"summary_pointer": thread.SummaryPointer,
		"pinned_facts":    jsonOrDefault(thread.PinnedFacts, `[]`),
		"workspace_tier":  thread.WorkspaceTier,
		"version":         gorm.Expr("version + 1"),
		"updated_at":      time.Now().UTC(),
	}
	if thread.Status == "" {
		delete(updates, "status")
	}
	if thread.WorkspaceTier == "" {
		delete(updates, "workspace_tier")
	}
	result := s.db.WithContext(ctx).Model(&threadModel{}).
		Where("id = ? AND version = ?", thread.ID, expectedVersion).
		Updates(updates)
	if result.Error != nil {
		return false, fmt.Errorf("update thread state: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// InsertChatMessage stores a message idempotently.
func (s *GORMStore) InsertChatMessage(ctx context.Context, message ChatMessage) (bool, error) {
	if err := s.requireDB(); err != nil {
		return false, err
	}
	model := chatMessageToModel(message)
	if model.ID == "" {
		model.ID = stableStoreID("message", model.SpaceID, firstNonEmpty(model.GoogleEventID, model.GoogleMessageID, model.GoogleMessageName))
	}
	if model.ReceivedAt.IsZero() {
		model.ReceivedAt = time.Now().UTC()
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "space_id"}, {Name: "google_message_name"}},
		DoNothing: true,
	}).Create(&model)
	if result.Error != nil {
		return false, fmt.Errorf("insert chat message: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// InsertChatArtifact stores attachment metadata idempotently by ID.
func (s *GORMStore) InsertChatArtifact(ctx context.Context, artifact ChatArtifact) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	model := chatArtifactToModel(artifact)
	if model.ID == "" {
		model.ID = stableStoreID("artifact", model.MessageID, model.GoogleAttachment, model.FileName, model.ContentHash)
	}
	if model.ExtractionStatus == "" {
		model.ExtractionStatus = string(ArtifactExtractionPending)
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	model.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&model).Error; err != nil {
		return fmt.Errorf("insert chat artifact: %w", err)
	}
	return nil
}

// UpdateChatArtifact updates attachment storage and extraction pointers.
func (s *GORMStore) UpdateChatArtifact(ctx context.Context, artifact ChatArtifact) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	artifact.ID = strings.TrimSpace(artifact.ID)
	if artifact.ID == "" {
		return fmt.Errorf("artifact ID is empty")
	}
	updates := map[string]any{
		"content_hash":      artifact.ContentHash,
		"storage_path":      artifact.StoragePath,
		"extraction_status": string(artifact.ExtractionStatus),
		"metadata":          jsonOrDefault(artifact.Metadata, `{}`),
		"updated_at":        time.Now().UTC(),
	}
	if artifact.ExtractionStatus == "" {
		delete(updates, "extraction_status")
	}
	result := s.db.WithContext(ctx).Model(&chatArtifactModel{}).
		Where("id = ?", artifact.ID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update chat artifact: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update chat artifact: artifact %q not found", artifact.ID)
	}
	return nil
}

// CreateTask stores a task row.
func (s *GORMStore) CreateTask(ctx context.Context, task Task) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	model := taskToModel(task)
	if model.ID == "" {
		model.ID = stableStoreID("task", model.ThreadID, model.Goal, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if model.Status == "" {
		model.Status = string(TaskStatusPending)
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	model.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

// UpdateTaskStatus updates the lifecycle status for a task.
func (s *GORMStore) UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task ID is empty")
	}
	updates := map[string]any{
		"status":     string(status),
		"updated_at": time.Now().UTC(),
	}
	switch status {
	case TaskStatusRunning:
		updates["started_at"] = time.Now().UTC()
	case TaskStatusSucceeded, TaskStatusFailed, TaskStatusCancelled:
		updates["completed_at"] = time.Now().UTC()
	}
	result := s.db.WithContext(ctx).Model(&taskModel{}).Where("id = ?", taskID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update task status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task %q not found", taskID)
	}
	return nil
}

// CreateWorkerRun stores one worker execution record.
func (s *GORMStore) CreateWorkerRun(ctx context.Context, run WorkerRun) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	model := workerRunToModel(run)
	if model.ID == "" {
		model.ID = stableStoreID("worker", model.TaskID, model.WorkerRole, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if model.Status == "" {
		model.Status = string(WorkerRunStatusPending)
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	model.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "model", "sandbox_image", "started_at", "updated_at",
		}),
	}).Create(&model).Error; err != nil {
		return fmt.Errorf("create worker run: %w", err)
	}
	return nil
}

// UpdateWorkerRunStatus updates worker execution state and result pointers.
func (s *GORMStore) UpdateWorkerRunStatus(ctx context.Context, runID string, status WorkerRunStatus, resultPointer string, errMessage string) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("worker run ID is empty")
	}
	updates := map[string]any{
		"status":         string(status),
		"result_pointer": resultPointer,
		"error_message":  errMessage,
		"updated_at":     time.Now().UTC(),
	}
	switch status {
	case WorkerRunStatusRunning:
		updates["started_at"] = time.Now().UTC()
	case WorkerRunStatusSucceeded, WorkerRunStatusFailed, WorkerRunStatusCancelled:
		updates["completed_at"] = time.Now().UTC()
	}
	result := s.db.WithContext(ctx).Model(&workerRunModel{}).Where("id = ?", runID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update worker run status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("worker run %q not found", runID)
	}
	return nil
}

// RecordUsage stores a usage ledger entry.
func (s *GORMStore) RecordUsage(ctx context.Context, entry UsageLedgerEntry) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	model := usageLedgerToModel(entry)
	if model.ID == "" {
		model.ID = stableStoreID("usage", model.ThreadID, model.TaskID, model.Model, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&model).Error; err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	return nil
}

func (s *GORMStore) requireDB() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm store is not configured")
	}
	return nil
}

type spaceModel struct {
	ID              string          `gorm:"primaryKey"`
	GoogleSpaceID   string          `gorm:"uniqueIndex;not null"`
	GoogleSpaceName string          `gorm:"uniqueIndex;not null"`
	DisplayName     string          `gorm:"not null;default:''"`
	Enabled         bool            `gorm:"not null;default:true"`
	BudgetPolicy    json.RawMessage `gorm:"type:jsonb;not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (spaceModel) TableName() string { return "chat_spaces" }

type threadModel struct {
	ID                 string          `gorm:"primaryKey"`
	SpaceID            string          `gorm:"index:idx_threads_space_google_thread_name,unique;not null"`
	GoogleThreadID     string          `gorm:"not null"`
	GoogleThreadName   string          `gorm:"index:idx_threads_space_google_thread_name,unique;not null"`
	Status             string          `gorm:"not null;default:'active'"`
	SummaryPointer     string          `gorm:"not null;default:''"`
	PinnedFacts        json.RawMessage `gorm:"type:jsonb;not null"`
	WorkspaceTier      string          `gorm:"not null;default:'standard'"`
	TemporalWorkflowID string          `gorm:"uniqueIndex;not null"`
	Version            int64           `gorm:"not null;default:1"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (threadModel) TableName() string { return "chat_threads" }

type chatMessageModel struct {
	ID                string          `gorm:"primaryKey"`
	SpaceID           string          `gorm:"index:idx_messages_space_google_message_name,unique;not null"`
	ThreadID          string          `gorm:"index;not null"`
	GoogleMessageID   string          `gorm:"not null"`
	GoogleMessageName string          `gorm:"index:idx_messages_space_google_message_name,unique;not null"`
	GoogleEventID     string          `gorm:"uniqueIndex"`
	SenderID          string          `gorm:"not null;default:''"`
	SenderDisplayName string          `gorm:"not null;default:''"`
	Text              string          `gorm:"not null;default:''"`
	ArgumentText      string          `gorm:"not null;default:''"`
	MentionedEpsilon  bool            `gorm:"not null;default:false"`
	RawEvent          json.RawMessage `gorm:"type:jsonb;not null"`
	GoogleCreateTime  time.Time
	ReceivedAt        time.Time
	CreatedAt         time.Time
}

func (chatMessageModel) TableName() string { return "chat_messages" }

type chatArtifactModel struct {
	ID               string          `gorm:"primaryKey"`
	MessageID        string          `gorm:"index;not null"`
	SpaceID          string          `gorm:"index;not null"`
	ThreadID         string          `gorm:"index;not null"`
	GoogleAttachment string          `gorm:"not null;default:''"`
	FileName         string          `gorm:"not null;default:''"`
	MimeType         string          `gorm:"not null;default:''"`
	ContentHash      string          `gorm:"not null;default:''"`
	StoragePath      string          `gorm:"not null;default:''"`
	ExtractionStatus string          `gorm:"not null;default:'pending'"`
	Metadata         json.RawMessage `gorm:"type:jsonb;not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (chatArtifactModel) TableName() string { return "chat_artifacts" }

type taskModel struct {
	ID              string          `gorm:"primaryKey"`
	ThreadID        string          `gorm:"index;not null"`
	SpaceID         string          `gorm:"index;not null"`
	ParentTaskID    string          `gorm:"index"`
	Status          string          `gorm:"index;not null;default:'pending'"`
	Goal            string          `gorm:"not null"`
	Plan            json.RawMessage `gorm:"type:jsonb;not null"`
	ResultPointer   string          `gorm:"not null;default:''"`
	Budget          json.RawMessage `gorm:"type:jsonb;not null"`
	CancellationGen int64           `gorm:"not null;default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartedAt       time.Time
	CompletedAt     time.Time
}

func (taskModel) TableName() string { return "tasks" }

type epsilonSessionModel struct {
	ID               string `gorm:"primaryKey"`
	SpaceID          string `gorm:"index;not null"`
	ThreadID         string `gorm:"index;not null"`
	TaskID           string `gorm:"index"`
	HarnessSessionID string `gorm:"uniqueIndex;not null"`
	EventLogPath     string `gorm:"not null;default:''"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (epsilonSessionModel) TableName() string { return "epsilon_sessions" }

type workerRunModel struct {
	ID            string `gorm:"primaryKey"`
	TaskID        string `gorm:"index;not null"`
	ThreadID      string `gorm:"index;not null"`
	WorkerRole    string `gorm:"not null;default:''"`
	Model         string `gorm:"not null;default:''"`
	Status        string `gorm:"index;not null;default:'pending'"`
	SandboxImage  string `gorm:"not null;default:''"`
	ResultPointer string `gorm:"not null;default:''"`
	ExitCode      *int
	ErrorMessage  string `gorm:"not null;default:''"`
	InputTokens   int64  `gorm:"not null;default:0"`
	OutputTokens  int64  `gorm:"not null;default:0"`
	EstimatedCost string `gorm:"not null;default:'0'"`
	StartedAt     time.Time
	CompletedAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (workerRunModel) TableName() string { return "worker_runs" }

type memoryRecordModel struct {
	ID         string          `gorm:"primaryKey"`
	SpaceID    string          `gorm:"index"`
	ThreadID   string          `gorm:"index"`
	TaskID     string          `gorm:"index"`
	Mem0ID     string          `gorm:"uniqueIndex;not null"`
	Scope      json.RawMessage `gorm:"type:jsonb;not null"`
	Provenance json.RawMessage `gorm:"type:jsonb;not null"`
	Confidence float64
	ExpiresAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (memoryRecordModel) TableName() string { return "memory_records" }

type usageLedgerModel struct {
	ID            string          `gorm:"primaryKey"`
	SpaceID       string          `gorm:"index"`
	ThreadID      string          `gorm:"index"`
	TaskID        string          `gorm:"index"`
	WorkerRunID   string          `gorm:"index"`
	Provider      string          `gorm:"not null;default:''"`
	Model         string          `gorm:"not null"`
	Reason        string          `gorm:"not null;default:''"`
	InputTokens   int64           `gorm:"not null;default:0"`
	OutputTokens  int64           `gorm:"not null;default:0"`
	EstimatedCost string          `gorm:"not null;default:'0'"`
	Metadata      json.RawMessage `gorm:"type:jsonb;not null"`
	CreatedAt     time.Time
}

func (usageLedgerModel) TableName() string { return "usage_ledger" }

func spaceToModel(space Space) spaceModel {
	return spaceModel{
		ID:              space.ID,
		GoogleSpaceID:   space.GoogleSpaceID,
		GoogleSpaceName: space.GoogleSpaceName,
		DisplayName:     space.DisplayName,
		Enabled:         space.Enabled,
		BudgetPolicy:    jsonOrDefault(space.BudgetPolicy, `{}`),
		CreatedAt:       space.CreatedAt,
		UpdatedAt:       space.UpdatedAt,
	}
}

func threadToModel(thread Thread) threadModel {
	return threadModel{
		ID:                 thread.ID,
		SpaceID:            thread.SpaceID,
		GoogleThreadID:     thread.GoogleThreadID,
		GoogleThreadName:   thread.GoogleThreadName,
		Status:             string(thread.Status),
		SummaryPointer:     thread.SummaryPointer,
		PinnedFacts:        jsonOrDefault(thread.PinnedFacts, `[]`),
		WorkspaceTier:      thread.WorkspaceTier,
		TemporalWorkflowID: thread.TemporalWorkflowID,
		Version:            thread.Version,
		CreatedAt:          thread.CreatedAt,
		UpdatedAt:          thread.UpdatedAt,
	}
}

func chatMessageToModel(message ChatMessage) chatMessageModel {
	return chatMessageModel{
		ID:                message.ID,
		SpaceID:           message.SpaceID,
		ThreadID:          message.ThreadID,
		GoogleMessageID:   message.GoogleMessageID,
		GoogleMessageName: message.GoogleMessageName,
		GoogleEventID:     message.GoogleEventID,
		SenderID:          message.SenderID,
		SenderDisplayName: message.SenderDisplayName,
		Text:              message.Text,
		ArgumentText:      message.ArgumentText,
		MentionedEpsilon:  message.MentionedEpsilon,
		RawEvent:          jsonOrDefault(message.RawEvent, `{}`),
		GoogleCreateTime:  message.GoogleCreateTime,
		ReceivedAt:        message.ReceivedAt,
		CreatedAt:         message.CreatedAt,
	}
}

func taskToModel(task Task) taskModel {
	return taskModel{
		ID:              task.ID,
		ThreadID:        task.ThreadID,
		SpaceID:         task.SpaceID,
		ParentTaskID:    task.ParentTaskID,
		Status:          string(task.Status),
		Goal:            task.Goal,
		Plan:            jsonOrDefault(task.Plan, `{}`),
		ResultPointer:   task.ResultPointer,
		Budget:          jsonOrDefault(task.Budget, `{}`),
		CancellationGen: task.CancellationGen,
		CreatedAt:       task.CreatedAt,
		UpdatedAt:       task.UpdatedAt,
		StartedAt:       task.StartedAt,
		CompletedAt:     task.CompletedAt,
	}
}

func chatArtifactToModel(artifact ChatArtifact) chatArtifactModel {
	return chatArtifactModel{
		ID:               artifact.ID,
		MessageID:        artifact.MessageID,
		SpaceID:          artifact.SpaceID,
		ThreadID:         artifact.ThreadID,
		GoogleAttachment: artifact.GoogleAttachment,
		FileName:         artifact.FileName,
		MimeType:         artifact.MimeType,
		ContentHash:      artifact.ContentHash,
		StoragePath:      artifact.StoragePath,
		ExtractionStatus: string(artifact.ExtractionStatus),
		Metadata:         jsonOrDefault(artifact.Metadata, `{}`),
		CreatedAt:        artifact.CreatedAt,
		UpdatedAt:        artifact.UpdatedAt,
	}
}

func workerRunToModel(run WorkerRun) workerRunModel {
	return workerRunModel{
		ID:            run.ID,
		TaskID:        run.TaskID,
		ThreadID:      run.ThreadID,
		WorkerRole:    run.WorkerRole,
		Model:         run.Model,
		Status:        string(run.Status),
		SandboxImage:  run.SandboxImage,
		ResultPointer: run.ResultPointer,
		ExitCode:      run.ExitCode,
		ErrorMessage:  run.ErrorMessage,
		InputTokens:   run.InputTokens,
		OutputTokens:  run.OutputTokens,
		EstimatedCost: run.EstimatedCost,
		StartedAt:     run.StartedAt,
		CompletedAt:   run.CompletedAt,
		CreatedAt:     run.CreatedAt,
		UpdatedAt:     run.UpdatedAt,
	}
}

func usageLedgerToModel(entry UsageLedgerEntry) usageLedgerModel {
	return usageLedgerModel{
		ID:            entry.ID,
		SpaceID:       entry.SpaceID,
		ThreadID:      entry.ThreadID,
		TaskID:        entry.TaskID,
		WorkerRunID:   entry.WorkerRunID,
		Provider:      entry.Provider,
		Model:         entry.Model,
		Reason:        entry.Reason,
		InputTokens:   entry.InputTokens,
		OutputTokens:  entry.OutputTokens,
		EstimatedCost: entry.EstimatedCost,
		Metadata:      jsonOrDefault(entry.Metadata, `{}`),
		CreatedAt:     entry.CreatedAt,
	}
}

func jsonOrDefault(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}

func stableStoreID(parts ...string) string {
	value := strings.Join(parts, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:32]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

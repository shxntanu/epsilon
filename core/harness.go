package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	harnessconfig "github.com/shxntanu/epsilon/core/config"
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
	selectedModel    *types.ModelInfo
	model            string
	effort           string
	permissionBroker permissions.Broker
	eventStore       events.Store
	configStore      *harnessconfig.Store
	configSettings   harnessconfig.Settings
}

const defaultEventBufferSize = 64

var ErrEventStoreMissing = errors.New("event store missing")
var ErrProviderModelCatalogMissing = errors.New("provider model catalog missing")

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
	if h.model == "" {
		if selected, ok := h.provider.(types.SelectedModelProvider); ok {
			h.model = strings.TrimSpace(selected.SelectedModel())
		}
	}

	return h, nil
}

func (h *Harness) StartSession(ctx context.Context) (*session.Session, error) {
	id, err := h.newSessionID()
	if err != nil {
		return nil, fmt.Errorf("create session ID: %w", err)
	}

	sess := session.New(id, h.eventBufferSize, h.provider, h.modelRequestSettings, h.toolRegistry,
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

	sess := session.FromHistory(sessionID, h.eventBufferSize, h.provider,
		h.modelRequestSettings, h.toolRegistry,
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

func (h *Harness) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	catalog, ok := h.provider.(types.ModelCatalogProvider)
	if !ok {
		return nil, ErrProviderModelCatalogMissing
	}
	return catalog.ListModels(ctx)
}

func (h *Harness) SelectedModelInfo(ctx context.Context) (types.ModelInfo, bool, error) {
	if h.selectedModel != nil {
		return *h.selectedModel, true, nil
	}

	models, err := h.ListModels(ctx)
	if err != nil {
		return types.ModelInfo{}, false, err
	}

	selectedID := h.CurrentModel()
	for _, model := range models {
		if modelMatchesSelected(model, selectedID) {
			return model, true, nil
		}
	}
	return types.ModelInfo{}, false, nil
}

func (h *Harness) CachedSelectedModelInfo() (types.ModelInfo, bool) {
	if h.selectedModel == nil {
		return types.ModelInfo{}, false
	}
	return *h.selectedModel, true
}

func (h *Harness) CurrentModel() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.model
}

func (h *Harness) CurrentEffort() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.effort
}

func (h *Harness) SetModel(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model cannot be empty")
	}

	h.mu.Lock()
	h.model = model
	h.selectedModel = nil
	h.configSettings.Model = model
	h.mu.Unlock()

	h.refreshSelectedModel(ctx)
	return h.saveSessionDefaults()
}

func (h *Harness) SetEffort(effort string) error {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "default" || effort == "none" || effort == "off" {
		effort = ""
	}

	h.mu.Lock()
	h.effort = effort
	h.configSettings.Effort = effort
	h.mu.Unlock()

	return h.saveSessionDefaults()
}

func (h *Harness) modelRequestSettings() types.ModelRequestSettings {
	h.mu.Lock()
	defer h.mu.Unlock()
	return types.ModelRequestSettings{
		Model:  h.model,
		Effort: h.effort,
	}
}

func (h *Harness) refreshSelectedModel(ctx context.Context) {
	modelInfo, ok, err := h.SelectedModelInfo(ctx)
	if err != nil || !ok {
		return
	}
	h.mu.Lock()
	h.selectedModel = &modelInfo
	h.contextTracker = contextwindow.NewTracker(
		contextwindow.ConfigFromModelInfo(modelInfo, contextwindow.Config{}))
	h.mu.Unlock()
}

func (h *Harness) saveSessionDefaults() error {
	h.mu.Lock()
	store := h.configStore
	settings := h.configSettings
	h.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.SaveSessionDefaults(settings)
}

func modelMatchesSelected(model types.ModelInfo, selected string) bool {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return false
	}
	values := []string{model.ID, model.Name, model.ProviderModel}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == selected || strings.TrimPrefix(value, model.Provider+"/") == selected ||
			strings.TrimPrefix(selected, model.Provider+"/") == value {
			return true
		}
	}
	return false
}

func randomSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	return "session_" + hex.EncodeToString(b[:]), nil
}

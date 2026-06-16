// Package config stores persisted harness configuration and provider metadata.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

const (
	DefaultPath         = ".epsilon/config.json"
	DefaultSessionDir   = ".epsilon/sessions"
	DefaultWorkspace    = "."
	DefaultProvider     = "fake"
	DefaultLiteLLMURL   = "http://localhost:4000"
	DefaultLiteLLMModel = "gpt-5.4"
)

// Settings is the subset of harness configuration that can be persisted.
type Settings struct {
	SessionDir      string `json:"session_dir,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	Effort          string `json:"effort,omitempty"`
	LiteLLMBaseURL  string `json:"litellm_base_url,omitempty"`
	EventBufferSize int    `json:"event_buffer_size,omitempty"`
}

// Config is the persisted harness config file.
type Config struct {
	Defaults        Settings      `json:"defaults"`
	SessionDefaults Settings      `json:"session_defaults"`
	ProviderCache   ProviderCache `json:"provider_cache"`
}

// ProviderCache stores provider metadata that is expensive to fetch.
type ProviderCache struct {
	ModelCatalogs map[string]CachedModelCatalog `json:"model_catalogs,omitempty"`
}

// CachedModelCatalog is a cached list of provider model metadata.
type CachedModelCatalog struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Models    []types.ModelInfo `json:"models"`
}

// Default returns the built-in harness config.
func Default() Config {
	return Config{
		Defaults: Settings{
			SessionDir:     DefaultSessionDir,
			Workspace:      DefaultWorkspace,
			Provider:       DefaultProvider,
			Model:          DefaultLiteLLMModel,
			LiteLLMBaseURL: DefaultLiteLLMURL,
		},
	}
}

// EffectiveSettings overlays persisted defaults and session defaults over the
// built-in defaults.
func (c Config) EffectiveSettings() Settings {
	settings := Default().Defaults
	settings = overlaySettings(settings, c.Defaults)
	settings = overlaySettings(settings, c.SessionDefaults)
	return settings
}

func overlaySettings(base Settings, override Settings) Settings {
	if strings.TrimSpace(override.SessionDir) != "" {
		base.SessionDir = override.SessionDir
	}
	if strings.TrimSpace(override.Workspace) != "" {
		base.Workspace = override.Workspace
	}
	if strings.TrimSpace(override.Provider) != "" {
		base.Provider = override.Provider
	}
	if strings.TrimSpace(override.Model) != "" {
		base.Model = override.Model
	}
	if strings.TrimSpace(override.Effort) != "" {
		base.Effort = override.Effort
	}
	if strings.TrimSpace(override.LiteLLMBaseURL) != "" {
		base.LiteLLMBaseURL = override.LiteLLMBaseURL
	}
	if override.EventBufferSize > 0 {
		base.EventBufferSize = override.EventBufferSize
	}
	return base
}

// Store provides synchronized access to a harness config file.
type Store struct {
	mu     sync.Mutex
	path   string
	config Config
}

// LoadStore loads a config store from path, or creates an in-memory default if
// the file does not exist yet.
func LoadStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultPath
	}

	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read harness config: %w", err)
		}
	} else if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("decode harness config: %w", err)
		}
		cfg.Defaults = overlaySettings(Default().Defaults, cfg.Defaults)
	}
	ensureProviderCache(&cfg)

	return &Store{path: path, config: cfg}, nil
}

// Config returns a copy of the current config.
func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(s.config)
}

// EffectiveSettings returns the current effective settings.
func (s *Store) EffectiveSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config.EffectiveSettings()
}

// SaveSessionDefaults stores defaults that should carry into following
// sessions.
func (s *Store) SaveSessionDefaults(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config.SessionDefaults = sanitizeSettings(settings)
	return s.saveLocked()
}

// CachedModels returns a cached model catalog for key.
func (s *Store) CachedModels(key string) ([]types.ModelInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.config.ProviderCache.ModelCatalogs[key]
	if !ok || len(entry.Models) == 0 {
		return nil, false
	}
	return cloneModels(entry.Models), true
}

// SaveModels caches a provider model catalog for key.
func (s *Store) SaveModels(key string, models []types.ModelInfo) error {
	if strings.TrimSpace(key) == "" || len(models) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ensureProviderCache(&s.config)
	s.config.ProviderCache.ModelCatalogs[key] = CachedModelCatalog{
		FetchedAt: time.Now().UTC(),
		Models:    cloneModels(models),
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create harness config dir: %w", err)
	}

	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode harness config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create harness config temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write harness config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close harness config temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace harness config: %w", err)
	}
	return nil
}

func sanitizeSettings(settings Settings) Settings {
	settings.SessionDir = strings.TrimSpace(settings.SessionDir)
	settings.Workspace = strings.TrimSpace(settings.Workspace)
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.Model = strings.TrimSpace(settings.Model)
	settings.Effort = strings.TrimSpace(settings.Effort)
	settings.LiteLLMBaseURL = strings.TrimRight(strings.TrimSpace(settings.LiteLLMBaseURL), "/")
	if settings.EventBufferSize < 0 {
		settings.EventBufferSize = 0
	}
	return settings
}

func ensureProviderCache(cfg *Config) {
	if cfg.ProviderCache.ModelCatalogs == nil {
		cfg.ProviderCache.ModelCatalogs = make(map[string]CachedModelCatalog)
	}
}

func cloneConfig(cfg Config) Config {
	cfg.ProviderCache.ModelCatalogs = cloneCatalogs(cfg.ProviderCache.ModelCatalogs)
	return cfg
}

func cloneCatalogs(catalogs map[string]CachedModelCatalog) map[string]CachedModelCatalog {
	if catalogs == nil {
		return nil
	}
	cloned := make(map[string]CachedModelCatalog, len(catalogs))
	for key, catalog := range catalogs {
		catalog.Models = cloneModels(catalog.Models)
		cloned[key] = catalog
	}
	return cloned
}

func cloneModels(models []types.ModelInfo) []types.ModelInfo {
	cloned := make([]types.ModelInfo, len(models))
	copy(cloned, models)
	return cloned
}

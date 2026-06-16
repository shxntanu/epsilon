package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestStorePersistsSessionDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if err := store.SaveSessionDefaults(Settings{
		SessionDir:     ".epsilon/custom-sessions",
		Workspace:      "/tmp/work",
		Provider:       "litellm",
		Model:          "openai/gpt-4o",
		LiteLLMBaseURL: "http://litellm.test/",
	}); err != nil {
		t.Fatalf("save session defaults: %v", err)
	}

	reloaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	settings := reloaded.EffectiveSettings()
	if settings.Provider != "litellm" {
		t.Fatalf("provider = %q, want litellm", settings.Provider)
	}
	if settings.Model != "openai/gpt-4o" {
		t.Fatalf("model = %q, want openai/gpt-4o", settings.Model)
	}
	if settings.LiteLLMBaseURL != "http://litellm.test" {
		t.Fatalf("base url = %q, want trimmed url", settings.LiteLLMBaseURL)
	}
}

func TestCachedProviderUsesPersistedCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	key := ModelCatalogKey("litellm", "http://litellm.test")
	if err := store.SaveModels(key, []types.ModelInfo{{ID: "cached-model"}}); err != nil {
		t.Fatalf("save models: %v", err)
	}

	underlying := &countingCatalogProvider{
		models: []types.ModelInfo{{ID: "network-model"}},
	}
	provider := WrapProvider(underlying, store, key)

	models, err := provider.(types.ModelCatalogProvider).ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "cached-model" {
		t.Fatalf("models = %#v, want cached model", models)
	}
	if underlying.calls != 0 {
		t.Fatalf("network calls = %d, want 0", underlying.calls)
	}
}

type countingCatalogProvider struct {
	models []types.ModelInfo
	calls  int
}

func (p *countingCatalogProvider) Respond(context.Context,
	types.ModelRequest) (*types.ModelResponse, error) {
	return &types.ModelResponse{}, nil
}

func (p *countingCatalogProvider) ListModels(context.Context) ([]types.ModelInfo, error) {
	p.calls++
	return p.models, nil
}

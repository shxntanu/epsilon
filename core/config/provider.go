package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

// CachedProvider wraps a provider with persisted model-catalog caching.
type CachedProvider struct {
	provider types.Provider
	store    *Store
	key      string
}

// WrapProvider returns a provider that serves model catalog metadata from the
// config cache when possible.
func WrapProvider(provider types.Provider, store *Store, key string) types.Provider {
	if provider == nil || store == nil || strings.TrimSpace(key) == "" {
		return provider
	}
	return &CachedProvider{
		provider: provider,
		store:    store,
		key:      key,
	}
}

// ModelCatalogKey returns a stable cache key for provider metadata.
func ModelCatalogKey(providerName string, endpoint string) string {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return providerName
	}
	return providerName + "|" + endpoint
}

func (p *CachedProvider) Respond(ctx context.Context,
	req types.ModelRequest) (*types.ModelResponse, error) {
	return p.provider.Respond(ctx, req)
}

func (p *CachedProvider) StreamRespond(ctx context.Context, req types.ModelRequest,
	emit func(types.ModelDelta) error) (*types.ModelResponse, error) {
	streamer, ok := p.provider.(types.StreamingProvider)
	if !ok {
		return p.provider.Respond(ctx, req)
	}
	return streamer.StreamRespond(ctx, req, emit)
}

func (p *CachedProvider) SelectedModel() string {
	selected, ok := p.provider.(types.SelectedModelProvider)
	if !ok {
		return ""
	}
	return selected.SelectedModel()
}

func (p *CachedProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	if models, ok := p.store.CachedModels(p.key); ok {
		return models, nil
	}

	catalog, ok := p.provider.(types.ModelCatalogProvider)
	if !ok {
		return nil, fmt.Errorf("wrapped provider does not support model catalog")
	}
	models, err := catalog.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	if err := p.store.SaveModels(p.key, models); err != nil {
		return nil, err
	}
	return models, nil
}

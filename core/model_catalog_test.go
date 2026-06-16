package core

import (
	"context"
	"errors"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

type catalogProvider struct {
	models []types.ModelInfo
}

func (p catalogProvider) Respond(context.Context, types.ModelRequest) (*types.ModelResponse, error) {
	return &types.ModelResponse{}, nil
}

func (p catalogProvider) ListModels(context.Context) ([]types.ModelInfo, error) {
	return p.models, nil
}

func (p catalogProvider) SelectedModel() string {
	return "openai/model-b"
}

func TestHarnessListModels(t *testing.T) {
	harness, err := New(WithProvider(catalogProvider{
		models: []types.ModelInfo{
			{ID: "model-a"},
			{ID: "model-b", Provider: "openai", ProviderModel: "openai/model-b", MaxInputTokens: 128000},
		},
	}))
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	models, err := harness.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %d, want 2", len(models))
	}

	selected, ok, err := harness.SelectedModelInfo(context.Background())
	if err != nil {
		t.Fatalf("selected model info: %v", err)
	}
	if !ok {
		t.Fatalf("expected selected model info")
	}
	if selected.ID != "model-b" || selected.MaxInputTokens != 128000 {
		t.Fatalf("selected = %#v, want model-b metadata", selected)
	}
}

func TestHarnessListModelsUnsupported(t *testing.T) {
	harness, err := New()
	if err != nil {
		t.Fatalf("new harness: %v", err)
	}

	_, err = harness.ListModels(context.Background())
	if !errors.Is(err, ErrProviderModelCatalogMissing) {
		t.Fatalf("err = %v, want ErrProviderModelCatalogMissing", err)
	}
}

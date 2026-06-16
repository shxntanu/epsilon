package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestListModelsUsesModelInfo(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/model/info" {
			t.Fatalf("path = %s, want /model/info", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q, want bearer key", got)
		}
		return jsonResponse(http.StatusOK, `{
			"data": [{
				"model_name": "gpt-4o",
				"litellm_params": {"model": "openai/gpt-4o"},
				"model_info": {
					"mode": "chat",
					"max_input_tokens": 128000,
					"max_output_tokens": 16384,
					"input_cost_per_token": 0.0000025,
					"output_cost_per_token": 0.00001,
					"cache_read_input_token_cost": 0.00000125,
					"cache_creation_input_token_cost": 0.00000375
				}
			}]
		}`), nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		APIKey:     "test-key",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	model := models[0]
	if model.ID != "gpt-4o" {
		t.Fatalf("id = %q, want gpt-4o", model.ID)
	}
	if model.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", model.Provider)
	}
	if model.ProviderModel != "openai/gpt-4o" {
		t.Fatalf("provider model = %q, want openai/gpt-4o", model.ProviderModel)
	}
	if model.MaxInputTokens != 128000 {
		t.Fatalf("max input = %d, want 128000", model.MaxInputTokens)
	}
	if model.MaxOutputTokens != 16384 {
		t.Fatalf("max output = %d, want 16384", model.MaxOutputTokens)
	}
	if model.Pricing.InputCostPer1KTokens != 0.0025 {
		t.Fatalf("input cost per 1k = %f, want 0.0025", model.Pricing.InputCostPer1KTokens)
	}
}

func TestListModelsFallsBackToOpenAIModels(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/model/info":
			return jsonResponse(http.StatusNotFound, "missing endpoint"), nil
		case "/v1/models":
			return jsonResponse(http.StatusOK, `{"data":[{"id":"gpt-4o-mini"}]}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return nil, nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	if models[0].ID != "gpt-4o-mini" {
		t.Fatalf("id = %q, want gpt-4o-mini", models[0].ID)
	}
	if models[0].MaxInputTokens != 0 {
		t.Fatalf("fallback max input = %d, want 0", models[0].MaxInputTokens)
	}
}

func TestRespondUsesRequestModelAndEffort(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-4o" {
			t.Fatalf("model = %v, want gpt-4o", payload["model"])
		}
		if payload["reasoning_effort"] != "high" {
			t.Fatalf("reasoning_effort = %v, want high", payload["reasoning_effort"])
		}

		return jsonResponse(http.StatusOK, `{
			"choices": [{"message": {"role": "assistant", "content": "ok"}}]
		}`), nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		Model:      "default-model",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = provider.Respond(context.Background(), types.ModelRequest{
		Model:    "gpt-4o",
		Effort:   "high",
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

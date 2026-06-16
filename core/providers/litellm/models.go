package litellm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

const maxModelMetadataResponseBytes = 8 * 1024 * 1024

func (p *Provider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	models, err := p.listModelInfo(ctx)
	if err == nil {
		return models, nil
	}

	fallback, fallbackErr := p.listOpenAIModels(ctx)
	if fallbackErr != nil {
		return nil, fmt.Errorf("list litellm model info: %w; fallback /v1/models: %v",
			err, fallbackErr)
	}
	return fallback, nil
}

func (p *Provider) listModelInfo(ctx context.Context) ([]types.ModelInfo, error) {
	body, err := p.getJSON(ctx, "/model/info")
	if err != nil {
		return nil, err
	}

	var decoded modelInfoResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode litellm model info: %w", err)
	}

	entries := decoded.entries()
	models := make([]types.ModelInfo, 0, len(entries))
	for _, entry := range entries {
		model := modelInfoFromEntry(entry)
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		models = append(models, model)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("litellm model info response contained no models")
	}

	return models, nil
}

func (p *Provider) listOpenAIModels(ctx context.Context) ([]types.ModelInfo, error) {
	body, err := p.getJSON(ctx, "/v1/models")
	if err != nil {
		return nil, err
	}

	var decoded openAIModelsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode litellm models: %w", err)
	}

	models := make([]types.ModelInfo, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		models = append(models, types.ModelInfo{
			ID:       item.ID,
			Name:     item.ID,
			Provider: "litellm",
		})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("litellm models response contained no models")
	}

	return models, nil
}

func (p *Provider) getJSON(ctx context.Context, path string) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create litellm metadata request: %w", err)
	}
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send litellm metadata request: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, maxModelMetadataResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read litellm metadata response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("litellm metadata request %s failed: status %d: %s",
			path, httpResp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

type modelInfoResponse struct {
	Data      []modelInfoEntry `json:"data"`
	ModelInfo []modelInfoEntry `json:"model_info"`
}

func (r modelInfoResponse) entries() []modelInfoEntry {
	if len(r.Data) > 0 {
		return r.Data
	}
	return r.ModelInfo
}

type modelInfoEntry struct {
	ModelName     string         `json:"model_name"`
	ModelID       string         `json:"model_id"`
	LiteLLMParams map[string]any `json:"litellm_params"`
	ModelInfo     map[string]any `json:"model_info"`
}

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func modelInfoFromEntry(entry modelInfoEntry) types.ModelInfo {
	info := entry.ModelInfo
	params := entry.LiteLLMParams

	id := firstString(entry.ModelName, entry.ModelID, stringValue(info, "id"),
		stringValue(info, "model_name"), stringValue(params, "model"))
	name := firstString(entry.ModelName, stringValue(info, "display_name"), id)
	provider := firstString(stringValue(info, "provider"), providerFromModelName(stringValue(params, "model")))

	model := types.ModelInfo{
		ID:              id,
		Name:            name,
		Provider:        provider,
		Mode:            stringValue(info, "mode"),
		MaxInputTokens:  intValue(info, "max_input_tokens", "input_cost_per_token_limit"),
		MaxOutputTokens: intValue(info, "max_output_tokens"),
		MaxTokens:       intValue(info, "max_tokens", "context_window", "context_length"),
		Pricing: types.ModelPricing{
			InputCostPerToken:      floatValue(info, "input_cost_per_token"),
			OutputCostPerToken:     floatValue(info, "output_cost_per_token"),
			CacheReadCostPerToken:  floatValue(info, "cache_read_input_token_cost", "cache_read_cost_per_token"),
			CacheWriteCostPerToken: floatValue(info, "cache_creation_input_token_cost", "cache_write_cost_per_token"),
			ReasoningCostPerToken:  floatValue(info, "output_cost_per_reasoning_token", "reasoning_cost_per_token"),
		},
	}
	model.Pricing.InputCostPer1KTokens = model.Pricing.InputCostPerToken * 1000
	model.Pricing.OutputCostPer1KTokens = model.Pricing.OutputCostPerToken * 1000
	model.Pricing.CacheReadCostPer1KToken = model.Pricing.CacheReadCostPerToken * 1000
	model.Pricing.CacheWriteCostPer1KToken = model.Pricing.CacheWriteCostPerToken * 1000

	if model.MaxInputTokens == 0 {
		model.MaxInputTokens = model.MaxTokens
	}
	if model.MaxTokens == 0 {
		model.MaxTokens = model.MaxInputTokens
	}
	return model
}

func firstString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case json.Number:
			return typed.String()
		}
	}
	return ""
}

func intValue(values map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case json.Number:
			asInt, err := typed.Int64()
			if err == nil {
				return int(asInt)
			}
		case string:
			asInt, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return asInt
			}
		}
	}
	return 0
}

func floatValue(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed
		case json.Number:
			asFloat, err := typed.Float64()
			if err == nil {
				return asFloat
			}
		case string:
			asFloat, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			if err == nil {
				return asFloat
			}
		}
	}
	return 0
}

func providerFromModelName(model string) string {
	provider, _, ok := strings.Cut(model, "/")
	if ok {
		return provider
	}
	return ""
}

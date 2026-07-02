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

func TestRespondUsesResponsesAPIForGPT55ToolsAndEffort(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-5.5" {
			t.Fatalf("model = %v, want gpt-5.5", payload["model"])
		}
		if _, ok := payload["reasoning_effort"]; ok {
			t.Fatalf("reasoning_effort should not be sent to responses payload")
		}
		reasoning, ok := payload["reasoning"].(map[string]any)
		if !ok {
			t.Fatalf("reasoning missing or wrong type: %#v", payload["reasoning"])
		}
		if reasoning["effort"] != "high" {
			t.Fatalf("reasoning effort = %v, want high", reasoning["effort"])
		}
		tools, ok := payload["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one tool", payload["tools"])
		}
		tool := tools[0].(map[string]any)
		if tool["type"] != "function" || tool["name"] != "read_file" {
			t.Fatalf("tool = %#v, want responses function tool", tool)
		}

		return jsonResponse(http.StatusOK, `{
			"output": [
				{"type": "message", "role": "assistant", "content": [
					{"type": "output_text", "text": "checking"}
				]},
				{"type": "function_call", "call_id": "call_1", "name": "read_file", "arguments": "{\"path\":\"README.md\"}"}
			],
			"usage": {"input_tokens": 10, "output_tokens": 3, "total_tokens": 13}
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

	resp, err := provider.Respond(context.Background(), types.ModelRequest{
		Model:    "gpt-5.5",
		Effort:   "high",
		Messages: []types.Message{types.UserMessage("hello")},
		Tools: []types.ToolDefinition{{
			Name:        "read_file",
			Description: "read a file",
		}},
	})
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if resp.Message.Content[0].Text != "checking" {
		t.Fatalf("message = %q, want checking", resp.Message.Content[0].Text)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "read_file" {
		t.Fatalf("call = %#v, want read_file call", call)
	}
	if string(call.Input) != `{"path":"README.md"}` {
		t.Fatalf("call input = %s, want README path", call.Input)
	}
	if resp.Usage != (types.Usage{InputTokens: 10, OutputTokens: 3, TotalTokens: 13}) {
		t.Fatalf("usage = %#v, want responses usage", resp.Usage)
	}
}

func TestStreamRespondRequestsAndParsesUsage(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["stream"] != true {
			t.Fatalf("stream = %v, want true", payload["stream"])
		}
		options, ok := payload["stream_options"].(map[string]any)
		if !ok {
			t.Fatalf("stream_options missing or wrong type: %#v", payload["stream_options"])
		}
		if options["include_usage"] != true {
			t.Fatalf("include_usage = %v, want true", options["include_usage"])
		}

		body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hel\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":5,\"total_tokens\":17}}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		Model:      "default-model",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	var streamed string
	resp, err := provider.StreamRespond(context.Background(), types.ModelRequest{
		Messages: []types.Message{types.UserMessage("hello")},
	}, func(delta types.ModelDelta) error {
		streamed += delta.Text
		return nil
	})
	if err != nil {
		t.Fatalf("stream respond: %v", err)
	}
	if streamed != "hello" {
		t.Fatalf("streamed = %q, want hello", streamed)
	}
	if resp.Message.Content[0].Text != "hello" {
		t.Fatalf("message = %q, want hello", resp.Message.Content[0].Text)
	}
	want := types.Usage{InputTokens: 12, OutputTokens: 5, TotalTokens: 17}
	if resp.Usage != want {
		t.Fatalf("usage = %#v, want %#v", resp.Usage, want)
	}
}

func TestStreamRespondUsesResponsesAPIForGPT55ToolsAndEffort(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["stream"] != true {
			t.Fatalf("stream = %v, want true", payload["stream"])
		}
		if _, ok := payload["reasoning_effort"]; ok {
			t.Fatalf("reasoning_effort should not be sent to responses payload")
		}
		reasoning, ok := payload["reasoning"].(map[string]any)
		if !ok || reasoning["effort"] != "medium" {
			t.Fatalf("reasoning = %#v, want medium effort", payload["reasoning"])
		}

		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hel\"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n" +
			"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"list_dir\",\"arguments\":\"{}\"}}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"output\":[" +
			"{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}," +
			"{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"list_dir\",\"arguments\":\"{}\"}" +
			"],\"usage\":{\"input_tokens\":7,\"output_tokens\":4,\"total_tokens\":11}}}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	var streamed string
	resp, err := provider.StreamRespond(context.Background(), types.ModelRequest{
		Model:    "openai/gpt-5.5",
		Effort:   "medium",
		Messages: []types.Message{types.UserMessage("hello")},
		Tools: []types.ToolDefinition{{
			Name:        "list_dir",
			Description: "list a directory",
		}},
	}, func(delta types.ModelDelta) error {
		streamed += delta.Text
		return nil
	})
	if err != nil {
		t.Fatalf("stream respond: %v", err)
	}
	if streamed != "hello" {
		t.Fatalf("streamed = %q, want hello", streamed)
	}
	if resp.Message.Content[0].Text != "hello" {
		t.Fatalf("message = %q, want hello", resp.Message.Content[0].Text)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Name != "list_dir" {
		t.Fatalf("tool calls = %#v, want list_dir call", resp.Message.ToolCalls)
	}
	if resp.Usage != (types.Usage{InputTokens: 7, OutputTokens: 4, TotalTokens: 11}) {
		t.Fatalf("usage = %#v, want responses stream usage", resp.Usage)
	}
}

func TestStreamRespondMergesRepeatedToolCallSnapshots(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"list_dir\",\"arguments\":\"{\\\"path\\\":\\\".\\\"}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"list_dir\",\"arguments\":\"{\\\"path\\\":\\\".\\\"}\"}}]}}]}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := provider.StreamRespond(context.Background(), types.ModelRequest{
		Messages: []types.Message{types.UserMessage("hello")},
	}, nil)
	if err != nil {
		t.Fatalf("stream respond: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "list_dir" {
		t.Fatalf("tool name = %q, want list_dir", call.Name)
	}
	if string(call.Input) != `{"path":"."}` {
		t.Fatalf("tool input = %s, want path object", call.Input)
	}
}

func TestStreamRespondMergesRepeatedToolCallSnapshotsWithoutIndex(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"list_dir\",\"arguments\":\"{\\\"path\\\":\\\".\\\"}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"list_dir\",\"arguments\":\"{\\\"path\\\":\\\".\\\"}\"}}]}}]}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := provider.StreamRespond(context.Background(), types.ModelRequest{
		Messages: []types.Message{types.UserMessage("hello")},
	}, nil)
	if err != nil {
		t.Fatalf("stream respond: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "list_dir" {
		t.Fatalf("tool name = %q, want list_dir", call.Name)
	}
}

func TestStreamRespondSeparatesMultipleToolCallsWithoutIndex(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[" +
			"{\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"list_dir\",\"arguments\":\"{\\\"path\\\":\\\".\\\"}\"}}," +
			"{\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"git_status\",\"arguments\":\"{}\"}}," +
			"{\"id\":\"call_3\",\"type\":\"function\",\"function\":{\"name\":\"ripgrep\",\"arguments\":\"{\\\"pattern\\\":\\\"TODO\\\"}\"}}" +
			"]}}]}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := provider.StreamRespond(context.Background(), types.ModelRequest{
		Messages: []types.Message{types.UserMessage("hello")},
	}, nil)
	if err != nil {
		t.Fatalf("stream respond: %v", err)
	}
	if len(resp.Message.ToolCalls) != 3 {
		t.Fatalf("tool calls = %#v, want 3 separate calls", resp.Message.ToolCalls)
	}
	for i, want := range []string{"list_dir", "git_status", "ripgrep"} {
		if resp.Message.ToolCalls[i].Name != want {
			t.Fatalf("tool call %d name = %q, want %q", i,
				resp.Message.ToolCalls[i].Name, want)
		}
	}
}

func TestStreamRespondMergesFragmentedToolCallArguments(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"list\",\"arguments\":\"{\\\"path\\\"\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"_dir\",\"arguments\":\":\\\".\\\"}\"}}]}}]}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	provider, err := New(Config{
		BaseURL:    "http://litellm.test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := provider.StreamRespond(context.Background(), types.ModelRequest{
		Messages: []types.Message{types.UserMessage("hello")},
	}, nil)
	if err != nil {
		t.Fatalf("stream respond: %v", err)
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "list_dir" {
		t.Fatalf("tool name = %q, want list_dir", call.Name)
	}
	if string(call.Input) != `{"path":"."}` {
		t.Fatalf("tool input = %s, want path object", call.Input)
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

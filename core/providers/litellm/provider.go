package litellm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shxntanu/epsilon/core/types"
)

const (
	defaultBaseURL = "http://localhost:4000"
	defaultModel   = "gpt-5.4"
)

var emptyObjectSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

// Config configures a LiteLLM proxy provider.
type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// Provider adapts a LiteLLM proxy to the harness provider interface.
type Provider struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// New returns a LiteLLM provider.
func New(config Config) (*Provider, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	model := strings.TrimSpace(config.Model)
	if model == "" {
		model = defaultModel
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 2 * time.Minute,
		}
	}

	return &Provider{
		baseURL:    baseURL,
		apiKey:     config.APIKey,
		model:      model,
		httpClient: httpClient,
	}, nil
}

func (p *Provider) SelectedModel() string {
	return p.model
}

// Respond sends a chat completion request to the LiteLLM proxy.
func (p *Provider) Respond(ctx context.Context,
	req types.ModelRequest) (*types.ModelResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if shouldUseResponsesAPI(model, req) {
		return p.respondResponses(ctx, model, req)
	}
	payload := chatCompletionRequest{
		Model:           model,
		Messages:        convertMessages(req.Messages),
		Tools:           convertTools(req.Tools),
		ReasoningEffort: strings.TrimSpace(req.Effort),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode litellm request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create litellm request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send litellm request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read litellm response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("litellm request failed: status %d: %s",
			httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode litellm response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("litellm response has no choices")
	}

	message := convertResponseMessage(decoded.Choices[0].Message)
	return &types.ModelResponse{
		Message: message,
		Usage: types.Usage{
			InputTokens:  decoded.Usage.PromptTokens,
			OutputTokens: decoded.Usage.CompletionTokens,
			TotalTokens:  decoded.Usage.TotalTokens,
		},
	}, nil
}

type chatCompletionRequest struct {
	Model           string             `json:"model"`
	Messages        []chatMessage      `json:"messages"`
	Tools           []chatTool         `json:"tools,omitempty"`
	Stream          bool               `json:"stream,omitempty"`
	StreamOptions   *chatStreamOptions `json:"stream_options,omitempty"`
	ReasoningEffort string             `json:"reasoning_effort,omitempty"`
}

type chatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Index    *int             `json:"index,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function chatCallFunction `json:"function"`
}

type chatCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type chatCompletionStreamResponse struct {
	Choices []struct {
		Delta chatStreamDelta `json:"delta"`
	} `json:"choices"`
	Usage chatUsage `json:"usage,omitempty"`
}

type chatStreamDelta struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type responsesRequest struct {
	Model     string               `json:"model"`
	Input     []responsesInputItem `json:"input"`
	Tools     []responsesTool      `json:"tools,omitempty"`
	Stream    bool                 `json:"stream,omitempty"`
	Reasoning *responsesReasoning  `json:"reasoning,omitempty"`
}

type responsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type responsesInputItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type responsesResponse struct {
	Output []responsesOutputItem `json:"output"`
	Usage  responsesUsage        `json:"usage,omitempty"`
}

type responsesOutputItem struct {
	Type      string                   `json:"type"`
	Role      string                   `json:"role,omitempty"`
	Content   []responsesOutputContent `json:"content,omitempty"`
	CallID    string                   `json:"call_id,omitempty"`
	Name      string                   `json:"name,omitempty"`
	Arguments string                   `json:"arguments,omitempty"`
}

type responsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type responsesStreamEvent struct {
	Type     string              `json:"type"`
	Delta    string              `json:"delta,omitempty"`
	Item     responsesOutputItem `json:"item,omitempty"`
	Response responsesResponse   `json:"response,omitempty"`
}

// StreamRespond sends a streaming chat completion request to the LiteLLM proxy.
func (p *Provider) StreamRespond(ctx context.Context, req types.ModelRequest,
	emit func(types.ModelDelta) error) (*types.ModelResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if shouldUseResponsesAPI(model, req) {
		return p.streamRespondResponses(ctx, model, req, emit)
	}
	payload := chatCompletionRequest{
		Model:           model,
		Messages:        convertMessages(req.Messages),
		Tools:           convertTools(req.Tools),
		Stream:          true,
		StreamOptions:   &chatStreamOptions{IncludeUsage: true},
		ReasoningEffort: strings.TrimSpace(req.Effort),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode litellm stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create litellm stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send litellm stream request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 4*1024*1024))
		if err != nil {
			return nil, fmt.Errorf("read litellm stream error response: %w", err)
		}
		return nil, fmt.Errorf("litellm stream request failed: status %d: %s",
			httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	message, usage, err := readStreamResponse(ctx, httpResp.Body, emit)
	if err != nil {
		return nil, err
	}

	return &types.ModelResponse{
		Message: message,
		Usage:   usage,
	}, nil
}

func (p *Provider) respondResponses(ctx context.Context, model string,
	req types.ModelRequest) (*types.ModelResponse, error) {
	payload := responsesRequest{
		Model:     model,
		Input:     convertResponsesInput(req.Messages),
		Tools:     convertResponsesTools(req.Tools),
		Reasoning: responsesReasoningFromEffort(req.Effort),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode litellm responses request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create litellm responses request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send litellm responses request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read litellm responses response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("litellm responses request failed: status %d: %s",
			httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded responsesResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode litellm responses response: %w", err)
	}

	return &types.ModelResponse{
		Message: convertResponsesMessage(decoded.Output),
		Usage:   convertResponsesUsage(decoded.Usage),
	}, nil
}

func (p *Provider) streamRespondResponses(ctx context.Context, model string,
	req types.ModelRequest, emit func(types.ModelDelta) error) (*types.ModelResponse, error) {
	payload := responsesRequest{
		Model:     model,
		Input:     convertResponsesInput(req.Messages),
		Tools:     convertResponsesTools(req.Tools),
		Stream:    true,
		Reasoning: responsesReasoningFromEffort(req.Effort),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode litellm responses stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create litellm responses stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send litellm responses stream request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 4*1024*1024))
		if err != nil {
			return nil, fmt.Errorf("read litellm responses stream error response: %w", err)
		}
		return nil, fmt.Errorf("litellm responses stream request failed: status %d: %s",
			httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	message, usage, err := readResponsesStreamResponse(ctx, httpResp.Body, emit)
	if err != nil {
		return nil, err
	}

	return &types.ModelResponse{
		Message: message,
		Usage:   usage,
	}, nil
}

func readResponsesStreamResponse(ctx context.Context, body io.Reader,
	emit func(types.ModelDelta) error) (types.Message, types.Usage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var content strings.Builder
	var output []responsesOutputItem
	var completed *responsesResponse

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return types.Message{}, types.Usage{}, fmt.Errorf("litellm responses stream cancelled: %w", err)
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return types.Message{}, types.Usage{},
				fmt.Errorf("decode litellm responses stream event: %w", err)
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta == "" {
				continue
			}
			content.WriteString(event.Delta)
			if emit != nil {
				if err := emit(types.ModelDelta{Text: event.Delta}); err != nil {
					return types.Message{}, types.Usage{}, err
				}
			}
		case "response.output_item.done":
			if event.Item.Type != "" {
				output = append(output, event.Item)
			}
		case "response.completed":
			completed = &event.Response
		}
	}
	if err := scanner.Err(); err != nil {
		return types.Message{}, types.Usage{}, fmt.Errorf("read litellm responses stream: %w", err)
	}

	if completed != nil {
		return convertResponsesMessage(completed.Output),
			convertResponsesUsage(completed.Usage), nil
	}
	if content.Len() > 0 {
		output = append([]responsesOutputItem{{
			Type: "message",
			Role: string(types.RoleAssistant),
			Content: []responsesOutputContent{{
				Type: "output_text",
				Text: content.String(),
			}},
		}}, output...)
	}

	return convertResponsesMessage(output), types.Usage{}, nil
}

func readStreamResponse(ctx context.Context, body io.Reader,
	emit func(types.ModelDelta) error) (types.Message, types.Usage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var content strings.Builder
	var toolCalls []chatToolCall
	var usage types.Usage

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return types.Message{}, types.Usage{}, fmt.Errorf("litellm stream cancelled: %w", err)
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var chunk chatCompletionStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return types.Message{}, types.Usage{}, fmt.Errorf("decode litellm stream chunk: %w", err)
		}
		if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 ||
			chunk.Usage.TotalTokens != 0 {
			usage = types.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				TotalTokens:  chunk.Usage.TotalTokens,
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if emit != nil {
				if err := emit(types.ModelDelta{Text: delta.Content}); err != nil {
					return types.Message{}, types.Usage{}, err
				}
			}
		}
		toolCalls = mergeStreamToolCalls(toolCalls, delta.ToolCalls)
	}
	if err := scanner.Err(); err != nil {
		return types.Message{}, types.Usage{}, fmt.Errorf("read litellm stream: %w", err)
	}

	return types.Message{
		Role:      types.RoleAssistant,
		Content:   []types.ContentPart{types.TextPart(content.String())},
		ToolCalls: convertResponseToolCalls(toolCalls),
	}, usage, nil
}

func mergeStreamToolCalls(existing []chatToolCall, deltas []chatToolCall) []chatToolCall {
	for _, delta := range deltas {
		index := streamToolCallIndex(existing, delta)
		for len(existing) <= index {
			existing = append(existing, chatToolCall{Index: intPtr(len(existing))})
		}

		call := &existing[index]
		if call.Index == nil {
			call.Index = intPtr(index)
		}
		if delta.ID != "" {
			call.ID = delta.ID
		}
		if delta.Type != "" {
			call.Type = delta.Type
		}
		if delta.Function.Name != "" {
			call.Function.Name = mergeStreamField(call.Function.Name, delta.Function.Name)
		}
		if delta.Function.Arguments != "" {
			call.Function.Arguments = mergeStreamField(call.Function.Arguments,
				delta.Function.Arguments)
		}
	}

	return existing
}

func streamToolCallIndex(existing []chatToolCall, delta chatToolCall) int {
	if delta.ID != "" {
		for i, call := range existing {
			if call.ID == delta.ID {
				return i
			}
		}
	}

	if delta.Index != nil {
		index := *delta.Index
		if index >= 0 && index < len(existing) && !canMergeStreamToolCall(existing[index], delta) {
			return len(existing)
		}
		if index >= 0 {
			return index
		}
	}

	if len(existing) == 0 {
		return 0
	}

	last := len(existing) - 1
	if canMergeStreamToolCall(existing[last], delta) {
		return last
	}
	return len(existing)
}

func canMergeStreamToolCall(existing chatToolCall, delta chatToolCall) bool {
	if existing.ID != "" && delta.ID != "" && existing.ID != delta.ID {
		return false
	}
	if !canMergeStreamField(existing.Function.Name, delta.Function.Name) {
		return false
	}
	return canMergeStreamArguments(existing.Function.Arguments, delta.Function.Arguments)
}

func canMergeStreamField(existing string, delta string) bool {
	if existing == "" || delta == "" {
		return true
	}
	if strings.HasPrefix(existing, delta) || strings.HasPrefix(delta, existing) {
		return true
	}
	return !json.Valid([]byte(existing)) && strings.HasPrefix(delta, "_")
}

func canMergeStreamArguments(existing string, delta string) bool {
	if existing == "" || delta == "" {
		return true
	}
	if strings.HasPrefix(existing, delta) || strings.HasPrefix(delta, existing) {
		return true
	}
	if json.Valid([]byte(existing)) {
		return false
	}
	trimmedDelta := strings.TrimSpace(delta)
	return !strings.HasPrefix(trimmedDelta, "{") && !strings.HasPrefix(trimmedDelta, "[")
}

func mergeStreamField(existing string, delta string) string {
	if existing == "" {
		return delta
	}
	if delta == "" || strings.HasPrefix(existing, delta) {
		return existing
	}
	if strings.HasPrefix(delta, existing) {
		return delta
	}
	return existing + delta
}

func intPtr(value int) *int {
	return &value
}

func shouldUseResponsesAPI(model string, req types.ModelRequest) bool {
	effort := strings.TrimSpace(req.Effort)
	if effort == "" || len(req.Tools) == 0 {
		return false
	}
	model = strings.TrimSpace(model)
	if provider, name, ok := strings.Cut(model, "/"); ok && provider == "openai" {
		model = name
	}
	return model == "gpt-5.5" || strings.HasPrefix(model, "gpt-5.5-")
}

func responsesReasoningFromEffort(effort string) *responsesReasoning {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return nil
	}
	return &responsesReasoning{Effort: effort}
}

func convertMessages(messages []types.Message) []chatMessage {
	converted := make([]chatMessage, 0, len(messages))
	for _, msg := range messages {
		converted = append(converted, chatMessage{
			Role:       string(msg.Role),
			Content:    textFromContent(msg.Content),
			ToolCalls:  convertToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		})
	}

	return converted
}

func convertResponsesInput(messages []types.Message) []responsesInputItem {
	converted := make([]responsesInputItem, 0, len(messages))
	for _, msg := range messages {
		content := textFromContent(msg.Content)
		switch msg.Role {
		case types.RoleTool:
			converted = append(converted, responsesInputItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: content,
			})
		case types.RoleAssistant:
			if strings.TrimSpace(content) != "" {
				converted = append(converted, responsesInputItem{
					Role:    string(types.RoleAssistant),
					Content: content,
				})
			}
			for _, call := range msg.ToolCalls {
				converted = append(converted, responsesInputItem{
					Type:      "function_call",
					CallID:    call.ID,
					Name:      call.Name,
					Arguments: string(call.Input),
				})
			}
		default:
			converted = append(converted, responsesInputItem{
				Role:    string(msg.Role),
				Content: content,
			})
		}
	}

	return converted
}

func convertTools(defs []types.ToolDefinition) []chatTool {
	if len(defs) == 0 {
		return nil
	}

	converted := make([]chatTool, 0, len(defs))
	for _, def := range defs {
		parameters := def.InputSchema
		if len(parameters) == 0 {
			parameters = emptyObjectSchema
		}

		converted = append(converted, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  parameters,
			},
		})
	}

	return converted
}

func convertResponsesTools(defs []types.ToolDefinition) []responsesTool {
	if len(defs) == 0 {
		return nil
	}

	converted := make([]responsesTool, 0, len(defs))
	for _, def := range defs {
		parameters := def.InputSchema
		if len(parameters) == 0 {
			parameters = emptyObjectSchema
		}

		converted = append(converted, responsesTool{
			Type:        "function",
			Name:        def.Name,
			Description: def.Description,
			Parameters:  parameters,
		})
	}

	return converted
}

func convertToolCalls(calls []types.ToolCall) []chatToolCall {
	if len(calls) == 0 {
		return nil
	}

	converted := make([]chatToolCall, 0, len(calls))
	for _, call := range calls {
		converted = append(converted, chatToolCall{
			ID:   call.ID,
			Type: "function",
			Function: chatCallFunction{
				Name:      call.Name,
				Arguments: string(call.Input),
			},
		})
	}

	return converted
}

func convertResponseMessage(msg chatMessage) types.Message {
	return types.Message{
		Role:       types.Role(msg.Role),
		Content:    []types.ContentPart{types.TextPart(msg.Content)},
		ToolCalls:  convertResponseToolCalls(msg.ToolCalls),
		ToolCallID: msg.ToolCallID,
	}
}

func convertResponsesMessage(output []responsesOutputItem) types.Message {
	var content strings.Builder
	var calls []types.ToolCall
	for _, item := range output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Text == "" {
					continue
				}
				if content.Len() > 0 {
					content.WriteString("\n")
				}
				content.WriteString(part.Text)
			}
		case "function_call":
			input := json.RawMessage(strings.TrimSpace(item.Arguments))
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			} else if !json.Valid(input) {
				input = json.RawMessage(`{"_invalid_input":true}`)
			}
			calls = append(calls, types.ToolCall{
				ID:    item.CallID,
				Name:  item.Name,
				Input: input,
			})
		}
	}

	return types.Message{
		Role:      types.RoleAssistant,
		Content:   []types.ContentPart{types.TextPart(content.String())},
		ToolCalls: calls,
	}
}

func convertResponsesUsage(usage responsesUsage) types.Usage {
	return types.Usage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
	}
}

func convertResponseToolCalls(calls []chatToolCall) []types.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	converted := make([]types.ToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		input := json.RawMessage(strings.TrimSpace(call.Function.Arguments))
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		} else if !json.Valid(input) {
			input = json.RawMessage(`{"_invalid_input":true}`)
		}

		converted = append(converted, types.ToolCall{
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: input,
		})
	}

	return converted
}

func textFromContent(content []types.ContentPart) string {
	var b strings.Builder
	for _, part := range content {
		if part.Kind != types.ContentText {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(part.Text)
	}

	return b.String()
}

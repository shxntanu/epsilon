package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

var (
	ErrClosed             = errors.New("session closed")
	ErrInvalidMessageRole = errors.New("invalid message role")
	ErrProviderMissing    = errors.New("provider missing")
)

type Session struct {
	id              string
	bus             *events.Bus
	provider        types.Provider
	requestSettings func() types.ModelRequestSettings
	tools           *tools.Registry
	broker          permissions.Broker
	mu              sync.Mutex
	closed          bool
	messages        []types.Message
}

// New returns a session ready to accept messages.
func New(id string, eventBufferSize int, provider types.Provider,
	requestSettings func() types.ModelRequestSettings, toolRegistry *tools.Registry,
	broker permissions.Broker, eventStore events.Store) *Session {
	return &Session{
		id:              id,
		bus:             events.NewBusWithStore(id, eventBufferSize, eventStore),
		provider:        provider,
		requestSettings: requestSettings,
		tools:           toolRegistry,
		broker:          broker,
	}
}

// FromHistory returns a session restored from persisted events.
func FromHistory(id string, eventBufferSize int, provider types.Provider,
	requestSettings func() types.ModelRequestSettings,
	toolRegistry *tools.Registry, broker permissions.Broker, eventStore events.Store,
	history []types.Event) *Session {
	return &Session{
		id:              id,
		bus:             events.NewBusFromHistory(id, eventBufferSize, eventStore, history),
		provider:        provider,
		requestSettings: requestSettings,
		tools:           toolRegistry,
		broker:          broker,
		closed:          eventHistoryClosed(history),
		messages:        messagesFromEvents(history),
	}
}

// ID returns the stable session identifier.
func (s *Session) ID() string {
	return s.id
}

// Start initializes the session. Session creation stays in memory until the
// first user/model/tool event so empty sessions are not persisted.
func (s *Session) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	return nil
}

// Subscribe subscribes to session events.
func (s *Session) Subscribe() (*events.Subscription, error) {
	return s.bus.Subscribe()
}

// SubscribeLive subscribes to future events without replaying session history.
func (s *Session) SubscribeLive() (*events.Subscription, error) {
	return s.bus.SubscribeLive()
}

// History returns the session event history known to the session bus.
func (s *Session) History() []types.Event {
	return s.bus.History()
}

func messagesFromEvents(history []types.Event) []types.Message {
	messages := make([]types.Message, 0)
	for _, event := range history {
		switch event.Kind {
		case types.EventUserMessageAdded, types.EventModelMessageCompleted:
			if event.Message != nil {
				messages = append(messages, *event.Message)
			}
		case types.EventToolCallCompleted:
			if event.ToolCall != nil && event.ToolResult != nil {
				messages = append(messages, types.ToolMessage(
					event.ToolCall.ID,
					event.ToolCall.Name,
					*event.ToolResult,
				))
			}
		}
	}

	return messages
}

func eventHistoryClosed(history []types.Event) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Kind == types.EventSessionCompleted {
			return true
		}
	}

	return false
}

// Send is used to send messages to the bus.
// It records state and emits an event.
func (s *Session) Send(ctx context.Context, msg types.Message) error {
	if msg.Role != types.RoleUser {
		return fmt.Errorf("%w: Send only accepts user messages", ErrInvalidMessageRole)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrClosed
	}

	s.messages = append(s.messages, msg)

	_, err := s.bus.Publish(ctx, types.NewUserMessageAddedEvent(time.Now().UTC(), msg))
	if err != nil {
		return fmt.Errorf("publish user message event: %w", err)
	}

	return nil
}

// Messages returns a snapshot of messages sent during this session.
func (s *Session) Messages() []types.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := make([]types.Message, len(s.messages))
	copy(messages, s.messages)
	return messages
}

func (s *Session) HasMessages() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages) > 0
}

func (s *Session) ContextSummary(tracker *contextwindow.Tracker) contextwindow.Summary {
	if tracker == nil {
		tracker = contextwindow.DefaultTracker()
	}
	return tracker.Analyze(s.Messages())
}

// Close marks the session closed and emits a completion event.
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	hasMessages := len(s.messages) > 0
	s.closed = true
	s.mu.Unlock()

	defer s.bus.Close()
	if !hasMessages {
		return nil
	}

	_, err := s.bus.Publish(ctx, types.Event{
		Kind:      types.EventSessionCompleted,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("publish session completed event: %w", err)
	}

	return nil
}

func (s *Session) Step(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("agent step cancelled: %w", err)
		}

		messages, provider, toolRegistry, err := s.modelRequestState()
		if err != nil {
			return err
		}

		var toolDefs []types.ToolDefinition
		if toolRegistry != nil {
			toolDefs = toolRegistry.Definitions()
		}

		if _, err := s.bus.Publish(ctx, types.Event{
			Kind:      types.EventModelStarted,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("publish model started event: %w", err)
		}

		requestSettings := s.currentRequestSettings()
		if requestSettings.PlanMode {
			toolDefs = planModeToolDefinitions(toolDefs)
		}
		messages = messagesWithSystemPrompt(systemPromptForMode(requestSettings), messages)
		resp, err := s.respond(ctx, provider, types.ModelRequest{
			Model:    requestSettings.Model,
			Effort:   requestSettings.Effort,
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			_, _ = s.bus.Publish(ctx, types.NewSessionErrorEvent(time.Now().UTC(), err))
			return fmt.Errorf("provider respond: %w", err)
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return ErrClosed
		}
		s.messages = append(s.messages, resp.Message)
		s.mu.Unlock()

		if _, err = s.bus.Publish(ctx, types.Event{
			Kind:      types.EventModelMessageCompleted,
			CreatedAt: time.Now().UTC(),
			Message:   &resp.Message,
			Usage:     usagePointer(resp.Usage),
		}); err != nil {
			return fmt.Errorf("publish model completed event: %w", err)
		}

		if len(resp.Message.ToolCalls) == 0 {
			return nil
		}
		if err := s.executeToolCalls(ctx, resp.Message.ToolCalls, toolRegistry); err != nil {
			return err
		}
	}
}

func (s *Session) currentRequestSettings() types.ModelRequestSettings {
	if s.requestSettings == nil {
		return types.ModelRequestSettings{}
	}
	return s.requestSettings()
}

func (s *Session) modelRequestState() ([]types.Message, types.Provider, *tools.Registry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, nil, nil, ErrClosed
	}
	if s.provider == nil {
		return nil, nil, nil, ErrProviderMissing
	}

	messages := make([]types.Message, len(s.messages))
	copy(messages, s.messages)

	return messages, s.provider, s.tools, nil
}

func messagesWithSystemPrompt(prompt string, messages []types.Message) []types.Message {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return messages
	}

	withPrompt := make([]types.Message, 0, len(messages)+1)
	withPrompt = append(withPrompt, types.SystemMessage(prompt))
	withPrompt = append(withPrompt, messages...)
	return withPrompt
}

func systemPromptForMode(settings types.ModelRequestSettings) string {
	prompt := strings.TrimSpace(settings.SystemPrompt)
	if !settings.PlanMode {
		return prompt
	}
	return strings.TrimSpace(strings.Join([]string{prompt, planModeSystemPrompt}, "\n\n"))
}

const planModeSystemPrompt = `Plan mode is active.

You are planning only. You may inspect the workspace using read-only tools, but you must not modify files, run commands, apply patches, install packages, commit changes, or perform any other mutating action.

Produce a concise implementation plan before coding. Include the files or areas you expect to change, the approach, risks or open questions, and verification steps. End by asking for approval to implement. Do not proceed with implementation until the user explicitly approves or switches out of plan mode.`

func planModeToolDefinitions(defs []types.ToolDefinition) []types.ToolDefinition {
	if len(defs) == 0 {
		return defs
	}

	filtered := make([]types.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if planModeAllowsTool(def.Name) {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func planModeAllowsTool(name string) bool {
	switch name {
	case "read_file", "list_dir", "file_tree", "grep_search", "ripgrep", "git_status", "git_diff":
		return true
	default:
		return false
	}
}

func usagePointer(usage types.Usage) *types.Usage {
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return &usage
}

func (s *Session) respond(ctx context.Context, provider types.Provider,
	req types.ModelRequest) (*types.ModelResponse, error) {
	streamer, ok := provider.(types.StreamingProvider)
	if !ok {
		return provider.Respond(ctx, req)
	}

	return streamer.StreamRespond(ctx, req, func(delta types.ModelDelta) error {
		if delta.Text == "" {
			return nil
		}

		_, err := s.bus.Publish(ctx, types.NewModelTextDeltaEvent(time.Now().UTC(), delta.Text))
		if err != nil {
			return fmt.Errorf("publish model text delta event: %w", err)
		}
		return nil
	})
}

func (s *Session) executeToolCalls(ctx context.Context, calls []types.ToolCall,
	registry *tools.Registry) error {
	for _, call := range calls {
		if _, err := s.bus.Publish(ctx, types.NewToolCallRequestedEvent(
			time.Now().UTC(), call)); err != nil {
			return fmt.Errorf("publish tool call requested event: %w", err)
		}

		result, stop, err := s.executeToolCall(ctx, call, registry)
		if err != nil {
			return err
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return ErrClosed
		}
		s.messages = append(s.messages, types.ToolMessage(call.ID, call.Name, result))
		s.mu.Unlock()
		if stop {
			return nil
		}
	}

	return nil
}

func (s *Session) executeToolCall(ctx context.Context, call types.ToolCall,
	registry *tools.Registry) (types.ToolResult, bool, error) {
	if registry == nil {
		result := types.ErrorToolResult("tool registry is not configured")
		if err := s.publishToolCallCompleted(ctx, call, result); err != nil {
			return types.ToolResult{}, false, err
		}
		return result, true, nil
	}

	tool, ok := registry.Get(call.Name)
	if !ok {
		result := types.ErrorToolResult(fmt.Sprintf("tool %q not found", call.Name))
		if err := s.publishToolCallCompleted(ctx, call, result); err != nil {
			return types.ToolResult{}, false, err
		}
		return result, true, nil
	}
	if s.currentRequestSettings().PlanMode && !planModeAllowsTool(call.Name) {
		result := types.ErrorToolResult("tool denied because plan mode is active")
		if err := s.publishToolCallCompleted(ctx, call, result); err != nil {
			return types.ToolResult{}, false, err
		}
		return result, true, nil
	}

	allowed, deniedResult, err := s.authorizeToolCall(ctx, call, tool)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
			ctx.Err() != nil {
			result := types.ErrorToolResult("tool call cancelled before approval completed")
			if err := s.publishToolCallCompleted(ctx, call, result); err != nil {
				return types.ToolResult{}, false, err
			}
			return result, true, nil
		}
		return types.ToolResult{}, false, err
	}
	if !allowed {
		if err := s.publishToolCallCompleted(ctx, call, deniedResult); err != nil {
			return types.ToolResult{}, false, err
		}
		return deniedResult, true, nil
	}

	if _, err := s.bus.Publish(ctx, types.NewToolCallStartedEvent(
		time.Now().UTC(), call)); err != nil {
		return types.ToolResult{}, false, fmt.Errorf("publish tool call started event: %w", err)
	}

	result, err := tool.Run(ctx, call.Input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			cancelled := types.ErrorToolResult("tool call cancelled")
			if err := s.publishToolCallCompleted(ctx, call, cancelled); err != nil {
				return types.ToolResult{}, false, err
			}
			return cancelled, true, nil
		}
		errorResult := types.ErrorToolResult(err.Error())
		result = &errorResult
	}
	if result == nil {
		result = &types.ToolResult{}
	}

	if err := s.publishToolCallCompleted(ctx, call, *result); err != nil {
		return types.ToolResult{}, false, err
	}

	return *result, false, nil
}

func (s *Session) publishToolCallCompleted(ctx context.Context, call types.ToolCall,
	result types.ToolResult) error {
	if _, err := s.bus.Publish(context.WithoutCancel(ctx), types.NewToolCallCompletedEvent(
		time.Now().UTC(), call, result)); err != nil {
		return fmt.Errorf("publish tool call completed event: %w", err)
	}
	return nil
}

func (s *Session) authorizeToolCall(ctx context.Context, call types.ToolCall,
	tool tools.Tool) (bool, types.ToolResult, error) {
	mode := tools.Permission(tool)
	request := types.PermissionRequest{
		SessionID:  s.id,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Input:      call.Input,
		Mode:       mode,
		Reason:     "tool requested by model",
	}

	switch mode {
	case types.PermissionAllow:
		return true, types.ToolResult{}, nil
	case types.PermissionDeny:
		result := types.PermissionResult{
			Request:  request,
			Decision: types.PermissionDecisionDeny,
			Reason:   "denied by tool permission policy",
		}
		if _, err := s.bus.Publish(ctx, types.NewPermissionDeniedEvent(
			time.Now().UTC(), result)); err != nil {
			return false, types.ToolResult{}, fmt.Errorf("publish permission denied event: %w", err)
		}
		return false, types.ErrorToolResult(result.Reason), nil
	case types.PermissionAsk:
	default:
		request.Mode = types.PermissionAsk
	}

	if cachedBroker, ok := s.broker.(permissions.CachedBroker); ok {
		if cached, ok := cachedBroker.CachedDecision(request); ok {
			if cached.Request.ToolName == "" {
				cached.Request = request
			}
			switch cached.Decision {
			case types.PermissionDecisionAllow, types.PermissionDecisionAllowSession:
				if _, err := s.bus.Publish(ctx, types.NewPermissionGrantedEvent(
					time.Now().UTC(), cached)); err != nil {
					return false, types.ToolResult{}, fmt.Errorf("publish permission granted event: %w", err)
				}
				return true, types.ToolResult{}, nil
			case types.PermissionDecisionDeny:
				if cached.Reason == "" {
					cached.Reason = "permission denied"
				}
				if _, err := s.bus.Publish(ctx, types.NewPermissionDeniedEvent(
					time.Now().UTC(), cached)); err != nil {
					return false, types.ToolResult{}, fmt.Errorf("publish permission denied event: %w", err)
				}
				return false, types.ErrorToolResult(cached.Reason), nil
			}
		}
	}

	if _, err := s.bus.Publish(ctx, types.NewPermissionRequestedEvent(
		time.Now().UTC(), request)); err != nil {
		return false, types.ToolResult{}, fmt.Errorf("publish permission requested event: %w", err)
	}

	if s.broker == nil {
		result := types.PermissionResult{
			Request:  request,
			Decision: types.PermissionDecisionDeny,
			Reason:   "no permission broker configured",
		}
		if _, err := s.bus.Publish(ctx, types.NewPermissionDeniedEvent(
			time.Now().UTC(), result)); err != nil {
			return false, types.ToolResult{}, fmt.Errorf("publish permission denied event: %w", err)
		}
		return false, types.ErrorToolResult(result.Reason), nil
	}

	result, err := s.broker.Decide(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_, _ = s.bus.Publish(ctx, types.NewSessionErrorEvent(time.Now().UTC(), err))
		}
		return false, types.ToolResult{}, fmt.Errorf("decide permission for tool %q: %w",
			call.Name, err)
	}
	if result.Request.ToolName == "" {
		result.Request = request
	}

	switch result.Decision {
	case types.PermissionDecisionAllow, types.PermissionDecisionAllowSession:
		if _, err := s.bus.Publish(ctx, types.NewPermissionGrantedEvent(
			time.Now().UTC(), result)); err != nil {
			return false, types.ToolResult{}, fmt.Errorf("publish permission granted event: %w", err)
		}
		return true, types.ToolResult{}, nil
	case types.PermissionDecisionDeny:
		if result.Reason == "" {
			result.Reason = "permission denied"
		}
		if _, err := s.bus.Publish(ctx, types.NewPermissionDeniedEvent(
			time.Now().UTC(), result)); err != nil {
			return false, types.ToolResult{}, fmt.Errorf("publish permission denied event: %w", err)
		}
		return false, types.ErrorToolResult(result.Reason), nil
	default:
		result.Decision = types.PermissionDecisionDeny
		result.Reason = "permission broker returned an invalid decision"
		if _, err := s.bus.Publish(ctx, types.NewPermissionDeniedEvent(
			time.Now().UTC(), result)); err != nil {
			return false, types.ToolResult{}, fmt.Errorf("publish permission denied event: %w", err)
		}
		return false, types.ErrorToolResult(result.Reason), nil
	}
}

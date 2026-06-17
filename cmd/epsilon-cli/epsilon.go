package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/shxntanu/epsilon/core"
	harnessconfig "github.com/shxntanu/epsilon/core/config"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/providers/fake"
	"github.com/shxntanu/epsilon/core/providers/litellm"
	"github.com/shxntanu/epsilon/core/types"
)

type harnessConfig struct {
	sessionDir      string
	workspace       string
	provider        string
	model           string
	effort          string
	systemPrompt    string
	litellmBaseURL  string
	litellmAPIKey   string
	eventBufferSize int
	allowAll        bool
	broker          permissions.Broker
	configStore     *harnessconfig.Store
}

func main() {
	if err := execute(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "epsilon:", err)
		os.Exit(1)
	}
}

func execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return fmt.Errorf("missing command")
	}

	switch args[0] {
	case "run":
		return runCommand(ctx, args[1:])
	case "tui":
		return tuiCommand(ctx, args[1:])
	case "resume":
		return resumeCommand(ctx, args[1:])
	case "events":
		return eventsCommand(ctx, args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCommand(ctx context.Context, args []string) error {
	configStore, defaults, err := loadHarnessConfig(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("config", harnessconfig.DefaultPath, "harness config file")
	sessionDir := fs.String("session-dir", defaults.SessionDir, "directory for persisted sessions")
	workspace := fs.String("workspace", defaults.Workspace, "workspace root for file tools")
	allowAll := fs.Bool("y", false, "allow permission requests without prompting")
	provider := fs.String("provider", envOrDefault("EPSILON_PROVIDER", defaults.Provider), "provider to use: fake or litellm")
	model := fs.String("model", envOrDefault("LITELLM_MODEL", defaults.Model), "model name for LiteLLM")
	effort := fs.String("effort", envOrDefault("EPSILON_MODEL_EFFORT", defaults.Effort), "model reasoning effort")
	systemPrompt := fs.String("system-prompt", envOrDefault("EPSILON_SYSTEM_PROMPT", defaults.SystemPrompt), "additional system prompt text")
	systemPromptFile := fs.String("system-prompt-file", envOrDefault("EPSILON_SYSTEM_PROMPT_FILE", ""), "path to an additional system prompt file")
	litellmBaseURL := fs.String("litellm-base-url", envOrDefault("LITELLM_BASE_URL", defaults.LiteLLMBaseURL), "LiteLLM proxy base URL")
	litellmAPIKey := fs.String("litellm-api-key", os.Getenv("LITELLM_API_KEY"), "LiteLLM proxy API key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resolvedSystemPrompt, err := resolveSystemPrompt(*systemPrompt, *systemPromptFile)
	if err != nil {
		return err
	}

	message, err := messageFromArgs(fs.Args())
	if err != nil {
		return err
	}

	harness, err := newHarness(ctx, harnessConfig{
		sessionDir:      *sessionDir,
		workspace:       *workspace,
		provider:        *provider,
		model:           *model,
		effort:          *effort,
		systemPrompt:    resolvedSystemPrompt,
		litellmBaseURL:  *litellmBaseURL,
		litellmAPIKey:   *litellmAPIKey,
		eventBufferSize: defaults.EventBufferSize,
		allowAll:        *allowAll,
		configStore:     configStore,
	})
	if err != nil {
		return err
	}

	sess, err := harness.StartSession(ctx)
	if err != nil {
		return err
	}

	sub, err := sess.Subscribe()
	if err != nil {
		return err
	}
	defer sub.Close()

	if err := sess.Send(ctx, types.UserMessage(message)); err != nil {
		return err
	}
	if err := sess.Step(ctx); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "session_id: %s\n", sess.ID())
	drainEvents(os.Stdout, sub.Events())
	return nil
}

func resumeCommand(ctx context.Context, args []string) error {
	configStore, defaults, err := loadHarnessConfig(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("config", harnessconfig.DefaultPath, "harness config file")
	sessionDir := fs.String("session-dir", defaults.SessionDir, "directory for persisted sessions")
	workspace := fs.String("workspace", defaults.Workspace, "workspace root for file tools")
	allowAll := fs.Bool("y", false, "allow permission requests without prompting")
	provider := fs.String("provider", envOrDefault("EPSILON_PROVIDER", defaults.Provider), "provider to use: fake or litellm")
	model := fs.String("model", envOrDefault("LITELLM_MODEL", defaults.Model), "model name for LiteLLM")
	effort := fs.String("effort", envOrDefault("EPSILON_MODEL_EFFORT", defaults.Effort), "model reasoning effort")
	systemPrompt := fs.String("system-prompt", envOrDefault("EPSILON_SYSTEM_PROMPT", defaults.SystemPrompt), "additional system prompt text")
	systemPromptFile := fs.String("system-prompt-file", envOrDefault("EPSILON_SYSTEM_PROMPT_FILE", ""), "path to an additional system prompt file")
	litellmBaseURL := fs.String("litellm-base-url", envOrDefault("LITELLM_BASE_URL", defaults.LiteLLMBaseURL), "LiteLLM proxy base URL")
	litellmAPIKey := fs.String("litellm-api-key", os.Getenv("LITELLM_API_KEY"), "LiteLLM proxy API key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resolvedSystemPrompt, err := resolveSystemPrompt(*systemPrompt, *systemPromptFile)
	if err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("resume requires session ID")
	}
	sessionID := remaining[0]

	harness, err := newHarness(ctx, harnessConfig{
		sessionDir:      *sessionDir,
		workspace:       *workspace,
		provider:        *provider,
		model:           *model,
		effort:          *effort,
		systemPrompt:    resolvedSystemPrompt,
		litellmBaseURL:  *litellmBaseURL,
		litellmAPIKey:   *litellmAPIKey,
		eventBufferSize: defaults.EventBufferSize,
		allowAll:        *allowAll,
		configStore:     configStore,
	})
	if err != nil {
		return err
	}

	sess, err := harness.ResumeSession(ctx, sessionID)
	if err != nil {
		return err
	}

	sub, err := sess.Subscribe()
	if err != nil {
		return err
	}
	defer sub.Close()

	if len(remaining) > 1 {
		message := strings.Join(remaining[1:], " ")
		if err := sess.Send(ctx, types.UserMessage(message)); err != nil {
			return err
		}
		if err := sess.Step(ctx); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stdout, "session_id: %s\n", sess.ID())
	drainEvents(os.Stdout, sub.Events())
	return nil
}

func eventsCommand(ctx context.Context, args []string) error {
	_, defaults, err := loadHarnessConfig(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("config", harnessconfig.DefaultPath, "harness config file")
	sessionDir := fs.String("session-dir", defaults.SessionDir, "directory for persisted sessions")
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("events requires session ID")
	}

	store, err := events.NewJSONLStore(*sessionDir)
	if err != nil {
		return err
	}

	history, err := store.Load(ctx, remaining[0])
	if err != nil {
		return err
	}
	for _, event := range history {
		printEvent(os.Stdout, event)
	}

	return nil
}

func newHarness(ctx context.Context, config harnessConfig) (*core.Harness, error) {
	broker := config.broker
	if config.allowAll {
		broker = permissions.NewAllowBroker()
	}
	if broker == nil {
		broker = permissions.Broker(promptBroker{})
	}

	provider, err := newProvider(config)
	if err != nil {
		return nil, err
	}

	options := []core.Option{
		core.WithProvider(provider),
		core.WithConfigStore(config.configStore),
		core.WithRuntimeSettings(harnessconfig.Settings{
			SessionDir:      config.sessionDir,
			Workspace:       config.workspace,
			Provider:        config.provider,
			Model:           config.model,
			Effort:          config.effort,
			SystemPrompt:    config.systemPrompt,
			LiteLLMBaseURL:  config.litellmBaseURL,
			EventBufferSize: config.eventBufferSize,
		}),
		core.WithDefaultTools(config.workspace),
		core.WithSessionDir(config.sessionDir),
		core.WithPermissionBroker(broker),
	}
	if config.eventBufferSize > 0 {
		options = append(options, core.WithEventBufferSize(config.eventBufferSize))
	}
	if modelInfo, ok := selectedModelInfoForProvider(ctx, provider); ok {
		options = append(options, core.WithSelectedModelInfo(modelInfo))
	}

	harness, err := core.New(options...)
	if err != nil {
		return nil, err
	}

	if config.configStore != nil {
		if err := config.configStore.SaveSessionDefaults(harnessconfig.Settings{
			SessionDir:      config.sessionDir,
			Workspace:       config.workspace,
			Provider:        config.provider,
			Model:           config.model,
			Effort:          config.effort,
			SystemPrompt:    config.systemPrompt,
			LiteLLMBaseURL:  config.litellmBaseURL,
			EventBufferSize: config.eventBufferSize,
		}); err != nil {
			return nil, err
		}
	}

	return harness, nil
}

func selectedModelInfoForProvider(ctx context.Context, provider types.Provider) (types.ModelInfo, bool) {
	catalog, ok := provider.(types.ModelCatalogProvider)
	if !ok {
		return types.ModelInfo{}, false
	}
	selected, ok := provider.(types.SelectedModelProvider)
	if !ok || strings.TrimSpace(selected.SelectedModel()) == "" {
		return types.ModelInfo{}, false
	}

	metadataCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	models, err := catalog.ListModels(metadataCtx)
	if err != nil {
		return types.ModelInfo{}, false
	}
	selectedID := strings.TrimSpace(selected.SelectedModel())
	for _, model := range models {
		if model.ID == selectedID || model.Name == selectedID || model.ProviderModel == selectedID ||
			strings.TrimPrefix(selectedID, model.Provider+"/") == model.ID ||
			strings.TrimPrefix(selectedID, model.Provider+"/") == model.Name ||
			strings.TrimPrefix(model.ProviderModel, model.Provider+"/") == selectedID {
			return model, true
		}
	}

	return types.ModelInfo{}, false
}

func newProvider(config harnessConfig) (types.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(config.provider)) {
	case "", "fake":
		return fake.New(), nil
	case "litellm":
		provider, err := litellm.New(litellm.Config{
			BaseURL: config.litellmBaseURL,
			APIKey:  config.litellmAPIKey,
			Model:   config.model,
		})
		if err != nil {
			return nil, err
		}
		cacheKey := harnessconfig.ModelCatalogKey("litellm", config.litellmBaseURL)
		return harnessconfig.WrapProvider(provider, config.configStore, cacheKey), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", config.provider)
	}
}

func loadHarnessConfig(args []string) (*harnessconfig.Store, harnessconfig.Settings, error) {
	store, err := harnessconfig.LoadStore(configPathFromArgs(args))
	if err != nil {
		return nil, harnessconfig.Settings{}, err
	}
	return store, store.EffectiveSettings(), nil
}

func configPathFromArgs(args []string) string {
	for i, arg := range args {
		if arg == "-config" && i+1 < len(args) {
			return args[i+1]
		}
		if value, ok := strings.CutPrefix(arg, "-config="); ok {
			return value
		}
	}
	return harnessconfig.DefaultPath
}

func messageFromArgs(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	message := strings.TrimSpace(string(data))
	if message == "" {
		return "", fmt.Errorf("message is empty")
	}

	return message, nil
}

func resolveSystemPrompt(prompt string, promptFile string) (string, error) {
	promptFile = strings.TrimSpace(promptFile)
	if promptFile == "" {
		return strings.TrimSpace(prompt), nil
	}

	data, err := os.ReadFile(promptFile)
	if err != nil {
		return "", fmt.Errorf("read system prompt file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func drainEvents(w io.Writer, ch <-chan types.Event) {
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			printEvent(w, event)
		default:
			return
		}
	}
}

func printEvent(w io.Writer, event types.Event) {
	fmt.Fprintf(w, "[%03d] %-24s %s\n", event.Sequence, event.Kind,
		event.CreatedAt.Format(time.RFC3339))

	if event.Message != nil {
		fmt.Fprintf(w, "      message %s: %s\n", event.Message.Role,
			truncate(textFromContent(event.Message.Content), 240))
		if len(event.Message.ToolCalls) > 0 {
			for _, call := range event.Message.ToolCalls {
				fmt.Fprintf(w, "      tool_call %s(%s): %s\n", call.Name, call.ID,
					truncate(string(call.Input), 240))
			}
		}
	}
	if event.TextDelta != "" {
		fmt.Fprintf(w, "      delta: %s\n", truncate(event.TextDelta, 240))
	}
	if event.ToolCall != nil {
		fmt.Fprintf(w, "      tool_call %s(%s): %s\n", event.ToolCall.Name, event.ToolCall.ID,
			truncate(string(event.ToolCall.Input), 240))
	}
	if event.ToolResult != nil {
		fmt.Fprintf(w, "      tool_result error=%t: %s\n", event.ToolResult.IsError,
			truncate(textFromContent(event.ToolResult.Content), 240))
		if len(event.ToolResult.Metadata) > 0 {
			data, err := json.Marshal(event.ToolResult.Metadata)
			if err == nil {
				fmt.Fprintf(w, "      metadata: %s\n", data)
			}
		}
	}
	if event.Permission != nil {
		fmt.Fprintf(w, "      permission %s for %s: %s\n",
			event.Permission.Decision,
			event.Permission.Request.ToolName,
			event.Permission.Reason)
	}
	if event.Error != "" {
		fmt.Fprintf(w, "      error: %s\n", event.Error)
	}
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

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}

	return text[:limit] + "..."
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  epsilon run [flags] <message>")
	fmt.Fprintln(w, "  epsilon tui [flags] [session-id]")
	fmt.Fprintln(w, "  epsilon resume [flags] <session-id> [message]")
	fmt.Fprintln(w, "  epsilon events [flags] <session-id>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "fake tool prompts:")
	fmt.Fprintln(w, "  tool:echo hello")
	fmt.Fprintln(w, "  tool:read README.md")
	fmt.Fprintln(w, "  tool:read README.md 10 20")
	fmt.Fprintln(w, "  tool:write scratch.txt hello")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "provider flags:")
	fmt.Fprintln(w, "  -provider fake|litellm")
	fmt.Fprintln(w, "  -litellm-base-url http://localhost:4000")
	fmt.Fprintln(w, "  -litellm-api-key <key>")
	fmt.Fprintln(w, "  -model <model>")
	fmt.Fprintln(w, "  -effort <effort>")
	fmt.Fprintln(w, "  -system-prompt <text>        append instructions to the default system prompt")
	fmt.Fprintln(w, "  -system-prompt-file <path>   append instructions from a file")
	fmt.Fprintln(w, "  -config .epsilon/config.json")
}

type promptBroker struct{}

func (promptBroker) Decide(ctx context.Context,
	request types.PermissionRequest) (types.PermissionResult, error) {
	select {
	case <-ctx.Done():
		return types.PermissionResult{}, fmt.Errorf("permission prompt cancelled: %w", ctx.Err())
	default:
	}

	fmt.Fprintf(os.Stderr, "Allow tool %q with input %s? [y/N] ",
		request.ToolName, truncate(string(request.Input), 240))

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return types.PermissionResult{}, fmt.Errorf("read permission decision: %w", err)
	}

	decision := types.PermissionDecisionDeny
	reason := "denied by user"
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "y" || answer == "yes" {
		decision = types.PermissionDecisionAllow
		reason = "allowed by user"
	}

	return types.PermissionResult{
		Request:  request,
		Decision: decision,
		Reason:   reason,
	}, nil
}

func envOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

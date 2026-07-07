package main

import (
	"context"
	"flag"
	"io"

	harnessconfig "github.com/shxntanu/epsilon/core/config"
	"github.com/shxntanu/epsilon/epsilon-cli/tui"
)

func tuiCommand(ctx context.Context, args []string) error {
	configStore, defaults, err := loadHarnessConfig(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("config", harnessconfig.DefaultPath, "harness config file")
	sessionDir := fs.String("session-dir", defaults.SessionDir, "directory for persisted sessions")
	workspace := fs.String("workspace", defaults.Workspace, "workspace root for file tools")
	allowAll := fs.Bool("y", false, "allow permission requests without prompting")
	provider := fs.String("provider", envOrDefault("EPSILON_PROVIDER", defaults.Provider), "provider to use: fake or litellm")
	model := fs.String("model", envOrDefault("LITELLM_MODEL", defaults.Model), "model name for LiteLLM")
	effort := fs.String("effort", envOrDefault("EPSILON_MODEL_EFFORT", defaults.Effort), "model reasoning effort")
	planMode := fs.Bool("plan", defaults.PlanMode, "enable read-only planning mode")
	systemPrompt := fs.String("system-prompt", envOrDefault("EPSILON_SYSTEM_PROMPT", defaults.SystemPrompt), "additional system prompt text")
	systemPromptFile := fs.String("system-prompt-file", envOrDefault("EPSILON_SYSTEM_PROMPT_FILE", ""), "path to an additional system prompt file")
	litellmBaseURL := fs.String("litellm-base-url", envOrDefault("LITELLM_BASE_URL", defaults.LiteLLMBaseURL), "LiteLLM proxy base URL")
	litellmAPIKey := fs.String("litellm-api-key", envOrDefault("LITELLM_API_KEY", ""), "LiteLLM proxy API key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resolvedSystemPrompt, err := resolveSystemPrompt(*systemPrompt, *systemPromptFile)
	if err != nil {
		return err
	}

	var permissionBroker *tui.PermissionBroker
	if !*allowAll {
		permissionBroker = tui.NewPermissionBroker()
	}

	harness, err := newHarness(ctx, harnessConfig{
		sessionDir:      *sessionDir,
		workspace:       *workspace,
		provider:        *provider,
		model:           *model,
		effort:          *effort,
		planMode:        *planMode,
		systemPrompt:    resolvedSystemPrompt,
		litellmBaseURL:  *litellmBaseURL,
		litellmAPIKey:   *litellmAPIKey,
		eventBufferSize: defaults.EventBufferSize,
		allowAll:        *allowAll,
		broker:          permissionBroker,
		configStore:     configStore,
	})
	if err != nil {
		return err
	}

	config := tui.Config{
		Harness:          harness,
		PermissionBroker: permissionBroker,
	}
	if remaining := fs.Args(); len(remaining) > 0 {
		config.SessionID = remaining[0]
	}

	return tui.Start(ctx, config)
}

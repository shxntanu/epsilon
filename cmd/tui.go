package main

import (
	"context"
	"flag"
	"io"

	"github.com/shxntanu/epsilon/tui"
)

func tuiCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sessionDir := fs.String("session-dir", defaultSessionDir, "directory for persisted sessions")
	workspace := fs.String("workspace", ".", "workspace root for file tools")
	allowAll := fs.Bool("y", false, "allow permission requests without prompting")
	provider := fs.String("provider", envOrDefault("EPSILON_PROVIDER", "fake"), "provider to use: fake or litellm")
	model := fs.String("model", envOrDefault("LITELLM_MODEL", ""), "model name for LiteLLM")
	litellmBaseURL := fs.String("litellm-base-url", envOrDefault("LITELLM_BASE_URL", "http://localhost:4000"), "LiteLLM proxy base URL")
	litellmAPIKey := fs.String("litellm-api-key", envOrDefault("LITELLM_API_KEY", ""), "LiteLLM proxy API key")
	if err := fs.Parse(args); err != nil {
		return err
	}

	harness, err := newHarness(harnessConfig{
		sessionDir:     *sessionDir,
		workspace:      *workspace,
		provider:       *provider,
		model:          *model,
		litellmBaseURL: *litellmBaseURL,
		litellmAPIKey:  *litellmAPIKey,
		allowAll:       *allowAll,
	})
	if err != nil {
		return err
	}

	config := tui.Config{
		Harness: harness,
	}
	if remaining := fs.Args(); len(remaining) > 0 {
		config.RunID = remaining[0]
	}

	return tui.Run(ctx, config)
}

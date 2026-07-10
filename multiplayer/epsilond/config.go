package epsilond

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr             = ":8080"
	defaultShutdownTimeout      = 10 * time.Second
	defaultMaxEventBodyBytes    = int64(1 << 20)
	defaultWorkspaceRoot        = "/data/workspaces"
	defaultRepoCacheRoot        = "/data/repo-cache"
	defaultTemporalNamespace    = "default"
	defaultTemporalTaskQueue    = "epsilond"
	defaultPerTaskTokenBudget   = 200_000
	defaultPerThreadDailyBudget = 1_000_000
	defaultPerSpaceDailyBudget  = 10_000_000
)

// Config contains runtime settings for the epsilond service.
type Config struct {
	HTTPAddr             string
	ShutdownTimeout      time.Duration
	MaxEventBodyBytes    int64
	PostgresDSN          string
	TemporalAddress      string
	TemporalNamespace    string
	TemporalTaskQueue    string
	LiteLLMBaseURL       string
	LiteLLMAPIKey        string
	Mem0BaseURL          string
	Mem0APIKey           string
	GoogleChatAudience   string
	GoogleChatServiceKey string
	WorkspaceRoot        string
	RepoCacheRoot        string
	SandboxBackend       string
	SRTBinary            string
	AllowedSpaces        []string
	AllowedRepos         []string
	PerTaskTokenBudget   int
	PerThreadDailyBudget int
	PerSpaceDailyBudget  int
}

// LoadConfigFromEnv returns epsilond configuration from environment variables.
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		HTTPAddr:             envOrDefault("EPSILOND_HTTP_ADDR", defaultHTTPAddr),
		ShutdownTimeout:      durationEnvOrDefault("EPSILOND_SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		MaxEventBodyBytes:    int64EnvOrDefault("EPSILOND_MAX_EVENT_BODY_BYTES", defaultMaxEventBodyBytes),
		PostgresDSN:          strings.TrimSpace(os.Getenv("EPSILOND_POSTGRES_DSN")),
		TemporalAddress:      strings.TrimSpace(os.Getenv("EPSILOND_TEMPORAL_ADDRESS")),
		TemporalNamespace:    envOrDefault("EPSILOND_TEMPORAL_NAMESPACE", defaultTemporalNamespace),
		TemporalTaskQueue:    envOrDefault("EPSILOND_TEMPORAL_TASK_QUEUE", defaultTemporalTaskQueue),
		LiteLLMBaseURL:       strings.TrimSpace(os.Getenv("LITELLM_BASE_URL")),
		LiteLLMAPIKey:        strings.TrimSpace(os.Getenv("LITELLM_API_KEY")),
		Mem0BaseURL:          strings.TrimSpace(os.Getenv("EPSILOND_MEM0_BASE_URL")),
		Mem0APIKey:           strings.TrimSpace(os.Getenv("EPSILOND_MEM0_API_KEY")),
		GoogleChatAudience:   strings.TrimSpace(os.Getenv("EPSILOND_GOOGLE_CHAT_AUDIENCE")),
		GoogleChatServiceKey: strings.TrimSpace(os.Getenv("EPSILOND_GOOGLE_CHAT_SERVICE_KEY")),
		WorkspaceRoot:        envOrDefault("EPSILOND_WORKSPACE_ROOT", defaultWorkspaceRoot),
		RepoCacheRoot:        envOrDefault("EPSILOND_REPO_CACHE_ROOT", defaultRepoCacheRoot),
		SandboxBackend:       envOrDefault("EPSILOND_SANDBOX_BACKEND", "srt"),
		SRTBinary:            envOrDefault("EPSILOND_SRT_BINARY", "srt"),
		AllowedSpaces:        listEnv("EPSILOND_ALLOWED_SPACES"),
		AllowedRepos:         listEnv("EPSILOND_ALLOWED_REPOS"),
		PerTaskTokenBudget:   intEnvOrDefault("EPSILOND_PER_TASK_TOKEN_BUDGET", defaultPerTaskTokenBudget),
		PerThreadDailyBudget: intEnvOrDefault("EPSILOND_PER_THREAD_DAILY_TOKEN_BUDGET", defaultPerThreadDailyBudget),
		PerSpaceDailyBudget:  intEnvOrDefault("EPSILOND_PER_SPACE_DAILY_TOKEN_BUDGET", defaultPerSpaceDailyBudget),
	}
	if cfg.MaxEventBodyBytes <= 0 {
		return Config{}, fmt.Errorf("EPSILOND_MAX_EVENT_BODY_BYTES must be positive")
	}
	if cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("EPSILOND_SHUTDOWN_TIMEOUT must be positive")
	}
	return cfg, nil
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func listEnv(name string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func intEnvOrDefault(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func int64EnvOrDefault(name string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func durationEnvOrDefault(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

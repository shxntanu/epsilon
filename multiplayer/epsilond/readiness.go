package epsilond

import (
	"context"
	"fmt"
	"strings"
)

// ConfigReadiness checks whether required production configuration is present.
type ConfigReadiness struct {
	Config Config
}

// Ready reports missing core epsilond dependencies.
func (r ConfigReadiness) Ready(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("readiness cancelled: %w", ctx.Err())
	default:
	}
	var missing []string
	if strings.TrimSpace(r.Config.PostgresDSN) == "" {
		missing = append(missing, "EPSILOND_POSTGRES_DSN")
	}
	if strings.TrimSpace(r.Config.TemporalAddress) == "" {
		missing = append(missing, "EPSILOND_TEMPORAL_ADDRESS")
	}
	if strings.TrimSpace(r.Config.LiteLLMBaseURL) == "" {
		missing = append(missing, "LITELLM_BASE_URL")
	}
	if strings.TrimSpace(r.Config.Mem0BaseURL) == "" {
		missing = append(missing, "EPSILOND_MEM0_BASE_URL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

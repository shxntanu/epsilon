package models

import "strings"

// Role identifies the model role requested by the epsilond orchestrator.
type Role string

const (
	RoleOrchestratorFrontier  Role = "orchestrator_frontier"
	RoleWorkerDefaultCode     Role = "worker_default_code"
	RoleWorkerDefaultArtifact Role = "worker_default_artifact"
	RoleWorkerEscalation      Role = "worker_escalation"
	RoleCheapClassifier       Role = "cheap_classifier"
	RoleSummarizer            Role = "summarizer"
	RoleMemoryDistiller       Role = "memory_distiller"
)

// Effort is a provider-neutral hint for reasoning depth or latency/cost tier.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// EscalationReason describes why a task should move from a default worker to
// a more capable model route.
type EscalationReason string

const (
	EscalationReasonLowConfidence                 EscalationReason = "low_confidence"
	EscalationReasonConflictingFindings           EscalationReason = "conflicting_findings"
	EscalationReasonMissingEvidence               EscalationReason = "missing_evidence"
	EscalationReasonProductionSecuritySensitivity EscalationReason = "production_security_sensitivity"
	EscalationReasonBudgetAllowed                 EscalationReason = "budget_allowed"
)

// Config maps each model role to the provider/model route and cost policy used
// for estimates. Costs are expressed in USD per 1M tokens and are intentionally
// configurable because provider pricing changes over time.
type Config struct {
	Routes map[Role]Route
}

// Route describes the model selection and budget policy for one role.
type Route struct {
	Provider                               string
	Model                                  string
	Effort                                 Effort
	MaxTokens                              int64
	EstimatedInputCostPerMillionTokensUSD  float64
	EstimatedOutputCostPerMillionTokensUSD float64
}

// Router resolves model routes from an immutable-by-convention Config.
type Router struct {
	config Config
}

// CostEstimate is the estimated spend for a model invocation.
type CostEstimate struct {
	Provider      string
	Model         string
	InputTokens   int64
	OutputTokens  int64
	InputCostUSD  float64
	OutputCostUSD float64
	TotalCostUSD  float64
}

// DefaultConfig returns conservative placeholder model routes for epsilond.
func DefaultConfig() Config {
	return Config{
		Routes: map[Role]Route{
			RoleOrchestratorFrontier: {
				Effort:    EffortHigh,
				MaxTokens: 32_000,
			},
			RoleWorkerDefaultCode: {
				Effort:    EffortMedium,
				MaxTokens: 16_000,
			},
			RoleWorkerDefaultArtifact: {
				Effort:    EffortMedium,
				MaxTokens: 16_000,
			},
			RoleWorkerEscalation: {
				Effort:    EffortHigh,
				MaxTokens: 32_000,
			},
			RoleCheapClassifier: {
				Effort:    EffortLow,
				MaxTokens: 2_000,
			},
			RoleSummarizer: {
				Effort:    EffortLow,
				MaxTokens: 8_000,
			},
			RoleMemoryDistiller: {
				Effort:    EffortLow,
				MaxTokens: 8_000,
			},
		},
	}
}

// Roles returns the stable list of supported model routing roles.
func Roles() []Role {
	return []Role{
		RoleOrchestratorFrontier,
		RoleWorkerDefaultCode,
		RoleWorkerDefaultArtifact,
		RoleWorkerEscalation,
		RoleCheapClassifier,
		RoleSummarizer,
		RoleMemoryDistiller,
	}
}

// ParseRole accepts a role name when it matches a known role.
func ParseRole(value string) (Role, bool) {
	role := Role(strings.TrimSpace(value))
	for _, known := range Roles() {
		if role == known {
			return role, true
		}
	}
	return "", false
}

// NewRouter creates a router backed by cfg.
func NewRouter(cfg Config) Router {
	return Router{config: cfg}
}

// Route returns the configured route for role.
func (r Router) Route(role Role) (Route, bool) {
	route, ok := r.config.Routes[role]
	return route, ok
}

// MustRoute returns role's route or the zero route when it is not configured.
func (r Router) MustRoute(role Role) Route {
	route, _ := r.Route(role)
	return route
}

// EstimateCost estimates invocation cost from the route's configured rates.
func (r Router) EstimateCost(route Route, inputTokens, outputTokens int64) CostEstimate {
	return EstimateCost(route, inputTokens, outputTokens)
}

// EstimateCost estimates invocation cost from the route's configured rates.
func EstimateCost(route Route, inputTokens, outputTokens int64) CostEstimate {
	inputTokens = nonNegative(inputTokens)
	outputTokens = nonNegative(outputTokens)

	inputCost := costForTokens(inputTokens, route.EstimatedInputCostPerMillionTokensUSD)
	outputCost := costForTokens(outputTokens, route.EstimatedOutputCostPerMillionTokensUSD)

	return CostEstimate{
		Provider:      route.Provider,
		Model:         route.Model,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		InputCostUSD:  inputCost,
		OutputCostUSD: outputCost,
		TotalCostUSD:  inputCost + outputCost,
	}
}

func costForTokens(tokens int64, costPerMillionTokensUSD float64) float64 {
	return (float64(tokens) / 1_000_000) * costPerMillionTokensUSD
}

func nonNegative(tokens int64) int64 {
	if tokens < 0 {
		return 0
	}
	return tokens
}

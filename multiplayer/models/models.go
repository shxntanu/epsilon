package models

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
				Provider:                               "frontier",
				Model:                                  "frontier-orchestrator-placeholder",
				Effort:                                 EffortHigh,
				MaxTokens:                              32_000,
				EstimatedInputCostPerMillionTokensUSD:  3.00,
				EstimatedOutputCostPerMillionTokensUSD: 15.00,
			},
			RoleWorkerDefaultCode: {
				Provider:                               "anthropic",
				Model:                                  "claude-haiku-code-worker-placeholder",
				Effort:                                 EffortMedium,
				MaxTokens:                              16_000,
				EstimatedInputCostPerMillionTokensUSD:  0.80,
				EstimatedOutputCostPerMillionTokensUSD: 4.00,
			},
			RoleWorkerDefaultArtifact: {
				Provider:                               "google",
				Model:                                  "gemini-flash-artifact-worker-placeholder",
				Effort:                                 EffortMedium,
				MaxTokens:                              16_000,
				EstimatedInputCostPerMillionTokensUSD:  0.40,
				EstimatedOutputCostPerMillionTokensUSD: 1.50,
			},
			RoleWorkerEscalation: {
				Provider:                               "frontier",
				Model:                                  "frontier-worker-escalation-placeholder",
				Effort:                                 EffortHigh,
				MaxTokens:                              32_000,
				EstimatedInputCostPerMillionTokensUSD:  3.00,
				EstimatedOutputCostPerMillionTokensUSD: 15.00,
			},
			RoleCheapClassifier: {
				Provider:                               "google",
				Model:                                  "gemini-flash-classifier-placeholder",
				Effort:                                 EffortLow,
				MaxTokens:                              2_000,
				EstimatedInputCostPerMillionTokensUSD:  0.10,
				EstimatedOutputCostPerMillionTokensUSD: 0.40,
			},
			RoleSummarizer: {
				Provider:                               "google",
				Model:                                  "gemini-flash-summarizer-placeholder",
				Effort:                                 EffortLow,
				MaxTokens:                              8_000,
				EstimatedInputCostPerMillionTokensUSD:  0.20,
				EstimatedOutputCostPerMillionTokensUSD: 0.80,
			},
			RoleMemoryDistiller: {
				Provider:                               "anthropic",
				Model:                                  "claude-haiku-memory-distiller-placeholder",
				Effort:                                 EffortLow,
				MaxTokens:                              8_000,
				EstimatedInputCostPerMillionTokensUSD:  0.80,
				EstimatedOutputCostPerMillionTokensUSD: 4.00,
			},
		},
	}
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

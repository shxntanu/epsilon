package providers

import (
	"fmt"
	"strings"

	harnessconfig "github.com/shxntanu/epsilon/core/config"
	"github.com/shxntanu/epsilon/core/providers/fake"
	"github.com/shxntanu/epsilon/core/providers/litellm"
	"github.com/shxntanu/epsilon/core/types"
)

// Config configures a core harness provider.
type Config struct {
	Name           string
	Model          string
	LiteLLMBaseURL string
	LiteLLMAPIKey  string
	ConfigStore    *harnessconfig.Store
}

// New returns a provider by name.
func New(config Config) (types.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(config.Name)) {
	case "", "fake":
		return fake.New(), nil
	case "litellm":
		provider, err := litellm.New(litellm.Config{
			BaseURL: config.LiteLLMBaseURL,
			APIKey:  config.LiteLLMAPIKey,
			Model:   config.Model,
		})
		if err != nil {
			return nil, err
		}
		cacheKey := harnessconfig.ModelCatalogKey("litellm", config.LiteLLMBaseURL)
		return harnessconfig.WrapProvider(provider, config.ConfigStore, cacheKey), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", config.Name)
	}
}

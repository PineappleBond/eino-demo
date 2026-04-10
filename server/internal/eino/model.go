package eino

import (
	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
)

// ModelProvider creates LLM chat model instances.
type ModelProvider struct {
	cfg *config.Config
}

// NewModelProvider creates a model provider from config.
func NewModelProvider(cfg *config.Config) *ModelProvider {
	return &ModelProvider{cfg: cfg}
}

// GetModel returns the LLM model config for the given tier.
func (p *ModelProvider) GetModel(tier string) config.ModelConfig {
	if mc, ok := p.cfg.Models[tier]; ok {
		return mc
	}
	return p.cfg.Models["sonnet"] // fallback
}

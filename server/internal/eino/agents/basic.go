package agents

import (
	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/tools"
)

// AgentConfig holds the runtime config for an agent.
type AgentConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
	ToolNames    []string
}

// BasicAgentBuilder builds a Basic Agent for Template 01.
type BasicAgentBuilder struct {
	cfg   *config.Config
	tools *tools.ToolRegistry
}

// NewBasicAgentBuilder creates a builder for the basic agent.
func NewBasicAgentBuilder(cfg *config.Config, toolRegistry *tools.ToolRegistry) *BasicAgentBuilder {
	return &BasicAgentBuilder{cfg: cfg, tools: toolRegistry}
}

// Build returns the agent configuration for a given model tier.
func (b *BasicAgentBuilder) Build(modelTier string) AgentConfig {
	mc := b.cfg.Models[modelTier]
	return AgentConfig{
		BaseURL:      mc.BaseURL,
		APIKey:       mc.APIKey,
		Model:        mc.Model,
		SystemPrompt: "You are a helpful assistant. Use tools when appropriate to answer questions.",
		ToolNames:    b.tools.ListToolNames(),
	}
}

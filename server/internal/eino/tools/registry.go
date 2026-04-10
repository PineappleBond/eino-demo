package tools

import (
	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
)

// ToolRegistry holds all registered Eino tools.
type ToolRegistry struct {
	cfg *config.Config
}

// NewToolRegistry creates a tool registry.
func NewToolRegistry(cfg *config.Config) *ToolRegistry {
	return &ToolRegistry{cfg: cfg}
}

// GetWeatherTool returns the registered weather tool.
func (r *ToolRegistry) GetWeatherTool() *WeatherTool {
	return NewWeatherTool()
}

// ListToolNames returns all registered tool names for agent config.
func (r *ToolRegistry) ListToolNames() []string {
	return []string{"weather"}
}

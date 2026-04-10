package tools

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
)

// ToolRegistry holds all registered Eino tools.
type ToolRegistry struct {
	cfg        *config.Config
	baseTools  []tool.BaseTool
}

// NewToolRegistry creates a tool registry.
func NewToolRegistry(cfg *config.Config) *ToolRegistry {
	r := &ToolRegistry{cfg: cfg}
	r.buildBaseTools()
	return r
}

// buildBaseTools constructs all tools as BaseTool instances.
func (r *ToolRegistry) buildBaseTools() {
	weather, err := utils.InferTool("weather", "Get current weather information for a location", func(ctx context.Context, input WeatherInput) (WeatherOutput, error) {
		return NewWeatherTool().Run(input), nil
	})
	if err != nil {
		// Should never happen with valid struct tags
		panic("failed to create weather tool: " + err.Error())
	}
	r.baseTools = []tool.BaseTool{weather}
}

// GetBaseTools returns all tools as BaseTool instances for use in agents.
func (r *ToolRegistry) GetBaseTools() []tool.BaseTool {
	return r.baseTools
}

// GetWeatherTool returns the registered weather tool.
func (r *ToolRegistry) GetWeatherTool() *WeatherTool {
	return NewWeatherTool()
}

// ListToolNames returns all registered tool names for agent config.
func (r *ToolRegistry) ListToolNames() []string {
	return []string{"weather"}
}

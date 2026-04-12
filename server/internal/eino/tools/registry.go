package tools

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
)

// ToolRegistry holds all registered Eino tools.
type ToolRegistry struct {
	cfg            *config.Config
	db             *gorm.DB
	baseTools      []tool.BaseTool
	conversationID uuid.UUID // Set per-run for context-aware tools
	workspaceDir   string    // Set per-run for filesystem tool working directory
}

// NewToolRegistry creates a tool registry.
func NewToolRegistry(cfg *config.Config, db *gorm.DB) *ToolRegistry {
	r := &ToolRegistry{cfg: cfg, db: db}
	r.buildBaseTools()
	return r
}

// buildBaseTools constructs all tools as BaseTool instances.
func (r *ToolRegistry) buildBaseTools() {
	weather, err := utils.InferTool("weather", "Get current weather information for a location", func(ctx context.Context, input WeatherInput) (WeatherOutput, error) {
		return NewWeatherTool().Run(input), nil
	})
	if err != nil {
		panic("failed to create weather tool: " + err.Error())
	}

	askUser, err := NewAskUserQuestionTool()
	if err != nil {
		panic("failed to create ask_user_question tool: " + err.Error())
	}

	r.baseTools = []tool.BaseTool{weather, askUser}
}

// SetConversationID sets the conversation ID for context-aware tools (todo_read, todo_write).
func (r *ToolRegistry) SetConversationID(conversationID uuid.UUID) {
	r.conversationID = conversationID
}

// SetWorkspaceDir sets the workspace directory for filesystem tools.
func (r *ToolRegistry) SetWorkspaceDir(workspaceDir string) {
	r.workspaceDir = workspaceDir
}

// GetBaseTools returns all tools as BaseTool instances for use in agents.
// Includes conversation-scoped tools (todo_read, todo_write) if conversationID is set.
func (r *ToolRegistry) GetBaseTools() []tool.BaseTool {
	tools := make([]tool.BaseTool, len(r.baseTools))
	copy(tools, r.baseTools)

	if r.db != nil && r.conversationID != uuid.Nil {
		todoRead, err := NewTodoReadTool(r.db, r.conversationID)
		if err != nil {
			// Tool creation failure is non-fatal; skip the tool silently.
		} else {
			tools = append(tools, todoRead)
		}

		todoWrite, err := NewTodoWriteTool(r.db, r.conversationID)
		if err != nil {
			// Tool creation failure is non-fatal; skip the tool silently.
		} else {
			tools = append(tools, todoWrite)
		}
	}

	return tools
}

// GetWeatherTool returns the registered weather tool.
func (r *ToolRegistry) GetWeatherTool() *WeatherTool {
	return NewWeatherTool()
}

// ListToolNames returns all registered tool names for agent config.
func (r *ToolRegistry) ListToolNames() []string {
	if r.conversationID != uuid.Nil {
		return []string{"weather", "ask_user_question", "todo_read", "todo_write"}
	}
	return []string{"weather", "ask_user_question"}
}

// GetPermissionTools returns filesystem and HTTP tools that implement NeedPermissioner.
// These tools require permission checks before execution.
func (r *ToolRegistry) GetPermissionTools() []tool.BaseTool {
	var tools []tool.BaseTool
	tools = append(tools, NewFilesystemTools(r.workspaceDir)...)
	if httpTools := NewHTTPTools(); httpTools != nil {
		tools = append(tools, httpTools...)
	}
	return tools
}

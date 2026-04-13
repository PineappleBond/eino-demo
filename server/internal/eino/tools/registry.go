package tools

import (
	"context"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
)

// ToolBuildContext holds per-request parameters needed to build tools.
// These values vary per agent run and must NOT be stored as struct fields.
type ToolBuildContext struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
	WorkspaceDir   string
}

// ToolRegistry holds all registered Eino tools.
type ToolRegistry struct {
	cfg                *config.Config
	db                 *gorm.DB
	baseTools          []tool.BaseTool
	syncFn             SyncPushFunc
	spawnSubAgentFn    SpawnSubAgentFunc // callback to spawn sub-agents
	registerCronTaskFn func(ctx context.Context, taskID uuid.UUID, nextRun time.Time) error
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

	if tavily := NewTavilySearchTool(r.cfg.TavilyAPIKey); tavily != nil {
		tavilyTool, err := utils.InferTool("tavily_search", "Search the web for current information using Tavily API", func(ctx context.Context, input TavilySearchInput) (string, error) {
			return tavily.Run(input), nil
		})
		if err != nil {
			panic("failed to create tavily_search tool: " + err.Error())
		}
		r.baseTools = append(r.baseTools, tavilyTool)
	}
}

// SetSyncPushFn sets the sync push function for tools that need to notify the frontend.
func (r *ToolRegistry) SetSyncPushFn(syncFn SyncPushFunc) {
	r.syncFn = syncFn
}

// SetSpawnSubAgentFunc sets the callback for spawning sub-agents.
func (r *ToolRegistry) SetSpawnSubAgentFunc(fn SpawnSubAgentFunc) {
	r.spawnSubAgentFn = fn
}

// SetRegisterCronTaskFunc sets the callback for registering cron tasks with the scheduler.
func (r *ToolRegistry) SetRegisterCronTaskFunc(fn func(ctx context.Context, taskID uuid.UUID, nextRun time.Time) error) {
	r.registerCronTaskFn = fn
}

// GetBaseTools returns all tools as BaseTool instances for use in agents.
// Includes conversation-scoped tools (todo_read, todo_write, cron_task, sub_agent)
// when conversationID is set and DB is available.
func (r *ToolRegistry) GetBaseTools(bc ToolBuildContext) []tool.BaseTool {
	tools := make([]tool.BaseTool, len(r.baseTools))
	copy(tools, r.baseTools)

	if r.db != nil && bc.ConversationID != uuid.Nil {
		todoRead, err := NewTodoReadTool(r.db, bc.ConversationID)
		if err != nil {
			// Tool creation failure is non-fatal; skip the tool silently.
		} else {
			tools = append(tools, todoRead)
		}

		todoWrite, err := NewTodoWriteTool(r.db, bc.ConversationID, bc.UserID, r.syncFn)
		if err != nil {
			// Tool creation failure is non-fatal; skip the tool silently.
		} else {
			tools = append(tools, todoWrite)
		}

		cronTask, err := NewCronTaskTool(r.db, bc.ConversationID, bc.UserID, r.syncFn, r.registerCronTaskFn)
		if err != nil {
			// Tool creation failure is non-fatal; skip the tool silently.
		} else {
			tools = append(tools, cronTask)
		}

		// Sub-agent tool: spawns a sub-conversation with an async agent.
		if r.spawnSubAgentFn != nil {
			subAgent, err := NewSubAgentTool(r.db, bc.ConversationID, bc.UserID, r.spawnSubAgentFn)
			if err != nil {
				// Tool creation failure is non-fatal; skip the tool silently.
			} else {
				tools = append(tools, subAgent)
			}
		}
	}

	return tools
}

// GetWeatherTool returns the registered weather tool.
func (r *ToolRegistry) GetWeatherTool() *WeatherTool {
	return NewWeatherTool()
}

// ListToolNames returns all registered tool names for agent config.
// Matches GetBaseTools: conversation-scoped tools require both a conversationID and a non-nil DB.
func (r *ToolRegistry) ListToolNames(bc ToolBuildContext) []string {
	if bc.ConversationID != uuid.Nil && r.db != nil {
		return []string{"weather", "tavily_search", "ask_user_question", "todo_read", "todo_write", "cron_task", "sub_agent"}
	}
	return []string{"weather", "tavily_search", "ask_user_question"}
}

// GetPermissionTools returns filesystem and HTTP tools that implement NeedPermissioner.
// These tools require permission checks before execution.
func (r *ToolRegistry) GetPermissionTools(workspaceDir string) []tool.BaseTool {
	var tools []tool.BaseTool
	tools = append(tools, NewFilesystemTools(workspaceDir)...)
	if httpTools := NewHTTPTools(); httpTools != nil {
		tools = append(tools, httpTools...)
	}
	return tools
}

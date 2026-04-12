package runner

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	openai "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/prompts"
)

// toolContext holds rendered tool metadata for template interpolation.
type toolContext struct {
	Name string
	Desc string
}

// agentContext holds rendered agent metadata for template interpolation.
type agentContext struct {
	Name string
	Desc string
}

// promptData holds all data passed to the root prompt template.
type promptData struct {
	Tools        []toolContext
	Agents       []agentContext
	OSInfo       string
	Timezone     string
	Language     string
	SystemPrompt string
}

// toolsToContext converts tool instances to template context data for rendering.
func toolsToContext(toolList []tool.BaseTool) ([]toolContext, error) {
	toolsData := make([]toolContext, 0, len(toolList))
	for _, t := range toolList {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		toolsData = append(toolsData, toolContext{
			Name: info.Name,
			Desc: strings.TrimSpace(info.Desc),
		})
	}
	return toolsData, nil
}

// renderRootPrompt renders the root prompt template with tool/agent metadata.
func renderRootPrompt(toolList []tool.BaseTool, subAgents []adk.Agent, systemPrompt string) (string, error) {
	tz, _ := time.Now().Local().Zone()
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "zh_CN.UTF-8"
	}

	toolsData, err := toolsToContext(toolList)
	if err != nil {
		return "", err
	}

	agentsData := make([]agentContext, 0, len(subAgents))
	for _, a := range subAgents {
		agentsData = append(agentsData, agentContext{
			Name: a.Name(context.Background()),
			Desc: strings.TrimSpace(a.Description(context.Background())),
		})
	}

	return prompts.Render("root", promptData{
		Tools:        toolsData,
		Agents:       agentsData,
		OSInfo:       runtime.GOOS + "/" + runtime.GOARCH,
		Timezone:     tz,
		Language:     lang,
		SystemPrompt: systemPrompt,
	})
}

// RootRunnerConfig holds all inputs needed to build a RootRunner for one conversation.
type RootRunnerConfig struct {
	// ModelProvider supplies LLM configs by tier.
	ModelProvider *eino.ModelProvider
	// ModelTier selects which LLM to use ("haiku", "sonnet", "opus").
	ModelTier string
	// SystemPrompt is the instruction for the root agent (from template config).
	SystemPrompt string
	// Tools are the concrete tool instances available to the agent.
	Tools []tool.BaseTool
	// SubAgents are optional nested agents (can be nil/empty).
	SubAgents []adk.Agent
	// MaxIteration limits reasoning loops. 0 means default (100).
	MaxIteration int
	// ReductionEnabled enables the reduction middleware, which proactively
	// clears old tool results from context when it grows too large.
	ReductionEnabled bool
	// ConversationID identifies the conversation for message injection.
	ConversationID uuid.UUID
	// MessageQueue holds pending messages to inject at model call boundaries.
	// Nil means no message injection.
	MessageQueue *MessageQueue
	// TokenCheck configures token overflow detection before each model call.
	// Nil means no token checking.
	TokenCheck *TokenCheckConfig
	// SummarizationCallback is called when the Summarization middleware compresses context.
	// Args: (ctx, compressedMsgCount). The caller queries DB to determine the new min_seq.
	// Nil disables summarization sync.
	SummarizationCallback func(ctx context.Context, compressedMsgCount int)
}

// RootRunner holds one execution instance. Created fresh per Run.
type RootRunner struct {
	runner *adk.Runner
}

// RootRunnerCallback is implemented by the caller to receive events and manage checkpoints.
type RootRunnerCallback interface {
	RootRunnerHandlerCallback
	adk.CheckPointStore
	OnError(err error)
	OnEnd()
	OnInterrupted(info *adk.InterruptInfo)
}

// NewRootRunner creates a fresh RootRunner for a single conversation run.
func NewRootRunner(ctx context.Context, cfg RootRunnerConfig, callback RootRunnerCallback) (*RootRunner, error) {
	// 1. Create ChatModel from ModelProvider.
	mc := cfg.ModelProvider.GetModel(cfg.ModelTier)
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
	})
	if err != nil {
		return nil, err
	}

	maxIter := cfg.MaxIteration
	if maxIter <= 0 {
		maxIter = 100
	}

	// 2. Render instruction — template includes SystemPrompt via {{ .SystemPrompt }}.
	instruction, err := renderRootPrompt(cfg.Tools, cfg.SubAgents, cfg.SystemPrompt)
	if err != nil {
		return nil, err
	}

	// 3. Build handlers (middleware chain).
	// Order: Summarization → Context injection → Reduction
	var handlers []adk.ChatModelAgentMiddleware

	// 3a. Summarization middleware — auto-compresses conversation history when tokens
	// exceed threshold. Syncs compressed state to DB via callback.
	if cfg.SummarizationCallback != nil {
		summarizationMW, err := summarization.New(ctx, &summarization.Config{
			Model: chatModel,
			Trigger: &summarization.TriggerCondition{
				ContextTokens: 80000, // Sync with maxPromptTokens in chat.go
			},
			Callback: func(cbCtx context.Context, before, after adk.ChatModelAgentState) error {
				// Calculate min_seq: find the last non-system message in before.Messages
				// and use its seq as the boundary. The callback in chat.go handles
				// the DB write.
				lastNonSystemIdx := -1
				for i, m := range before.Messages {
					if m != nil && m.Role != schema.System {
						lastNonSystemIdx = i
					}
				}
				if lastNonSystemIdx >= 0 {
					cfg.SummarizationCallback(cbCtx, lastNonSystemIdx+1)
				}
				return nil
			},
		})
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, summarizationMW)
	}

	// 3b. Context injection middleware (message injection from queue + token check).
	if cfg.MessageQueue != nil && cfg.ConversationID != uuid.Nil {
		handlers = append(handlers, NewContextInjectionMiddleware(cfg.MessageQueue, cfg.ConversationID, cfg.TokenCheck))
	}

	// 3c. Reduction middleware (proactively clears old tool results).
	if cfg.ReductionEnabled {
		reductionMW, err := reduction.New(ctx, &reduction.Config{
			SkipTruncation:            true, // token overflow check is the primary defense; reduction only clears old tool results
			MaxTokensForClear:         100000,
			ClearRetentionSuffixLimit: 2,
			RootDir:                   "/tmp/eino-reduction",
			ReadFileToolName:          "read_file",
		})
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, reductionMW)
	}

	// 4. Build DeepAgent.
	deepAgent, err := deep.New(ctx, &deep.Config{
		Name:        "root",
		Description: "Root agent for the chat flow",
		ChatModel:   chatModel,
		Instruction: instruction,
		SubAgents:   cfg.SubAgents,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               cfg.Tools,
				UnknownToolsHandler: unknownToolsHandler,
			},
			EmitInternalEvents: true,
		},
		MaxIteration:      maxIter,
		Handlers:          handlers,
		WithoutWriteTodos: true,
	})
	if err != nil {
		return nil, err
	}

	// 4. Build Runner with streaming and checkpoint store.
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           deepAgent,
		EnableStreaming: true,
		CheckPointStore: callback,
	})

	return &RootRunner{runner: runner}, nil
}

// Run starts a fresh agent run with the given messages.
func (r *RootRunner) Run(ctx context.Context, messages []*schema.Message, checkpointID string, handler *RootRunnerHandler) *adk.AsyncIterator[*adk.AgentEvent] {
	opts := []adk.AgentRunOption{
		adk.WithCallbacks(handler),
	}
	if checkpointID != "" {
		opts = append(opts, adk.WithCheckPointID(checkpointID))
	}
	return r.runner.Run(ctx, messages, opts...)
}

// Resume continues an interrupted run from a checkpoint.
func (r *RootRunner) Resume(ctx context.Context, checkpointID string, handler *RootRunnerHandler) (*adk.AsyncIterator[*adk.AgentEvent], error) {
	opts := []adk.AgentRunOption{
		adk.WithCallbacks(handler),
	}
	return r.runner.Resume(ctx, checkpointID, opts...)
}

// ResumeWithParams continues an interrupted run with targeted resume data.
func (r *RootRunner) ResumeWithParams(ctx context.Context, checkpointID string, params *adk.ResumeParams, handler *RootRunnerHandler) (*adk.AsyncIterator[*adk.AgentEvent], error) {
	opts := []adk.AgentRunOption{
		adk.WithCallbacks(handler),
	}
	return r.runner.ResumeWithParams(ctx, checkpointID, params, opts...)
}

// ---- Context helpers ----

// AddrString converts compose.Address to a readable string for Update payloads.
func AddrString(addr compose.Address) string {
	if len(addr) == 0 {
		return ""
	}
	return addr[0].ID
	//segs := make([]string, len(addr))
	//for i, seg := range addr {
	//	if seg.SubID != "" {
	//		segs[i] = string(seg.Type) + ":" + seg.ID + ":" + seg.SubID
	//	} else {
	//		segs[i] = string(seg.Type) + ":" + seg.ID
	//	}
	//}
	//result := ""
	//for i, s := range segs {
	//	if i > 0 {
	//		result += ";"
	//	}
	//	result += s
	//}
	//return result
}

// NowFunc returns current time. Exposed for testing.
var NowFunc = time.Now

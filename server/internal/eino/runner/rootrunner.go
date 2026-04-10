package runner

import (
	"context"
	"os"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	openai "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
)

// RootRunnerPromptTmpl is the Go text/template for the root agent system prompt.
// Syntax: Go text/template ({{range}}, {{.Field}}) — no external dependencies.
const RootRunnerPromptTmpl = `
# 角色

你是一个专业的 AI 助手，擅长利用可用工具解决问题。

## 你的特点

- **善于利用工具**：你拥有丰富的工具集，能够灵活组合使用它们来解决复杂问题
- **全力以赴**：你不惜一切成本为人类解决问题，不轻言放弃
- **系统性思维**：面对复杂任务时，你会将其拆解为多个步骤，逐步使用工具推进

## 行为准则

1. 充分理解用户的需求和意图
2. 优先使用工具获取准确信息，而非凭空猜测
3. 遇到复杂问题时，分步骤逐步解决
4. 每次工具调用后，仔细分析返回结果，决定下一步行动
5. 如果一次尝试未能解决问题，换一种方法继续尝试

# 可用工具

以下是你可以使用的工具列表：

| 工具名称 | 简介 |
| :--- | :--- |
{{ range .Tools }}| {{ .Name }} | {{ .Desc }} |
{{ end }}
# 可用子代理

以下是你可以调用的子代理列表：

**注意⚠️：调用子代理时你必须把你润色过的提示词写入description字段**

| 代理名称 | 简介 |
| :--- | :--- |
{{ range .Agents }}| {{ .Name }} | {{ .Desc }} |
{{ end }}
# 运行环境

- 操作系统: {{ .OSInfo }}
- 时区: {{ .Timezone }}
- 语言: {{ .Language }}
`

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

// promptData holds all data passed to the RootRunnerPromptTmpl.
type promptData struct {
	Tools    []toolContext
	Agents   []agentContext
	OSInfo   string
	Timezone string
	Language string
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

// renderRootPrompt renders the RootRunnerPromptTmpl with tool and agent metadata.
func renderRootPrompt(toolList []tool.BaseTool, subAgents []adk.Agent) (string, error) {
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

	osInfo := runtime.GOOS + "/" + runtime.GOARCH

	tpl, err := template.New("root_prompt").Parse(RootRunnerPromptTmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tpl.Execute(&buf, promptData{
		Tools:    toolsData,
		Agents:   agentsData,
		OSInfo:   osInfo,
		Timezone: tz,
		Language: lang,
	}); err != nil {
		return "", err
	}

	return strings.TrimSpace(buf.String()), nil
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

	// 2. Render instruction with tool/agent metadata, then append template system prompt.
	rendered, err := renderRootPrompt(cfg.Tools, cfg.SubAgents)
	if err != nil {
		return nil, err
	}
	instruction := rendered + "\n\n" + cfg.SystemPrompt

	// 3. Build DeepAgent.
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
		MaxIteration: maxIter,
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

// ---- Context helpers ----

// AddrString converts compose.Address to a readable string for Update payloads.
func AddrString(addr compose.Address) string {
	if len(addr) == 0 {
		return ""
	}
	segs := make([]string, len(addr))
	for i, seg := range addr {
		if seg.SubID != "" {
			segs[i] = string(seg.Type) + ":" + seg.ID + ":" + seg.SubID
		} else {
			segs[i] = string(seg.Type) + ":" + seg.ID
		}
	}
	result := ""
	for i, s := range segs {
		if i > 0 {
			result += ";"
		}
		result += s
	}
	return result
}

// NowFunc returns current time. Exposed for testing.
var NowFunc = time.Now

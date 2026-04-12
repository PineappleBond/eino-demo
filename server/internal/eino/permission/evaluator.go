package permission

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/prompts"
)

const (
	evaluateToolName  = "evaluate_permission"
	evaluateToolDesc  = "将权限请求的安全评估结果写入此工具"
	evaluateAgentName = "permission_evaluator"
	evaluateAgentDesc = "评估工具调用的安全风险等级"
	evaluateTimeout   = 30 * time.Second
)

// EvaluateTool holds a channel for receiving safety evaluations from the agent.
type EvaluateTool struct {
	ch chan SafetyEvaluation
}

// LLMReviewer implements SafetyEvaluator using a ChatModelAgent with tools.
type LLMReviewer struct {
	chatModel      model.ToolCallingChatModel
	chatModelAgent *adk.ChatModelAgent
	evaluateTool   *EvaluateTool
}

// evalContext holds project environment info for the evaluator prompt.
type evalContext struct {
	OSInfo       string
	Shell        string
	WorkspaceDir string
	IsGitRepo    bool
}

// NewLLMReviewer creates a safety evaluator backed by a ChatModelAgent.
func NewLLMReviewer(chatModel model.ToolCallingChatModel, workspaceDir string, isGitRepo bool) *LLMReviewer {
	evaluateTool := &EvaluateTool{
		ch: make(chan SafetyEvaluation, 1),
	}

	evaluateToolWrapper, err := utils.InferTool(evaluateToolName, evaluateToolDesc, func(ctx context.Context, input SafetyEvaluation) (SafetyEvaluation, error) {
		evaluateTool.ch <- input
		return input, nil
	})
	if err != nil {
		panic("failed to create evaluate tool: " + err.Error())
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}

	instruction, err := prompts.Render("evaluator", evalContext{
		OSInfo:       runtime.GOOS + "/" + runtime.GOARCH,
		Shell:        shell,
		WorkspaceDir: workspaceDir,
		IsGitRepo:    isGitRepo,
	})
	if err != nil {
		panic("failed to render evaluator prompt: " + err.Error())
	}

	chatModelAgent, err := adk.NewChatModelAgent(
		context.Background(),
		&adk.ChatModelAgentConfig{
			Name:        evaluateAgentName,
			Description: evaluateAgentDesc,
			Instruction: instruction,
			Model:       chatModel,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{
					Tools: []tool.BaseTool{
						evaluateToolWrapper,
					},
				},
				EmitInternalEvents: true,
			},
			MaxIterations: 10,
		},
	)
	if err != nil {
		panic("failed to create evaluate agent: " + err.Error())
	}

	return &LLMReviewer{
		chatModel:      chatModel,
		chatModelAgent: chatModelAgent,
		evaluateTool:   evaluateTool,
	}
}

// Evaluate runs a ChatModelAgent that analyzes the permission request and
// outputs a SafetyEvaluation by calling the evaluate tool.
func (r *LLMReviewer) Evaluate(pCtx context.Context, req *PermissionRequest) (*SafetyEvaluation, error) {
	userMsg := fmt.Sprintf(
		"请评估以下工具调用的安全风险等级：\n\n"+
			"- 工具名称: %s\n"+
			"- 工具描述: %s\n"+
			"- 操作: %s\n"+
			"- 内容: %s\n"+
			"- 参数摘要: %s\n"+
			"- 工具自评等级: %d",
		req.ToolName, req.ToolDesc, req.Action, req.Content, req.ArgsSummary, req.ToolLevel,
	)

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Duration(evaluateTimeout))
	defer cancelFunc()

	iter := r.chatModelAgent.Run(ctx, &adk.AgentInput{
		Messages: []*schema.Message{
			schema.UserMessage(userMsg),
		},
	})

	done := make(chan struct{}, 1)

	go func() {
		defer func() {
			done <- struct{}{}
		}()
		for {
			_, ok := iter.Next()
			if !ok {
				return
			}
		}
	}()

	select {
	case se := <-r.evaluateTool.ch:
		if se.Level < 1 || se.Level > 4 {
			return &SafetyEvaluation{
				Level:  3,
				Reason: "AI 评估的安全等级无效，按中等风险处理",
			}, nil
		}
		return &se, nil
	case <-ctx.Done():
		return &SafetyEvaluation{
			Level:  3,
			Reason: "评估超时，按中等风险处理",
		}, nil
	case <-time.After(evaluateTimeout):
		return &SafetyEvaluation{
			Level:  3,
			Reason: "评估超时，按中等风险处理",
		}, nil
	case <-done:
		return &SafetyEvaluation{
			Level:  3,
			Reason: "评估超时，按中等风险处理",
		}, nil
	}
}

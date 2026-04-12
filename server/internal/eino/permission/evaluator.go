package permission

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// LLMReviewer implements SafetyEvaluator using an LLM.
type LLMReviewer struct {
	chatModel model.ToolCallingChatModel
}

// NewLLMReviewer creates a safety evaluator backed by an LLM.
func NewLLMReviewer(chatModel model.ToolCallingChatModel) *LLMReviewer {
	return &LLMReviewer{chatModel: chatModel}
}

// Evaluate sends the permission request to the LLM and parses the safety assessment.
func (r *LLMReviewer) Evaluate(ctx context.Context, req *PermissionRequest) (*SafetyEvaluation, error) {
	prompt, err := renderEvaluatorPrompt(req)
	if err != nil {
		return nil, fmt.Errorf("render evaluator prompt: %w", err)
	}

	resp, err := r.chatModel.Generate(ctx, []*schema.Message{
		schema.UserMessage(prompt),
	})
	if err != nil {
		return nil, fmt.Errorf("generate safety evaluation: %w", err)
	}

	var eval SafetyEvaluation
	if err := json.Unmarshal([]byte(resp.Content), &eval); err != nil {
		// LLM returned non-JSON — default to level 3.
		return &SafetyEvaluation{
			Level:  3,
			Reason: "AI 评估返回格式异常，按中等风险处理",
		}, nil
	}

	if eval.Level < 1 || eval.Level > 4 {
		return &SafetyEvaluation{
			Level:  3,
			Reason: "AI 评估的安全等级无效，按中等风险处理",
		}, nil
	}

	return &eval, nil
}

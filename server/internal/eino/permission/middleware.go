package permission

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// MiddlewareConfig holds all inputs for the permission middleware.
type MiddlewareConfig struct {
	DB             *gorm.DB
	ProjectID      uuid.UUID
	UserID         uuid.UUID
	ConversationID uuid.UUID
	Threshold      int
	Evaluator      SafetyEvaluator
	Tools          []tool.BaseTool
	PushUpdate     func(userID uuid.UUID, update model.UserUpdate)
	NextSeq        func(ctx context.Context, userID uuid.UUID) (int64, error)
}

// Middleware implements ChatModelAgentMiddleware to intercept tool calls for permission checks.
type Middleware struct {
	*adk.BaseChatModelAgentMiddleware
	cfg       MiddlewareConfig
	checker   *Checker
	permTools map[string]NeedPermissioner
}

// NewMiddleware creates a new permission middleware.
func NewMiddleware(cfg MiddlewareConfig) *Middleware {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 2
	}

	checker := NewChecker(cfg.DB, cfg.Evaluator, cfg.ProjectID, cfg.UserID, cfg.Threshold)

	permTools := make(map[string]NeedPermissioner)
	for _, t := range cfg.Tools {
		if np, ok := t.(NeedPermissioner); ok {
			info, err := t.Info(context.Background())
			if err != nil {
				continue
			}
			permTools[info.Name] = np
		}
	}

	return &Middleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		cfg:       cfg,
		checker:   checker,
		permTools: permTools,
	}
}

// WrapInvokableToolCall wraps tool invocation with permission checking.
func (m *Middleware) WrapInvokableToolCall(ctx context.Context, next compose.InvokableToolEndpoint, tCtx *adk.ToolContext) (compose.InvokableToolEndpoint, error) {
	return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		return m.handleToolCall(ctx, next, input)
	}, nil
}

// WrapStreamableToolCall wraps streamable tool invocation with permission checking.
func (m *Middleware) WrapStreamableToolCall(ctx context.Context, next compose.StreamableToolEndpoint, tCtx *adk.ToolContext) (compose.StreamableToolEndpoint, error) {
	return func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
		// Permission check happens before streaming.
		_, err := m.handleToolCall(ctx, nil, input)
		if err != nil {
			return nil, err
		}
		// Allowed — proceed with streaming.
		return next(ctx, input)
	}, nil
}

func (m *Middleware) handleToolCall(ctx context.Context, next compose.InvokableToolEndpoint, input *compose.ToolInput) (*compose.ToolOutput, error) {
	toolName := input.Name

	// Skip if tool doesn't need permission checking.
	np, needsPerm := m.permTools[toolName]
	if !needsPerm {
		if next != nil {
			return next(ctx, input)
		}
		return nil, nil
	}

	// Check if resuming from a permission interrupt.
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if wasInterrupted {
		return m.handleResume(ctx, next, input)
	}

	// First invocation — get PermissionRequest from tool.
	var args map[string]any
	_ = json.Unmarshal([]byte(input.Arguments), &args)
	req := np.NeedPermission(args)
	if req == nil {
		if next != nil {
			return next(ctx, input)
		}
		return nil, nil
	}

	// Fill metadata.
	req.ToolName = toolName
	if req.ArgsSummary == "" {
		req.ArgsSummary = truncateJSON(input.Arguments, 200)
	}

	// Check whitelist.
	if m.checker.isWhitelisted(req.ToolName, req.Action, req.Content) {
		if next != nil {
			return next(ctx, input)
		}
		return nil, nil
	}

	// Safety evaluation.
	eval, evalErr := m.evaluateSafely(ctx, req)
	if evalErr != nil {
		eval = &SafetyEvaluation{Level: 3, Reason: "AI 评估失败，按中等风险处理"}
	}

	// Compare against threshold.
	if eval.Level <= m.checker.threshold {
		if next != nil {
			return next(ctx, input)
		}
		return nil, nil
	}

	// Exceeds threshold — interrupt for human approval.
	return nil, m.interruptForPermission(ctx, req, eval)
}

func (m *Middleware) handleResume(ctx context.Context, next compose.InvokableToolEndpoint, input *compose.ToolInput) (*compose.ToolOutput, error) {
	isTarget, hasData, data := tool.GetResumeContext[string](ctx)
	if !isTarget || !hasData {
		// Not our resume — re-interrupt.
		return nil, tool.Interrupt(ctx, nil)
	}

	// Decode resume data.
	var resumeInfo map[string]string
	if err := json.Unmarshal([]byte(data), &resumeInfo); err != nil {
		// Try raw string.
		resumeInfo = map[string]string{"decision": data}
	}

	decision := MapAnswerToDecision(resumeInfo["decision"])

	// Get the original permission request from interrupt context.
	_, _, rawPayload := tool.GetInterruptState[map[string]any](ctx)
	req := &PermissionRequest{
		ToolName:    getString(rawPayload, "tool_name"),
		Action:      getString(rawPayload, "action"),
		Content:     getString(rawPayload, "content"),
		ToolDesc:    getString(rawPayload, "tool_desc"),
		ArgsSummary: getString(rawPayload, "args_summary"),
	}

	result := m.checker.Check(ctx, req, string(decision))

	if !result.Allowed {
		return nil, fmt.Errorf("permission denied: %s 调用被用户拒绝", req.ToolName)
	}

	// Write whitelist if needed.
	if result.ShouldWriteWhitelist {
		if err := m.checker.WriteWhitelist(ctx, req, result.Wildcard); err != nil {
			// Log but don't block.
		}
	}

	// Push permission.decided update.
	if m.cfg.PushUpdate != nil && m.cfg.NextSeq != nil {
		seq, _ := m.cfg.NextSeq(ctx, m.cfg.UserID)
		if seq > 0 {
			m.cfg.PushUpdate(m.cfg.UserID, model.UserUpdate{
				UserID: m.cfg.UserID,
				Seq:    seq,
				Type:   "permission.decided",
				Payload: model.JSONMap{
					"conversation_id": m.cfg.ConversationID.String(),
					"decision":        string(result.Decision),
					"seq":             seq,
				},
			})
		}
	}

	// Proceed with tool call.
	return next(ctx, input)
}

func (m *Middleware) interruptForPermission(ctx context.Context, req *PermissionRequest, eval *SafetyEvaluation) error {
	question := fmt.Sprintf("Agent 想要调用 %s（%s），是否允许？\n\n操作类型: %s\n详情: %s\n安全评估: 等级 %d — %s",
		req.ToolName, req.ToolDesc, req.Action, req.Content, eval.Level, eval.Reason)

	choices := []map[string]string{
		{"title": "同意", "desc": "允许此次操作"},
		{"title": "同意并记住", "desc": "允许，且同一 Project 下该操作不再询问（精确匹配）"},
		{"title": "同意并通配记住", "desc": "允许，且同类操作不再询问（通配符匹配）"},
		{"title": "拒绝", "desc": "不允许此次操作"},
	}

	// Create HumanInPermission record.
	perm, err := CreatePendingPerm(m.cfg.DB, m.cfg.ConversationID, req.ToolName, req.Action, req.Content, req.ToolDesc, req.ArgsSummary, eval.Level, eval.Reason)
	if err != nil {
		// Continue anyway — the interrupt will still work.
	}

	// Push permission.pending update.
	if m.cfg.PushUpdate != nil && m.cfg.NextSeq != nil {
		seq, _ := m.cfg.NextSeq(ctx, m.cfg.UserID)
		if seq > 0 {
			m.cfg.PushUpdate(m.cfg.UserID, model.UserUpdate{
				UserID: m.cfg.UserID,
				Seq:    seq,
				Type:   "permission.pending",
				Payload: model.JSONMap{
					"conversation_id": m.cfg.ConversationID.String(),
					"permission_id":   perm.ID.String(),
					"tool_name":       req.ToolName,
					"action":          req.Action,
					"content":         req.Content,
					"tool_desc":       req.ToolDesc,
					"args_summary":    req.ArgsSummary,
					"safety_level":    eval.Level,
					"safety_reason":   eval.Reason,
					"seq":             seq,
				},
			})
		}
	}

	_ = choices // available for frontend UI

	return tool.Interrupt(ctx, map[string]any{
		"type":           "permission_request",
		"tool_name":      req.ToolName,
		"action":         req.Action,
		"content":        req.Content,
		"tool_desc":      req.ToolDesc,
		"args_summary":   req.ArgsSummary,
		"safety_level":   eval.Level,
		"safety_reason":  eval.Reason,
		"question":       question,
		"answer_type":    "single",
		"choices":        choices,
	})
}

func (m *Middleware) evaluateSafely(ctx context.Context, req *PermissionRequest) (*SafetyEvaluation, error) {
	if m.checker.evaluator == nil {
		return &SafetyEvaluation{Level: 3, Reason: "无评估器，按中等风险处理"}, nil
	}
	return m.checker.evaluator.Evaluate(ctx, req)
}

func truncateJSON(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

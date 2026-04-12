package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

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
		cfg:                          cfg,
		checker:                      checker,
		permTools:                    permTools,
	}
}

// WrapInvokableToolCall wraps tool invocation with permission checking.
func (m *Middleware) WrapInvokableToolCall(ctx context.Context, next adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		// Check permission before invoking.
		allowed, err := m.checkPermission(ctx, tCtx.Name, argumentsInJSON)
		if err != nil {
			return "", err
		}
		if !allowed {
			return "", nil // Interrupt was called, return empty.
		}
		return next(ctx, argumentsInJSON, opts...)
	}, nil
}

// WrapStreamableToolCall wraps streamable tool invocation with permission checking.
func (m *Middleware) WrapStreamableToolCall(ctx context.Context, next adk.StreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		// Permission check happens before streaming.
		allowed, err := m.checkPermission(ctx, tCtx.Name, argumentsInJSON)
		if err != nil {
			return nil, err
		}
		if !allowed {
			// Return a stream with a single empty chunk instead of nil.
			// Eino's callback framework calls Copy() on the returned reader (nil would panic),
			// and concatStreamReader requires at least 1 chunk (empty stream would fail).
			sr, sw := schema.Pipe[string](1)
			sw.Send("", nil)
			sw.Close()
			return sr, nil
		}
		return next(ctx, argumentsInJSON, opts...)
	}, nil
}

// checkPermission returns (shouldProceed, error).
// If error is non-nil, the tool call was interrupted (Interrupt was called).
// If shouldProceed is false and error is nil, the tool was denied.
func (m *Middleware) checkPermission(ctx context.Context, toolName, argumentsInJSON string) (bool, error) {
	// Skip if tool doesn't need permission checking.
	np, needsPerm := m.permTools[toolName]
	if !needsPerm {
		return true, nil
	}

	// Check if resuming from a permission interrupt.
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if wasInterrupted {
		return m.handleResume(ctx, toolName, argumentsInJSON)
	}

	// First invocation — get PermissionRequest from tool.
	var args map[string]any
	_ = json.Unmarshal([]byte(argumentsInJSON), &args)
	req := np.NeedPermission(args)
	if req == nil {
		return true, nil
	}

	// Fill metadata.
	req.ToolName = toolName
	if req.ArgsSummary == "" {
		req.ArgsSummary = truncateJSON(argumentsInJSON, 200)
	}

	// Check whitelist.
	if m.checker.isWhitelisted(req.ToolName, req.Action, req.Content) {
		return true, nil
	}

	// Safety evaluation.
	eval, evalErr := m.evaluateSafely(ctx, req)
	if evalErr != nil {
		eval = &SafetyEvaluation{Level: 3, Reason: "AI 评估失败，按中等风险处理"}
	}

	// Compare against threshold.
	if eval.Level <= m.checker.threshold {
		return true, nil
	}

	// Exceeds threshold — interrupt for human approval.
	return false, m.interruptForPermission(ctx, req, eval)
}

func (m *Middleware) handleResume(ctx context.Context, toolName, argumentsInJSON string) (bool, error) {
	isTarget, hasData, data := tool.GetResumeContext[string](ctx)
	if !isTarget || !hasData {
		// Not our resume — re-interrupt.
		_ = tool.Interrupt(ctx, nil)
		return false, fmt.Errorf("permission resume interrupted")
	}

	// Decode resume data.
	var resumeInfo map[string]string
	if err := json.Unmarshal([]byte(data), &resumeInfo); err != nil {
		resumeInfo = map[string]string{"decision": data}
	}

	decision := MapAnswerToDecision(resumeInfo["decision"])

	// Get the original permission request from interrupt context.
	_, _, rawPayload := tool.GetInterruptState[map[string]any](ctx)
	permID := getString(rawPayload, "permission_id")
	req := &PermissionRequest{
		ToolName:    getString(rawPayload, "tool_name"),
		Action:      getString(rawPayload, "action"),
		Content:     getString(rawPayload, "content"),
		ToolDesc:    getString(rawPayload, "tool_desc"),
		ArgsSummary: getString(rawPayload, "args_summary"),
	}

	result := m.checker.Check(ctx, req, string(decision))

	if !result.Allowed {
		return false, fmt.Errorf("permission denied: %s 调用被用户拒绝", req.ToolName)
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
					"permission_id":   permID,
					"decision":        string(result.Decision),
					"seq":             seq,
				},
			})
		}
	}

	return true, nil
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

	// Execute the Eino Interrupt. This generates a new InterruptSignal with a unique ID.
	// The InterruptSignal.ID is the key that Eino uses for resume targeting via BatchResumeWithData.
	// We must extract it and include in the permission.pending update so the frontend can
	// pass it back as the resume target.
	interruptErr := tool.Interrupt(ctx, map[string]any{
		"type":          "permission_request",
		"tool_name":     req.ToolName,
		"action":        req.Action,
		"content":       req.Content,
		"tool_desc":     req.ToolDesc,
		"args_summary":  req.ArgsSummary,
		"safety_level":  eval.Level,
		"safety_reason": eval.Reason,
		"question":      question,
		"answer_type":   "single",
		"choices":       choices,
		"permission_id": perm.ID.String(),
	})

	// Extract the InterruptSignal ID from the returned error.
	// The error implements adk.InterruptContextsProvider (GetInterruptContexts()).
	var interruptID string
	if interruptErr != nil {
		var provider interface {
			GetInterruptContexts() []*adk.InterruptCtx
		}
		if ok := errors.As(interruptErr, &provider); ok && provider != nil {
			ctxs := provider.GetInterruptContexts()
			if len(ctxs) > 0 {
				// Last context is the most specific — the actual interrupt source.
				interruptID = ctxs[len(ctxs)-1].ID
			}
		}
	}

	// Push permission.pending update with Eino's InterruptSignal.ID.
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
					"checkpoint_id":   perm.CheckpointID,
					"interrupt_id":    interruptID,
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

	return interruptErr
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

// matchPattern checks if a pattern (exact or glob) matches the content.
func matchPattern(pattern, content string) (bool, error) {
	return filepath.Match(pattern, content)
}

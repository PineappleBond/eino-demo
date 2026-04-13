package permission

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// Checker performs permission checks against whitelists and the safety evaluator.
type Checker struct {
	db        *gorm.DB
	evaluator SafetyEvaluator
	projectID uuid.UUID
	userID    uuid.UUID
	threshold int
}

// NewChecker creates a permission checker.
func NewChecker(db *gorm.DB, evaluator SafetyEvaluator, projectID, userID uuid.UUID, threshold int) *Checker {
	if threshold <= 0 {
		threshold = 2
	}
	return &Checker{
		db:        db,
		evaluator: evaluator,
		projectID: projectID,
		userID:    userID,
		threshold: threshold,
	}
}

// CheckResult is returned by Checker.Check.
type CheckResult struct {
	// Allowed is true if the tool call should proceed.
	Allowed bool
	// Interrupt is true if the call should be interrupted for human approval.
	Interrupt bool
	// Eval contains the safety evaluation (may be nil for whitelisted or resumed calls).
	Eval *SafetyEvaluation
	// Decision is the user's resumed decision (non-empty when resuming).
	Decision Decision
	// ShouldWriteWhitelist is true when the decision should be persisted.
	ShouldWriteWhitelist bool
	// Wildcard indicates the whitelist entry should use a wildcard pattern.
	Wildcard bool
}

// Check evaluates a tool call permission. If resumeDecision is non-empty, it applies the decision directly.
func (c *Checker) Check(ctx context.Context, req *PermissionRequest, resumeDecision string) CheckResult {
	// Resume path.
	if resumeDecision != "" {
		return c.applyDecision(Decision(resumeDecision))
	}

	// Check whitelist.
	if c.isWhitelisted(req.ToolName, req.Action, req.Content) {
		return CheckResult{Allowed: true}
	}

	// Safety evaluation.
	var eval *SafetyEvaluation
	if c.evaluator != nil {
		var err error
		eval, err = c.evaluator.Evaluate(ctx, req)
		if err != nil {
			eval = &SafetyEvaluation{Level: 3, Reason: "AI 评估失败，按中等风险处理"}
		}
	} else {
		eval = &SafetyEvaluation{Level: 3, Reason: "无评估器，按中等风险处理"}
	}

	// Compare against threshold.
	if eval.Level <= c.threshold {
		return CheckResult{Allowed: true, Eval: eval}
	}

	// Exceeds threshold — interrupt.
	return CheckResult{Interrupt: true, Eval: eval}
}

func (c *Checker) applyDecision(d Decision) CheckResult {
	switch d {
	case DecisionApproved:
		return CheckResult{Allowed: true, Decision: d}
	case DecisionApprovedExact:
		return CheckResult{Allowed: true, Decision: d, ShouldWriteWhitelist: true, Wildcard: false}
	case DecisionApprovedWildcard:
		return CheckResult{Allowed: true, Decision: d, ShouldWriteWhitelist: true, Wildcard: true}
	default:
		return CheckResult{Allowed: false, Decision: DecisionDenied}
	}
}

// isWhitelisted checks if the tool call matches a project whitelist entry.
func (c *Checker) isWhitelisted(toolName, action, content string) bool {
	var patterns []struct {
		Pattern string
		Level   string
	}
	if err := c.db.Table("project_tool_permissions").
		Select("pattern, level").
		Where("project_id = ? AND tool_name = ? AND action = ?", c.projectID, toolName, action).
		Find(&patterns).Error; err != nil {
		return false
	}

	for _, p := range patterns {
		if matched, _ := matchPattern(p.Pattern, content); matched {
			return true
		}
	}
	return false
}

// WriteWhitelist adds a new permission entry.
func (c *Checker) WriteWhitelist(ctx context.Context, req *PermissionRequest, wildcard bool) error {
	pattern := req.Content

	var count int64
	c.db.Table("project_tool_permissions").
		Where("project_id = ? AND tool_name = ? AND action = ? AND pattern = ?",
			c.projectID, req.ToolName, req.Action, pattern).
		Count(&count)
	if count > 0 {
		return nil
	}

	level := "exact"
	if wildcard {
		level = "wildcard"
	}

	return c.db.Table("project_tool_permissions").
		Create(map[string]interface{}{
			"id":         uuid.New().String(),
			"project_id": c.projectID,
			"tool_name":  req.ToolName,
			"action":     req.Action,
			"pattern":    pattern,
			"granted_by": c.userID,
			"level":      level,
		}).Error
}

// MapAnswerToDecision maps the user's answer text to a Decision.
func MapAnswerToDecision(answer string) Decision {
	switch answer {
	case "同意", string(DecisionApproved):
		return DecisionApproved
	case "同意并记住", string(DecisionApprovedExact):
		return DecisionApprovedExact
	case "同意并通配记住", string(DecisionApprovedWildcard):
		return DecisionApprovedWildcard
	default:
		return DecisionDenied
	}
}

// CreatePendingPerm creates a HumanInPermission record in the database.
func CreatePendingPerm(db *gorm.DB, convID uuid.UUID, toolName, action, content, toolDesc, argsSummary string, safetyLevel int, safetyReason string, sourceConversationID *uuid.UUID) (*model.HumanInPermission, error) {
	perm := &model.HumanInPermission{
		ConversationID:       convID,
		SourceConversationID: sourceConversationID,
		ToolName:             toolName,
		Action:               action,
		Content:              content,
		ToolDesc:             toolDesc,
		ArgsSummary:          argsSummary,
		SafetyLevel:          safetyLevel,
		SafetyReason:         safetyReason,
		Status:               "pending",
	}
	if err := db.Create(perm).Error; err != nil {
		return nil, fmt.Errorf("create permission record: %w", err)
	}
	return perm, nil
}

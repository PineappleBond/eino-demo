package permission

import (
	"context"
)

// PermissionRequest carries all context needed for permission evaluation.
type PermissionRequest struct {
	Action      string // "read", "write", "execute", "search", "network"
	Content     string // human-readable summary for whitelist matching
	ToolName    string // the tool's registered name
	ToolDesc    string // the tool's description
	ArgsSummary string // brief summary of input arguments
	ToolLevel   int    // self-assessed risk 1-4, 0 = no self-assessment
}

// NeedPermissioner is implemented by tools that require permission checks.
type NeedPermissioner interface {
	NeedPermission(input any) *PermissionRequest
}

// SafetyEvaluation is the output of the safety evaluator.
type SafetyEvaluation struct {
	Level  int    // 1-4
	Reason string // one-sentence explanation
}

// Decision represents the user's choice on a permission request.
type Decision string

const (
	DecisionApproved         Decision = "approved"
	DecisionApprovedExact    Decision = "approved_exact"
	DecisionApprovedWildcard Decision = "approved_wildcard"
	DecisionDenied           Decision = "denied"
)

// ConversationMode controls how tool calls are permission-checked.
type ConversationMode string

const (
	// ModeAskBeforeEdits interrupts for every tool call that needs permission.
	ModeAskBeforeEdits ConversationMode = "ask_before_edits"
	// ModeEditAutomatically uses threshold + whitelist + AI safety eval (existing behavior).
	ModeEditAutomatically ConversationMode = "edit_automatically"
	// ModeBypassPermissions skips all permission checking.
	ModeBypassPermissions ConversationMode = "bypass_permissions"
	// ModePlanMode is reserved for future planning-mode implementation.
	ModePlanMode ConversationMode = "plan_mode"
)

// SafetyEvaluator assesses the risk level of a tool call.
type SafetyEvaluator interface {
	Evaluate(ctx context.Context, req *PermissionRequest) (*SafetyEvaluation, error)
}

// GlobMatcher matches a pattern against a content string.
type GlobMatcher interface {
	Match(pattern, content string) (bool, error)
}

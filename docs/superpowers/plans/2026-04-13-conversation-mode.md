# Conversation Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `mode` field to conversations controlling how tool calls are permission-checked: ask_before_edits, edit_automatically, bypass_permissions, plan_mode (reserved).

**Architecture:** One DB column + OpenAPI enum change → type generation → permission middleware switch → frontend select in ConvInfoPanel. Changes take effect on next agent run.

**Tech Stack:** Go (GORM, Gin), Eino ADK, OpenAPI codegen, Next.js, Ant Design, TypeScript

---

### Task 1: OpenAPI Spec — Add mode enum and update payload field

**Files:**
- Modify: `openapi/spec.yaml`

- [ ] **Step 1: Add mode to Conversation schema**

In `openapi/spec.yaml`, find the `Conversation` schema (around line 876, after `updated_at`). Add:

```yaml
        updated_at:
          type: string
          format: date-time
        mode:
          type: string
          enum:
            - ask_before_edits
            - edit_automatically
            - bypass_permissions
            - plan_mode
          default: ask_before_edits
```

- [ ] **Step 2: Add mode to POST /projects/{id}/conversations requestBody**

Find the POST requestBody schema (around line 221). Add `mode` property:

```yaml
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                title:
                  type: string
                mode:
                  type: string
                  enum:
                    - ask_before_edits
                    - edit_automatically
                    - bypass_permissions
                    - plan_mode
```

- [ ] **Step 3: Update PATCH /conversations/{id} requestBody**

Find the PATCH requestBody schema (around line 263). Make `title` optional, add `mode`, remove `required`:

```yaml
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                title:
                  type: string
                  description: New conversation title
                mode:
                  type: string
                  enum:
                    - ask_before_edits
                    - edit_automatically
                    - bypass_permissions
                    - plan_mode
```

- [ ] **Step 4: Add mode to ConversationUpdatedPayload**

Find `ConversationUpdatedPayload` (around line 1157). Add `mode` after `token_completion`:

```yaml
        token_completion:
          type: integer
        mode:
          type: string
```

- [ ] **Step 5: Commit**

```bash
git add openapi/spec.yaml
git commit -m "feat: add mode enum to conversation schema, PATCH body, and ConversationUpdatedPayload"
```

---

### Task 2: Generate types from updated OpenAPI spec

**Files:**
- Modify: `server/internal/types/types.go` (generated)
- Modify: `web/src/types/api.d.ts` (generated)

- [ ] **Step 1: Run the type generator**

```bash
bash openapi/generate.sh
```

Expected output:
```
Generating Go types...
  → server/internal/types/types.go
Generating TypeScript types...
  → web/src/types/api.d.ts
Done.
```

- [ ] **Step 2: Verify generated Go types**

Check these exist in `server/internal/types/types.go`:
- `type ConversationMode string` with constants
- `Conversation` struct has `Mode *ConversationMode`
- `PatchConversationsIdJSONBody` has `Mode *ConversationMode` and `Title *string` (both optional)
- `PostProjectsIdConversationsJSONBody` has `Mode *ConversationMode`
- `ConversationUpdatedPayload` has `Mode *ConversationMode`

- [ ] **Step 3: Commit**

```bash
git add server/internal/types/types.go web/src/types/api.d.ts
git commit -m "chore: regenerate types from updated OpenAPI spec with mode"
```

---

### Task 3: Backend — Add mode to Conversation model

**Files:**
- Modify: `server/internal/model/conversation.go`
- Test: `server/internal/model/conversation_test.go` (new)

- [ ] **Step 1: Add Mode field to Conversation struct**

In `server/internal/model/conversation.go`, add after `CheckpointID`:

```go
	CheckpointID         string     `gorm:"type:varchar(255);not null;default:''"` // Eino checkpoint for resume
	Mode                 string     `gorm:"type:varchar(30);not null;default:'ask_before_edits'"`
```

- [ ] **Step 2: Write test for mode default**

Create `server/internal/model/conversation_test.go`:

```go
package model

import "testing"

func TestConversationModeDefault(t *testing.T) {
	c := Conversation{}
	if c.Mode != "" {
		t.Errorf("expected empty Mode in zero-value struct, got %q", c.Mode)
	}
}
```

- [ ] **Step 3: Run test**

```bash
cd server && go test ./internal/model/ -run TestConversationMode -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/model/conversation.go server/internal/model/conversation_test.go
git commit -m "feat: add mode field to Conversation model"
```

---

### Task 4: Backend — Define ConversationMode constants and update permission middleware

**Files:**
- Modify: `server/internal/eino/permission/types.go`
- Modify: `server/internal/eino/permission/middleware.go`
- Test: `server/internal/eino/permission/middleware_mode_test.go` (new)

- [ ] **Step 1: Add ConversationMode type and constants to types.go**

In `server/internal/eino/permission/types.go`, add after the `Decision` const block:

```go
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
```

- [ ] **Step 2: Add Mode to MiddlewareConfig**

In `server/internal/eino/permission/middleware.go`, add `Mode` field to `MiddlewareConfig`:

```go
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
	Mode           ConversationMode
}
```

- [ ] **Step 3: Rewrite checkPermission with mode switch**

Replace the entire `checkPermission` method in `server/internal/eino/permission/middleware.go` with:

```go
// checkPermission returns (shouldProceed, error).
// If error is non-nil, the tool call was interrupted (Interrupt was called).
// If shouldProceed is false and error is nil, the tool was denied.
func (m *Middleware) checkPermission(ctx context.Context, toolName, argumentsInJSON string) (bool, error) {
	// Skip if tool doesn't need permission checking.
	np, needsPerm := m.permTools[toolName]
	if !needsPerm {
		return true, nil
	}

	// Mode-based behavior.
	switch m.cfg.Mode {
	case ModeBypassPermissions:
		return true, nil
	case ModePlanMode:
		// TODO: implement plan_mode behavior
		return true, nil
	case ModeAskBeforeEdits:
		return m.checkPermissionAskBeforeEdits(ctx, toolName, np, argumentsInJSON)
	case ModeEditAutomatically, "":
		// Fall through to existing threshold + whitelist + AI eval logic.
	default:
		// Unknown mode — treat as edit_automatically (safe fallback).
	}

	// --- Existing logic for edit_automatically mode (unchanged) ---

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
```

- [ ] **Step 4: Add checkPermissionAskBeforeEdits method**

Add after the `checkPermission` method:

```go
// checkPermissionAskBeforeEdits always interrupts for human approval.
// Skips whitelist and safety evaluation — every tool call requiring permission
// is presented to the user for explicit approval.
func (m *Middleware) checkPermissionAskBeforeEdits(ctx context.Context, toolName string, np NeedPermissioner, argumentsInJSON string) (bool, error) {
	// Check if resuming from a permission interrupt.
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if wasInterrupted {
		return m.handleResume(ctx, toolName, argumentsInJSON)
	}

	// Get permission request from tool.
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

	// Always interrupt — no whitelist, no safety eval.
	question := fmt.Sprintf("Agent 想要调用 %s（%s），是否允许？\n\n操作类型: %s\n详情: %s",
		req.ToolName, req.ToolDesc, req.Action, req.Content)

	choices := []ChoiceOption{
		{Title: "同意", Desc: "允许此次操作"},
		{Title: "拒绝", Desc: "不允许此次操作"},
	}

	perm, err := CreatePendingPerm(m.cfg.DB, m.cfg.ConversationID, req.ToolName, req.Action, req.Content, req.ToolDesc, req.ArgsSummary, 0, "ask_before_edits 模式：每次操作都需要确认")
	if err != nil {
		// Continue anyway
	}

	interruptErr := tool.Interrupt(ctx, map[string]any{
		"type":          "permission_request",
		"tool_name":     req.ToolName,
		"action":        req.Action,
		"content":       req.Content,
		"tool_desc":     req.ToolDesc,
		"args_summary":  req.ArgsSummary,
		"safety_level":  0,
		"safety_reason": "ask_before_edits 模式",
		"question":      question,
		"answer_type":   "single",
		"choices":       choices,
		"permission_id": perm.ID.String(),
	})

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
					"checkpoint_id":   perm.CheckpointID,
					"interrupt_id":    "",
					"tool_name":       req.ToolName,
					"action":          req.Action,
					"content":         req.Content,
					"tool_desc":       req.ToolDesc,
					"args_summary":    req.ArgsSummary,
					"safety_level":    0,
					"safety_reason":   "ask_before_edits 模式",
					"seq":             seq,
				},
			})
		}
	}

	return false, interruptErr
}
```

- [ ] **Step 5: Write unit test for mode constants**

Create `server/internal/eino/permission/middleware_mode_test.go`:

```go
package permission

import "testing"

func TestConversationMode_Values(t *testing.T) {
	tests := []struct {
		name     string
		mode     ConversationMode
		expected string
	}{
		{"ask_before_edits", ModeAskBeforeEdits, "ask_before_edits"},
		{"edit_automatically", ModeEditAutomatically, "edit_automatically"},
		{"bypass_permissions", ModeBypassPermissions, "bypass_permissions"},
		{"plan_mode", ModePlanMode, "plan_mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mode) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.mode)
			}
		})
	}
}

func TestConversationMode_DefaultIsEmpty(t *testing.T) {
	var mode ConversationMode
	if mode != "" {
		t.Errorf("expected default ConversationMode to be empty, got %q", mode)
	}
}
```

- [ ] **Step 6: Run tests**

```bash
cd server && go test ./internal/eino/permission/ -run TestConversationMode -v
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add server/internal/eino/permission/types.go server/internal/eino/permission/middleware.go server/internal/eino/permission/middleware_mode_test.go
git commit -m "feat: add conversation mode support to permission middleware"
```

---

### Task 5: Backend — Pass conversation mode to permission middleware

**Files:**
- Modify: `server/internal/service/chat.go`

- [ ] **Step 1: Pass mode to MiddlewareConfig**

In `server/internal/service/chat.go`, find the `permission.MiddlewareConfig` creation (around line 423). Add `Mode`:

```go
mwCfg := permission.MiddlewareConfig{
	DB:             s.db,
	ProjectID:      project.ID,
	UserID:         userID,
	ConversationID: conversationID,
	Threshold:      2,
	Evaluator:      evaluator,
	Tools: func() []tool.BaseTool {
		s.toolRegistry.SetConversationID(conversationID)
		tools := s.toolRegistry.GetBaseTools()
		tools = append(tools, s.toolRegistry.GetPermissionTools()...)
		return tools
	}(),
	PushUpdate: pushUpdate,
	NextSeq:    nextSeq,
	Mode:       permission.ConversationMode(conv.Mode),
}
```

The `conv` variable is already loaded from DB at line 276. The `permission` package is already imported.

- [ ] **Step 2: Verify build**

```bash
cd server && go build ./...
```

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add server/internal/service/chat.go
git commit -m "feat: pass conversation mode to permission middleware"
```

---

### Task 6: Backend — Support mode in conversation create/update + converter

**Files:**
- Modify: `server/internal/service/conversation.go`
- Modify: `server/internal/handler/conversation.go`
- Modify: `server/internal/convert/convert.go`
- Test: `server/internal/service/conversation_mode_test.go` (new)

- [ ] **Step 1: Add Mode to CreateConversationRequest**

In `server/internal/service/conversation.go`:

```go
// CreateConversationRequest holds the fields for creating a conversation.
type CreateConversationRequest struct {
	Title string `json:"title"`
	Mode  string `json:"mode"`
}
```

- [ ] **Step 2: Use mode in CompleteCreateConversation**

In `CompleteCreateConversation`, find the conversation creation (around line 75). Add mode with default:

```go
conversation := model.Conversation{
	ProjectID: projectID,
	UserID:    userID,
	Title:     req.Title,
	Status:    "active",
	Mode:      req.Mode,
}
if conversation.Mode == "" {
	conversation.Mode = "ask_before_edits"
}
```

- [ ] **Step 3: Add UpdateConversationModeRequest and CompleteUpdateMode**

Add after `CompleteRenameConversation` in `server/internal/service/conversation.go`:

```go
// UpdateConversationModeRequest holds the fields for updating a conversation mode.
type UpdateConversationModeRequest struct {
	Mode string `json:"mode"`
}

// CompleteUpdateMode handles conversation mode update with seq assignment, user_update, and WS push.
func (s *ConversationService) CompleteUpdateMode(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req UpdateConversationModeRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		s.log.Error("update conversation mode: not found",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
		)
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Validate mode
	validModes := map[string]bool{
		"ask_before_edits":   true,
		"edit_automatically": true,
		"bypass_permissions": true,
		"plan_mode":          true,
	}
	if !validModes[req.Mode] {
		return nil, fmt.Errorf("invalid mode: %s", req.Mode)
	}

	// 3. Allocate seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	// 4. Update mode + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&conv).Update("mode", req.Mode).Error; err != nil {
			return err
		}
		conv.Mode = req.Mode

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.updated",
			Payload: model.JSONMap{
				"id":         conversationID.String(),
				"project_id": conv.ProjectID.String(),
				"title":      conv.Title,
				"mode":       conv.Mode,
				"seq":        seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		s.log.Error("update conversation mode: transaction failed",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
			zap.Error(err),
		)
		return nil, err
	}
	s.log.Info("conversation mode updated",
		zap.String("user_id", userID.String()),
		zap.String("conv_id", conversationID.String()),
		zap.String("mode", conv.Mode),
	)
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.updated",
		Payload: model.JSONMap{
			"id":         conversationID.String(),
			"project_id": conv.ProjectID.String(),
			"title":      conv.Title,
			"mode":       conv.Mode,
			"seq":        seq,
		},
	})
	return &conv, nil
}
```

- [ ] **Step 4: Update ToConversation converter**

In `server/internal/convert/convert.go`, add `Mode` to `ToConversation`:

```go
func ToConversation(m model.Conversation) types.Conversation {
	return types.Conversation{
		Id:              toUUID(m.ID),
		ProjectId:       toUUID(m.ProjectID),
		UserId:          toUUID(m.UserID),
		Title:           strPtr(m.Title),
		Summary:         strPtr(m.Summary),
		Status:          strPtr(m.Status),
		LastPreview:     strPtr(m.LastMessagePreview),
		MessageCount:    intPtr(m.MessageCount),
		LatestSeq:       intPtr(int(m.LatestMessageSeq)),
		MemberCount:     intPtr(m.MemberCount),
		TokenPrompt:     intPtr(int(m.TokenPrompt)),
		TokenCompletion: intPtr(int(m.TokenCompletion)),
		CreatedAt:       timePtr(m.CreatedAt),
		UpdatedAt:       timePtr(m.UpdatedAt),
		Mode:            (*types.ConversationMode)(strPtr(m.Mode)),
	}
}
```

- [ ] **Step 5: Rewrite PATCH handler to support both title and mode**

Replace the entire PATCH handler in `server/internal/handler/conversation.go` with:

```go
	// Update conversation (title and/or mode)
	api.PATCH("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("update conversation: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		var req types.PatchConversationsIdJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn("update conversation: invalid request body",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
			return
		}

		updateTitle := req.Title != nil && *req.Title != ""
		updateMode := req.Mode != nil && *req.Mode != ""

		if !updateTitle && !updateMode {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "at least one of title or mode must be provided")
			return
		}

		var conv *model.Conversation

		if updateTitle {
			svcReq := service.RenameConversationRequest{
				Title: *req.Title,
			}
			conv, err = svc.CompleteRenameConversation(
				c.Request.Context(),
				userID,
				conversationID,
				svcReq,
				wsManager.NextSeq,
				func(userID uuid.UUID, update model.UserUpdate) {
					wsUpdate := convert.ToUpdate(update)
					wsManager.PushToUserConnections(userID, wsUpdate)
				},
			)
			if err != nil {
				log.Error("rename conversation failed",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
				return
			}
		}

		if updateMode {
			svcReq := service.UpdateConversationModeRequest{
				Mode: string(*req.Mode),
			}
			modeConv, modeErr := svc.CompleteUpdateMode(
				c.Request.Context(),
				userID,
				conversationID,
				svcReq,
				wsManager.NextSeq,
				func(userID uuid.UUID, update model.UserUpdate) {
					wsUpdate := convert.ToUpdate(update)
					wsManager.PushToUserConnections(userID, wsUpdate)
				},
			)
			if modeErr != nil {
				log.Warn("update conversation mode failed",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
					zap.Error(modeErr),
				)
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to update mode: "+modeErr.Error())
				return
			}
			conv = modeConv
		}

		if conv != nil {
			log.Info("conversation updated",
				zap.String("user_id", userID.String()),
				zap.String("conv_id", conversationID.String()),
			)
			respondJSON(c, http.StatusOK, convert.ToConversation(*conv))
		} else {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "no changes to apply")
		}
	})
```

- [ ] **Step 6: Update POST handler to pass mode**

In the POST handler (around line 74), add mode to the service request:

```go
svcReq := service.CreateConversationRequest{
	Title: valueOrZero(req.Title),
	Mode:  valueOrZero(req.Mode),
}
```

- [ ] **Step 7: Write test for mode validation**

Create `server/internal/service/conversation_mode_test.go`:

```go
package service

import "testing"

func TestValidConversationModes(t *testing.T) {
	validModes := []string{"ask_before_edits", "edit_automatically", "bypass_permissions", "plan_mode"}
	for _, mode := range validModes {
		if mode == "" {
			t.Error("found empty string in valid modes")
		}
	}
}

func TestInvalidModeWouldBeRejected(t *testing.T) {
	invalidModes := []string{"invalid", "ASK_BEFORE_EDITS", "edit"}
	validModes := map[string]bool{
		"ask_before_edits":   true,
		"edit_automatically": true,
		"bypass_permissions": true,
		"plan_mode":          true,
	}

	for _, mode := range invalidModes {
		if validModes[mode] {
			t.Errorf("expected %q to be invalid", mode)
		}
	}
}
```

- [ ] **Step 8: Run tests and verify build**

```bash
cd server && go test ./internal/service/ -run TestValidConversationModes -v
cd server && go test ./internal/service/ -run TestInvalidModeWouldBeRejected -v
cd server && go build ./...
```

- [ ] **Step 9: Commit**

```bash
git add server/internal/service/conversation.go server/internal/handler/conversation.go server/internal/convert/convert.go server/internal/service/conversation_mode_test.go
git commit -m "feat: support mode in conversation create/update endpoints"
```

---

### Task 7: Frontend — Add mode display and selection in ConvInfoPanel

**Files:**
- Modify: `web/src/components/chat/ConvInfoPanel.tsx`
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add i18n keys for mode**

In `web/src/locales/en.json`, add under the `conv` section:

```json
"mode": "Mode",
"mode_ask_before_edits": "Ask before edits",
"mode_edit_automatically": "Edit automatically",
"mode_bypass_permissions": "Bypass permissions",
"mode_plan_mode": "Plan mode (coming soon)",
```

In `web/src/locales/zh.json`, add under the `conv` section:

```json
"mode": "模式",
"mode_ask_before_edits": "编辑前询问",
"mode_edit_automatically": "自动编辑",
"mode_bypass_permissions": "跳过权限检查",
"mode_plan_mode": "计划模式（开发中）",
```

- [ ] **Step 2: Add mode section to ConvInfoPanel**

In `web/src/components/chat/ConvInfoPanel.tsx`:

Add `Select` to imports:
```tsx
import { Avatar, Dropdown, Select } from 'antd';
```

Update the interface (add `mode` and `onModeChange`):
```tsx
interface ConvInfoPanelProps {
  onClose: () => void;
  members?: Member[];
  stats?: {
    messages: number;
    tokenPrompt: number;
    tokenCompletion: number;
    toolCalls: number;
  };
  model?: string;
  template?: string;
  mode?: string;
  onModeChange?: (mode: string) => void;
  onMemberMention?: (member: { id: string; name: string }) => void;
  conversationId?: string;
}
```

Update the destructured props:
```tsx
export function ConvInfoPanel({ onClose, members, stats, model, template, mode, onModeChange, onMemberMention, conversationId }: ConvInfoPanelProps) {
```

Add the mode section after the model/template section (after the closing `</div>` of the model block, around line 130):

```tsx
{mode && (
  <div className="right-panel-section">
    <div className="right-panel-section-title">{t('mode')}</div>
    <Select
      value={mode}
      onChange={(val) => onModeChange?.(val)}
      style={{ width: '100%' }}
      size="small"
      options={[
        { value: 'ask_before_edits', label: t('mode_ask_before_edits') },
        { value: 'edit_automatically', label: t('mode_edit_automatically') },
        { value: 'bypass_permissions', label: t('mode_bypass_permissions') },
        { value: 'plan_mode', label: t('mode_plan_mode'), disabled: true },
      ]}
    />
  </div>
)}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/chat/ConvInfoPanel.tsx web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat: add mode selector to ConvInfoPanel"
```

---

### Task 8: Frontend — Wire mode to chat page

**Files:**
- Modify: `web/src/app/[locale]/project/[id]/chat/[convId]/page.tsx`

- [ ] **Step 1: Import Conversation type and add mode state**

At the top of `web/src/app/[locale]/project/[id]/chat/[convId]/page.tsx`, add `Conversation` to the api import:

```tsx
import { api, Message as MessageType, Conversation } from '@/lib/api';
```

Add `projectId` extraction (it may already be available via params):

```tsx
const params = useParams();
const convId = params.convId as string;
const projectId = params.id as string;
```

Add mode state after the `useReducer`:
```tsx
const [convMode, setConvMode] = useState<string>('ask_before_edits');
```

- [ ] **Step 2: Load mode on initial fetch**

In the initial load `useEffect` (around line 404), add a fetch for the conversation list to get mode. After the permissions sync block (around line 490), add:

```tsx
// Fetch conversation metadata from list to get mode
api.get<Conversation[]>(`/projects/${projectId}/conversations`)
  .then((convs) => {
    const conv = convs.find((c) => c.id === convId);
    if (conv?.mode) setConvMode(conv.mode);
  })
  .catch(() => {});
```

Also add `projectId` to the useEffect dependency array:
```tsx
}, [convId, message, projectId]);
```

- [ ] **Step 3: Pass mode to ConvInfoPanel and handle changes**

Find where `ConvInfoPanel` is rendered (around line 743). Update to:

```tsx
{state.showConvInfo && (
  <ConvInfoPanel
    onClose={() => dispatch({ type: 'TOGGLE_CONV_INFO' })}
    members={state.members}
    stats={stats}
    model="Sonnet"
    mode={convMode}
    onModeChange={async (newMode) => {
      try {
        await api.patch(`/conversations/${convId}`, { mode: newMode });
        setConvMode(newMode);
        message.success(t('modeUpdated') || 'Mode updated');
      } catch (err) {
        message.error(t('modeUpdateFailed') || 'Failed to update mode');
      }
    }}
    onMemberMention={handleMemberMention}
    conversationId={convId}
  />
)}
```

- [ ] **Step 4: Add i18n keys for mode updates**

In `web/src/locales/en.json`, add to `chat` section:
```json
"modeUpdated": "Mode updated",
"modeUpdateFailed": "Failed to update mode"
```

In `web/src/locales/zh.json`, add to `chat` section:
```json
"modeUpdated": "模式已更新",
"modeUpdateFailed": "模式更新失败"
```

- [ ] **Step 5: Sync mode from conversation.updated events**

In `handleStreamingUpdate`, find the `case 'conversation.updated':` block. Add mode sync:

```tsx
case 'conversation.updated': {
  const payload = update.payload as components['schemas']['ConversationUpdatedPayload'];
  dispatch({
    type: 'SET_CONV_TOKENS',
    payload: {
      tokenPrompt: payload.token_prompt,
      tokenCompletion: payload.token_completion,
    },
  });
  // Sync mode from update payload
  if ((payload as Record<string, unknown>).mode) {
    setConvMode((payload as Record<string, unknown>).mode as string);
  }
  break;
}
```

Note: Using `as Record<string, unknown>` because the generated `ConversationUpdatedPayload` may have `mode` as an optional typed field.

- [ ] **Step 6: Verify frontend compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: No errors (or only pre-existing errors).

- [ ] **Step 7: Commit**

```bash
git add web/src/app/\[locale\]/project/\[id\]/chat/\[convId\]/page.tsx web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat: wire mode state and update to chat page"
```

---

### Task 9: Run GORM AutoMigrate and verify end-to-end

**Files:**
- No code changes — this is a verification step

- [ ] **Step 1: Start dependencies**

```bash
docker compose -f deploy/dependencies/dev/docker-compose.yaml up -d
```

- [ ] **Step 2: Start server**

```bash
go run ./server/cmd/server/main.go
```

Verify logs show successful startup. GORM AutoMigrate will add the `mode` column to the `conversations` table.

- [ ] **Step 3: Create a conversation with default mode**

```bash
curl -s -X POST http://localhost:8080/api/projects/TEST_PROJECT_ID/conversations \
  -H "Authorization: Bearer test" \
  -H "Content-Type: application/json" \
  -d '{"title":"Test Default Mode"}' | jq
```

Verify response includes `"mode": "ask_before_edits"`.

- [ ] **Step 4: Create a conversation with explicit mode**

```bash
curl -s -X POST http://localhost:8080/api/projects/TEST_PROJECT_ID/conversations \
  -H "Authorization: Bearer test" \
  -H "Content-Type: application/json" \
  -d '{"title":"Test Bypass Mode","mode":"bypass_permissions"}' | jq
```

Verify response includes `"mode": "bypass_permissions"`.

- [ ] **Step 5: Update mode via PATCH**

```bash
CONV_ID="from_step_3"
curl -s -X PATCH http://localhost:8080/api/conversations/$CONV_ID \
  -H "Authorization: Bearer test" \
  -H "Content-Type: application/json" \
  -d '{"mode":"edit_automatically"}' | jq
```

Verify response includes `"mode": "edit_automatically"`.

- [ ] **Step 6: Verify invalid mode is rejected**

```bash
curl -s -X PATCH http://localhost:8080/api/conversations/$CONV_ID \
  -H "Authorization: Bearer test" \
  -H "Content-Type: application/json" \
  -d '{"mode":"invalid"}' | jq
```

Expected: 400 error response.

- [ ] **Step 7: Commit final verification**

```bash
git add -A
git commit -m "chore: verify conversation mode end-to-end"
```

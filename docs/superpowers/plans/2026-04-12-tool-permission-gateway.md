# Tool Permission Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Eino Agent 的 Tool 调用增加多层权限网关（白名单匹配 + Agent 安全评估 + HITL 人类确认），确保敏感操作在执行前获得用户授权。

**Architecture:** 通过 `ChatModelAgentMiddleware.WrapInvokableToolCall` 在工具执行前拦截，实现三层递进权限检查：DB 白名单 → haiku Agent 安全评估 → tool.Interrupt 中断等待人类决策。用户决策通过 ResumeParams 传回中间件。

**Tech Stack:** Go 1.26, eino/adk, eino/compose, GORM, PostgreSQL, OpenAPI codegen

---

### Task 0: 更新 OpenAPI Spec（协议优先）

**Files:**
- Modify: `openapi/spec.yaml`

- [ ] **Step 1: 新增 HumanInPermission schema**

在 `Todo:` schema 之后、`HumanInTheLoopCreatedPayload:` 之前添加：

```yaml
    HumanInPermission:
      type: object
      required: [id, conversation_id, tool_name, action, content, safety_level, status, created_at]
      properties:
        id:
          type: string
          format: uuid
        conversation_id:
          type: string
          format: uuid
        checkpoint_id:
          type: string
        interrupt_id:
          type: string
        tool_name:
          type: string
        action:
          type: string
        content:
          type: string
        tool_desc:
          type: string
        args_summary:
          type: string
        safety_level:
          type: integer
          minimum: 1
          maximum: 4
        safety_reason:
          type: string
        decision:
          type: string
          nullable: true
          enum: [approved, approved_exact, approved_wildcard, denied]
        status:
          type: string
          enum: [pending, answered]
        created_at:
          type: string
          format: date-time
```

- [ ] **Step 2: 新增 PermissionPendingPayload 和 PermissionDecidedPayload**

在 `TodoSyncPayload:` 之后添加：

```yaml
    PermissionPendingPayload:
      type: object
      required: [conversation_id, permission_id, tool_name, action, content, safety_level, safety_reason, seq]
      properties:
        conversation_id: { type: string }
        permission_id: { type: string }
        tool_name: { type: string }
        action: { type: string }
        content: { type: string }
        tool_desc: { type: string }
        args_summary: { type: string }
        safety_level: { type: integer }
        safety_reason: { type: string }
        checkpoint_id: { type: string }
        interrupt_id: { type: string }
        seq: { type: integer, format: int64 }

    PermissionDecidedPayload:
      type: object
      required: [conversation_id, permission_id, decision, seq]
      properties:
        conversation_id: { type: string }
        permission_id: { type: string }
        decision: { type: string, enum: [approved, approved_exact, approved_wildcard, denied] }
        seq: { type: integer, format: int64 }
```

- [ ] **Step 3: 在 Update.type enum 中追加**

在 `- empty` 之后追加：
```yaml
            - permission.pending
            - permission.decided
```

- [ ] **Step 4: 在 Update.payload oneOf 中追加**

在 `- $ref: "#/components/schemas/EmptyPayload"` 之后追加：
```yaml
            - $ref: "#/components/schemas/PermissionPendingPayload"
            - $ref: "#/components/schemas/PermissionDecidedPayload"
```

- [ ] **Step 5: 新增 HTTP 端点**

在 `/conversations/{id}/hitl:` 块之后、`/conversations/{id}/todos:` 之前添加：

```yaml
  /conversations/{id}/permissions:
    get:
      summary: List permission requests for a conversation
      parameters:
        - name: id
          in: path
          required: true
          schema: { type: string }
        - name: status
          in: query
          required: false
          schema: { type: string, enum: [pending, answered] }
      responses:
        "200":
          description: List of permission requests
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/HumanInPermission" }
        "404": { description: Conversation not found }

  /conversations/{id}/permissions/{permId}/answer:
    post:
      summary: Answer a permission request
      parameters:
        - name: id
          in: path
          required: true
          schema: { type: string }
        - name: permId
          in: path
          required: true
          schema: { type: string }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [checkpoint_id, interrupt_id, decision]
              properties:
                checkpoint_id: { type: string }
                interrupt_id: { type: string }
                decision: { type: string, enum: [approved, approved_exact, approved_wildcard, denied] }
      responses:
        "200": { description: Permission answered, agent resumed }
        "404": { description: Permission request not found }
        "400": { description: Already answered or invalid request }
```

- [ ] **Step 6: 运行代码生成 + 编译验证**

```bash
bash openapi/generate.sh
cd server && go build ./...
```

- [ ] **Step 7: 提交**

```bash
git add openapi/spec.yaml server/internal/types/types.go web/src/types/api.d.ts
git commit -m "feat: add permission gateway API types to OpenAPI spec"
```

---

### Task 1: 新增 DB 模型 + AutoMigrate

**Files:**
- Create: `server/internal/model/project_tool_permission.go`
- Create: `server/internal/model/human_in_permission.go`
- Modify: `server/internal/db/db.go`

- [ ] **Step 1: 创建 ProjectToolPermission 模型**

```go
package model

import (
	"github.com/google/uuid"
)

// ProjectToolPermission stores per-project tool permission whitelists.
type ProjectToolPermission struct {
	BaseModel
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	ToolName  string    `gorm:"type:varchar(128);not null;index:idx_perm_tool_action"`
	Action    string    `gorm:"type:varchar(64);not null;index:idx_perm_tool_action"`
	Pattern   string    `gorm:"type:text;not null"`
	GrantedBy uuid.UUID `gorm:"type:uuid;not null"`
	Level     string    `gorm:"type:varchar(16);not null;default:'exact'"` // "exact" | "wildcard"
}

func (ProjectToolPermission) TableName() string { return "project_tool_permissions" }
```

- [ ] **Step 2: 创建 HumanInPermission 模型**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// HumanInPermission represents a pending tool permission request awaiting user approval.
type HumanInPermission struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	CheckpointID   string    `gorm:"type:varchar(255);not null"`
	InterruptID    string    `gorm:"type:varchar(255);not null"`
	ToolName       string    `gorm:"type:varchar(128);not null"`
	Action         string    `gorm:"type:varchar(64);not null"`
	Content        string    `gorm:"type:text;not null"`
	ToolDesc       string    `gorm:"type:text;not null"`
	ArgsSummary    string    `gorm:"type:text;not null"`
	SafetyLevel    int       `gorm:"not null"`
	SafetyReason   string    `gorm:"type:text;not null"`
	Decision       string    `gorm:"type:varchar(20);default:null"`
	Status         string    `gorm:"type:varchar(20);not null;default:'pending'"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (HumanInPermission) TableName() string { return "human_in_permissions" }
```

- [ ] **Step 3: 修改 db.go 的 AutoMigrate**

读取 `server/internal/db/db.go`，在 AutoMigrate 列表末尾追加：

```go
&model.ProjectToolPermission{},
&model.HumanInPermission{},
```

- [ ] **Step 4: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add server/internal/model/project_tool_permission.go server/internal/model/human_in_permission.go server/internal/db/db.go
git commit -m "feat: add permission gateway DB models"
```

---

### Task 2: 定义权限接口与核心类型

**Files:**
- Create: `server/internal/eino/permission/types.go`

- [ ] **Step 1: 创建 types.go**

```go
package permission

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
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

// SafetyEvaluator assesses the risk level of a tool call.
type SafetyEvaluator interface {
	Evaluate(ctx context.Context, req *PermissionRequest) (*SafetyEvaluation, error)
}
```

- [ ] **Step 2: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 3: 提交**

```bash
git add server/internal/eino/permission/types.go
git commit -m "feat: define permission interfaces and types"
```

---

### Task 3: 实现安全评估 Agent（Evaluator）

**Files:**
- Create: `server/internal/eino/permission/prompt.go`
- Create: `server/internal/eino/permission/evaluator.go`

- [ ] **Step 1: 创建 prompt.go**

```go
package permission

import (
	"bytes"
	"text/template"
)

const evaluatorPromptTemplate = `你是一个安全评估助手。你的任务是评估一个 AI Agent 即将执行的工具调用是否需要人类事先知情同意。

## 评估规则

安全等级分为 4 级：
- 等级 1 (Safe): 纯只读操作，访问公开信息，无任何副作用
- 等级 2 (Low): 读取操作，但可能涉及用户私人数据（如读取文件内容、列出目录）
- 等级 3 (Medium): 写入或修改操作（如创建/编辑文件、修改配置、发送只读请求以外的 HTTP）
- 等级 4 (High): 执行系统命令、不可逆操作、可能造成数据丢失的操作

## 输出格式

只输出 JSON，不要输出其他内容：
{"safety_level": <1-4>, "reason": "<一句话说明理由>"}

## 上下文

工具名称: {{.ToolName}}
工具用途: {{.ToolDesc}}
操作类型: {{.Action}}
操作内容: {{.Content}}
调用参数: {{.ArgsSummary}}
工具自评: {{.ToolLevel}} (0表示不自评)
`

type promptData struct {
	ToolName    string
	ToolDesc    string
	Action      string
	Content     string
	ArgsSummary string
	ToolLevel   int
}

func renderEvaluatorPrompt(req *PermissionRequest) (string, error) {
	tpl, err := template.New("evaluator").Parse(evaluatorPromptTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, promptData{
		ToolName:    req.ToolName,
		ToolDesc:    req.ToolDesc,
		Action:      req.Action,
		Content:     req.Content,
		ArgsSummary: req.ArgsSummary,
		ToolLevel:   req.ToolLevel,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

- [ ] **Step 2: 创建 evaluator.go**

```go
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
```

- [ ] **Step 3: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/internal/eino/permission/prompt.go server/internal/eino/permission/evaluator.go
git commit -m "feat: implement LLM-backed safety evaluator"
```

---

### Task 4: 实现权限中间件（核心拦截逻辑）

**Files:**
- Create: `server/internal/eino/permission/checker.go`
- Create: `server/internal/eino/permission/middleware.go`

- [ ] **Step 1: 创建 checker.go**

```go
package permission

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	// Eval contains the safety evaluation.
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
		if p.Level == "wildcard" {
			if matched, _ := matchPattern(p.Pattern, content); matched {
				return true
			}
		} else {
			if p.Pattern == content {
				return true
			}
		}
	}
	return false
}

// WriteWhitelist adds a new permission entry.
func (c *Checker) WriteWhitelist(ctx context.Context, req *PermissionRequest, wildcard bool) error {
	pattern := req.Content
	if wildcard {
		// Convert content to a wildcard pattern.
		// For now, use exact match — the frontend can refine later.
	}

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

	return c.db.Exec(
		"INSERT INTO project_tool_permissions (id, project_id, tool_name, action, pattern, granted_by, level, created_at, updated_at) VALUES (gen_random_uuid(), ?, ?, ?, ?, ?, ?, NOW(), NOW())",
		c.projectID, req.ToolName, req.Action, pattern, c.userID, level,
	).Error
}

// mapAnswerToDecision maps the user's answer text to a Decision.
func mapAnswerToDecision(answer string) Decision {
	switch answer {
	case "同意":
		return DecisionApproved
	case "同意并记住":
		return DecisionApprovedExact
	case "同意并通配记住":
		return DecisionApprovedWildcard
	default:
		return DecisionDenied
	}
}
```

- [ ] **Step 2: 创建 middleware.go**

```go
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

	decision := mapAnswerToDecision(resumeInfo["decision"])

	// Get the original permission request from interrupt context.
	_, _, rawPayload := tool.GetInterruptState[map[string]any](ctx)
	req := &PermissionRequest{
		ToolName:   getString(rawPayload, "tool_name"),
		Action:     getString(rawPayload, "action"),
		Content:    getString(rawPayload, "content"),
		ToolDesc:   getString(rawPayload, "tool_desc"),
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
	perm := model.HumanInPermission{
		ConversationID: m.cfg.ConversationID,
		ToolName:       req.ToolName,
		Action:         req.Action,
		Content:        req.Content,
		ToolDesc:       req.ToolDesc,
		ArgsSummary:    req.ArgsSummary,
		SafetyLevel:    eval.Level,
		SafetyReason:   eval.Reason,
		Status:         "pending",
	}

	if err := m.cfg.DB.Create(&perm).Error; err != nil {
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
					"safety_level":    eval.Level,
					"safety_reason":   eval.Reason,
					"seq":             seq,
				},
			})
		}
	}

	// Store checkpoint/interrupt IDs in the record.
	// These will be filled by the framework when Interrupt is called.
	// We use a callback approach — the callback in chat.go will update the record.
	_ = tool.Interrupt(ctx, map[string]any{
		"type":           "permission_request",
		"tool_name":      req.ToolName,
		"action":         req.Action,
		"content":        req.Content,
		"tool_desc":      req.ToolDesc,
		"args_summary":   req.ArgsSummary,
		"safety_level":   eval.Level,
		"safety_reason":  eval.Reason,
		"question":       question,
		"choices":        choices,
		"answer_type":    "single",
	})

	return nil
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
```

- [ ] **Step 3: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/internal/eino/permission/checker.go server/internal/eino/permission/middleware.go
git commit -m "feat: implement permission middleware with interrupt/resume flow"
```

---

### Task 5: 集成到 RootRunner

**Files:**
- Modify: `server/internal/eino/runner/rootrunner.go`

- [ ] **Step 1: 在 RootRunnerConfig 中添加 PermissionMW 字段**

在 `TokenCheck` 字段后添加：

```go
	// PermissionMW intercepts tool calls for permission checking.
	// Nil disables permission gateway.
	PermissionMW *permission.Middleware
```

在 import 中添加：
```go
"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
```

- [ ] **Step 2: 在 handler 链中插入权限中间件**

在 `NewRootRunner` 中，handlers 链构建处（Summarization 之后，Context Injection 之前），添加：

```go
// 3b. Permission middleware — intercepts tool calls for permission checks.
if cfg.PermissionMW != nil {
	handlers = append(handlers, cfg.PermissionMW)
}
```

确保顺序为：Summarization → Permission → Context Injection → Reduction

- [ ] **Step 3: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/internal/eino/runner/rootrunner.go
git commit -m "feat: integrate permission middleware into RootRunner handler chain"
```

---

### Task 6: 实现 Service 层（AnswerPermission + ListPendingPermissions）

**Files:**
- Create: `server/internal/service/permission.go`
- Modify: `server/internal/service/chat.go`（新增 AnswerPermission 方法）

- [ ] **Step 1: 创建 permission.go Service**

```go
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// PermissionService manages tool permission requests and whitelist.
type PermissionService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewPermissionService creates a PermissionService.
func NewPermissionService(db *gorm.DB, log *zap.Logger) *PermissionService {
	return &PermissionService{db: db, log: log}
}

// ListPendingPermissions returns permission requests for a conversation.
func (s *PermissionService) ListPendingPermissions(conversationID uuid.UUID, status string) ([]model.HumanInPermission, error) {
	var perms []model.HumanInPermission
	query := s.db.Where("conversation_id = ?", conversationID).Order("created_at ASC")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("failed to list permissions: %w", err)
	}
	return perms, nil
}

// AnswerPermissionRequest holds the fields for answering a permission request.
type AnswerPermissionRequest struct {
	CheckpointID string `json:"checkpoint_id"`
	InterruptID  string `json:"interrupt_id"`
	Decision     string `json:"decision"` // approved|approved_exact|approved_wildcard|denied
}

// AnswerPermissionResult holds the result of answering a permission request.
type AnswerPermissionResult struct {
	PermissionID uuid.UUID
	Decision     string
}

// AnswerPermission processes a user's decision on a permission request.
func (s *PermissionService) AnswerPermissionRequest(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req AnswerPermissionRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
	convProjectID uuid.UUID, // project ID for whitelist writing
) (*AnswerPermissionResult, error) {
	// 1. Verify conversation ownership.
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	// 2. Find the pending permission record.
	var perm model.HumanInPermission
	if err := s.db.Where(
		"conversation_id = ? AND checkpoint_id = ? AND interrupt_id = ? AND status = 'pending'",
		conversationID, req.CheckpointID, req.InterruptID,
	).First(&perm).Error; err != nil {
		return nil, fmt.Errorf("pending permission request not found")
	}

	// 3. Validate decision.
	validDecisions := map[string]bool{
		"approved":          true,
		"approved_exact":    true,
		"approved_wildcard": true,
		"denied":            true,
	}
	if !validDecisions[req.Decision] {
		return nil, fmt.Errorf("invalid decision: %s", req.Decision)
	}

	// 4. Update permission record.
	if err := s.db.Model(&perm).Updates(map[string]interface{}{
		"status":   "answered",
		"decision": req.Decision,
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update permission: %w", err)
	}

	// 5. Write whitelist if applicable.
	if req.Decision == "approved_exact" || req.Decision == "approved_wildcard" {
		wildcard := req.Decision == "approved_wildcard"
		level := "exact"
		if wildcard {
			level = "wildcard"
		}

		// Check for duplicates.
		var count int64
		s.db.Table("project_tool_permissions").
			Where("project_id = ? AND tool_name = ? AND action = ? AND pattern = ?",
				convProjectID, perm.ToolName, perm.Action, perm.Content).
			Count(&count)

		if count == 0 {
			wp := model.ProjectToolPermission{
				ProjectID: convProjectID,
				ToolName:  perm.ToolName,
				Action:    perm.Action,
				Pattern:   perm.Content,
				GrantedBy: userID,
				Level:     level,
			}
			if err := s.db.Create(&wp).Error; err != nil {
				s.log.Error("failed to write whitelist entry", zap.Error(err))
			}
		}
	}

	// 6. Push permission.decided Update.
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		s.log.Error("seq assignment failed", zap.Error(err))
	} else if seq > 0 {
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "permission.decided",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"permission_id":   perm.ID.String(),
				"decision":        req.Decision,
				"seq":             seq,
			},
		}
		if dbErr := s.db.Create(&update).Error; dbErr != nil {
			s.log.Error("failed to persist permission.decided update", zap.Error(dbErr))
		} else {
			pushUpdate(userID, update)
		}
	}

	// 7. Set ResumeParams and restart agent.
	// The resume data carries the decision string.
	runSessionMgr := getRunSessionMgr() // injected or global
	runSessionMgr.SetResumeParams(conversationID, &adk.ResumeParams{
		Targets: map[string]any{
			req.InterruptID: req.Decision,
		},
	})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("AnswerPermission runAgent panic recovered", zap.Any("recover", r))
			}
		}()
		// The runAgent call needs userID, conversationID etc.
		// This will be wired through the ChatService.
	}()

	return &AnswerPermissionResult{
		PermissionID: perm.ID,
		Decision:     req.Decision,
	}, nil
}
```

Wait, the `AnswerPermission` method needs access to `runAgent` which is in `ChatService`. It makes more sense to put this method on `ChatService` directly, similar to `AnswerQuestion`. Let me adjust the plan.

The PermissionService should only handle DB operations (list, answer, write whitelist). The ChatService should have `AnswerPermission` that calls PermissionService + restarts the agent.

- [ ] **Step 2: 创建 permission.go Service（精简版）**

```go
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// PermissionService manages tool permission requests and whitelist.
type PermissionService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewPermissionService creates a PermissionService.
func NewPermissionService(db *gorm.DB, log *zap.Logger) *PermissionService {
	return &PermissionService{db: db, log: log}
}

// ListPermissions returns permission requests for a conversation.
func (s *PermissionService) ListPermissions(conversationID uuid.UUID, status string) ([]model.HumanInPermission, error) {
	var perms []model.HumanInPermission
	query := s.db.Where("conversation_id = ?", conversationID).Order("created_at ASC")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return perms, nil
}

// AnswerPermissionResult holds the result of answering a permission request.
type AnswerPermissionResult struct {
	PermissionID uuid.UUID
	Decision     string
	CheckpointID string
	InterruptID  string
}

// AnswerPermissionRequest holds the fields for answering a permission request.
type AnswerPermissionRequest struct {
	CheckpointID string `json:"checkpoint_id"`
	InterruptID  string `json:"interrupt_id"`
	Decision     string `json:"decision"`
}

// AnswerPermission updates the permission record and writes whitelist if needed.
// Returns result for the caller to set resume params and restart agent.
func (s *PermissionService) AnswerPermission(
	ctx context.Context,
	userID, conversationID, projectID uuid.UUID,
	req AnswerPermissionRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*AnswerPermissionResult, error) {
	// 1. Verify conversation ownership.
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	// 2. Find pending record.
	var perm model.HumanInPermission
	if err := s.db.Where(
		"conversation_id = ? AND checkpoint_id = ? AND interrupt_id = ? AND status = 'pending'",
		conversationID, req.CheckpointID, req.InterruptID,
	).First(&perm).Error; err != nil {
		return nil, fmt.Errorf("pending permission request not found")
	}

	// 3. Validate decision.
	validDecisions := map[string]bool{
		"approved": true, "approved_exact": true,
		"approved_wildcard": true, "denied": true,
	}
	if !validDecisions[req.Decision] {
		return nil, fmt.Errorf("invalid decision: %s", req.Decision)
	}

	// 4. Update record.
	if err := s.db.Model(&perm).Updates(map[string]interface{}{
		"status": "answered", "decision": req.Decision,
	}).Error; err != nil {
		return nil, fmt.Errorf("update permission: %w", err)
	}

	// 5. Write whitelist if applicable.
	if req.Decision == "approved_exact" || req.Decision == "approved_wildcard" {
		level := "exact"
		if req.Decision == "approved_wildcard" {
			level = "wildcard"
		}

		var count int64
		s.db.Table("project_tool_permissions").
			Where("project_id = ? AND tool_name = ? AND action = ? AND pattern = ?",
				projectID, perm.ToolName, perm.Action, perm.Content).
			Count(&count)

		if count == 0 {
			wp := model.ProjectToolPermission{
				ProjectID: projectID,
				ToolName:  perm.ToolName,
				Action:    perm.Action,
				Pattern:   perm.Content,
				GrantedBy: userID,
				Level:     level,
			}
			if err := s.db.Create(&wp).Error; err != nil {
				s.log.Error("write whitelist", zap.Error(err))
			}
		}
	}

	// 6. Push Update.
	seq, seqErr := nextSeq(ctx, userID)
	if seqErr != nil {
		s.log.Error("seq assignment failed", zap.Error(seqErr))
	} else if seq > 0 {
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "permission.decided",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"permission_id":   perm.ID.String(),
				"decision":        req.Decision,
				"seq":             seq,
			},
		}
		if dbErr := s.db.Create(&update).Error; dbErr != nil {
			s.log.Error("persist permission.decided update", zap.Error(dbErr))
		} else {
			pushUpdate(userID, update)
		}
	}

	return &AnswerPermissionResult{
		PermissionID: perm.ID,
		Decision:     req.Decision,
		CheckpointID: req.CheckpointID,
		InterruptID:  req.InterruptID,
	}, nil
}
```

- [ ] **Step 3: 在 chat.go 中添加 AnswerPermission 方法**

在 `AnswerQuestion` 方法之后添加：

```go
// AnswerPermissionRequest holds the fields for answering a permission request.
type AnswerPermissionRequest struct {
	CheckpointID string `json:"checkpoint_id"`
	InterruptID  string `json:"interrupt_id"`
	Decision     string `json:"decision"` // approved|approved_exact|approved_wildcard|denied
}

// AnswerPermission handles a user's decision on a permission request.
func (s *ChatService) AnswerPermission(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req AnswerPermissionRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify ownership and get project ID.
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	var project model.Project
	if err := s.db.Where("id = ?", conv.ProjectID).First(&project).Error; err != nil {
		return fmt.Errorf("project not found")
	}

	// 2. Find the pending permission record.
	var perm model.HumanInPermission
	if err := s.db.Where(
		"conversation_id = ? AND checkpoint_id = ? AND interrupt_id = ? AND status = 'pending'",
		conversationID, req.CheckpointID, req.InterruptID,
	).First(&perm).Error; err != nil {
		return fmt.Errorf("pending permission request not found")
	}

	// 3. Validate decision.
	validDecisions := map[string]bool{
		"approved": true, "approved_exact": true,
		"approved_wildcard": true, "denied": true,
	}
	if !validDecisions[req.Decision] {
		return fmt.Errorf("invalid decision: %s", req.Decision)
	}

	// 4. Update record.
	if err := s.db.Model(&perm).Updates(map[string]interface{}{
		"status": "answered", "decision": req.Decision,
	}).Error; err != nil {
		return fmt.Errorf("update permission: %w", err)
	}

	// 5. Write whitelist if applicable.
	if req.Decision == "approved_exact" || req.Decision == "approved_wildcard" {
		level := "exact"
		if req.Decision == "approved_wildcard" {
			level = "wildcard"
		}

		var count int64
		s.db.Table("project_tool_permissions").
			Where("project_id = ? AND tool_name = ? AND action = ? AND pattern = ?",
				project.ID, perm.ToolName, perm.Action, perm.Content).
			Count(&count)

		if count == 0 {
			wp := model.ProjectToolPermission{
				ProjectID: project.ID,
				ToolName:  perm.ToolName,
				Action:    perm.Action,
				Pattern:   perm.Content,
				GrantedBy: userID,
				Level:     level,
			}
			if err := s.db.Create(&wp).Error; err != nil {
				s.log.Error("write whitelist", zap.Error(err))
			}
		}
	}

	// 6. Push permission.decided Update.
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		s.log.Error("seq assignment failed", zap.Error(err))
	} else if seq > 0 {
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "permission.decided",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"permission_id":   perm.ID.String(),
				"decision":        req.Decision,
				"seq":             seq,
			},
		}
		if dbErr := s.db.Create(&update).Error; dbErr != nil {
			s.log.Error("persist permission.decided update", zap.Error(dbErr))
		} else {
			pushUpdate(userID, update)
		}
	}

	// 7. Set ResumeParams and restart agent.
	s.runSessionMgr.SetResumeParams(conversationID, &adk.ResumeParams{
		Targets: map[string]any{
			req.InterruptID: req.Decision,
		},
	})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("AnswerPermission runAgent panic recovered", zap.Any("recover", r))
			}
		}()
		s.runAgent(context.WithoutCancel(ctx), userID, conversationID, "", nextSeq, pushUpdate)
	}()

	return nil
}

// ListPendingPermissions returns pending permission requests for a conversation.
func (s *ChatService) ListPendingPermissions(userID, conversationID uuid.UUID, status string) ([]model.HumanInPermission, error) {
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	var perms []model.HumanInPermission
	query := s.db.Where("conversation_id = ?", conversationID).Order("created_at ASC")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return perms, nil
}
```

- [ ] **Step 4: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add server/internal/service/chat.go
git commit -m "feat: add AnswerPermission and ListPendingPermissions to ChatService"
```

---

### Task 7: 实现 HTTP Handler 层

**Files:**
- Create: `server/internal/handler/permission.go`
- Modify: `server/internal/handler/http.go`（注册路由）

- [ ] **Step 1: 创建 permission.go handler**

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
)

// PermissionHandler handles permission-related HTTP endpoints.
type PermissionHandler struct {
	chatSvc *service.ChatService
}

// NewPermissionHandler creates a PermissionHandler.
func NewPermissionHandler(chatSvc *service.ChatService) *PermissionHandler {
	return &PermissionHandler{chatSvc: chatSvc}
}

// ListPermissions handles GET /conversations/:id/permissions
func (h *PermissionHandler) ListPermissions(c *gin.Context) {
	userID := getUserID(c)
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error: types.ErrorDetail{Code: "INVALID_ID", Message: "invalid conversation ID"},
		})
		return
	}

	status := c.Query("status")
	perms, err := h.chatSvc.ListPendingPermissions(userID, convID, status)
	if err != nil {
		c.JSON(http.StatusNotFound, types.ErrorResponse{
			Error: types.ErrorDetail{Code: "NOT_FOUND", Message: err.Error()},
		})
		return
	}

	result := make([]types.HumanInPermission, len(perms))
	for i, p := range perms {
		result[i] = permissionToType(p)
	}

	c.JSON(http.StatusOK, result)
}

// AnswerPermission handles POST /conversations/:id/permissions/:permId/answer
func (h *PermissionHandler) AnswerPermission(c *gin.Context) {
	userID := getUserID(c)
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error: types.ErrorDetail{Code: "INVALID_ID", Message: "invalid conversation ID"},
		})
		return
	}

	var req struct {
		CheckpointID string `json:"checkpoint_id"`
		InterruptID  string `json:"interrupt_id"`
		Decision     string `json:"decision"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error: types.ErrorDetail{Code: "INVALID_REQUEST", Message: "invalid request body"},
		})
		return
	}

	svcReq := service.AnswerPermissionRequest{
		CheckpointID: req.CheckpointID,
		InterruptID:  req.InterruptID,
		Decision:     req.Decision,
	}

	if err := h.chatSvc.AnswerPermission(c.Request.Context(), userID, convID, svcReq, nextSeqFromContext(c), pushUpdateFromContext(c)); err != nil {
		c.JSON(http.StatusNotFound, types.ErrorResponse{
			Error: types.ErrorDetail{Code: "NOT_FOUND", Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// permissionToType converts model.HumanInPermission to generated types.HumanInPermission.
func permissionToType(p model.HumanInPermission) types.HumanInPermission {
	result := types.HumanInPermission{
		Id:             p.ID.String(),
		ConversationId: p.ConversationID.String(),
		ToolName:       p.ToolName,
		Action:         p.Action,
		Content:        p.Content,
		ToolDesc:       p.ToolDesc,
		ArgsSummary:    p.ArgsSummary,
		SafetyLevel:    int64(p.SafetyLevel),
		SafetyReason:   p.SafetyReason,
		Status:         p.Status,
		CreatedAt:      p.CreatedAt,
	}
	if p.CheckpointID != "" {
		result.CheckpointId = &p.CheckpointID
	}
	if p.InterruptID != "" {
		result.InterruptId = &p.InterruptID
	}
	if p.Decision != "" {
		result.Decision = &p.Decision
	}
	return result
}
```

Note: `nextSeqFromContext` and `pushUpdateFromContext` are helpers that need to be provided — these follow the same pattern as existing handlers. The handler needs access to `NextSeq` and `PushUpdate` functions which are typically passed from the DI module.

- [ ] **Step 2: 注册路由**

在 `http.go` 的路由注册处添加：

```go
// Permissions
permissions := api.Group("/conversations/:id/permissions")
permissions.GET("", permissionHandler.ListPermissions)
permissions.POST("/:permId/answer", permissionHandler.AnswerPermission)
```

- [ ] **Step 3: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/internal/handler/permission.go server/internal/handler/http.go
git commit -m "feat: add permission HTTP endpoints"
```

---

### Task 8: 为工具实现 NeedPermissioner

**Files:**
- Create: `server/internal/eino/tools/http_permission.go`（httprequest 工具的权限包装）

- [ ] **Step 1: 创建 http_permission.go**

由于 httprequest 工具来自 eino-ext 外部库，我们无法直接给它添加方法。需要创建包装器：

```go
package tools

import (
	"encoding/json"
	"strings"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
)

// HTTPToolPermissioner wraps an httprequest tool to implement NeedPermissioner.
type HTTPToolPermissioner struct {
	toolName string
	toolDesc string
	method   string // GET, POST, PUT, DELETE
}

// NewHTTPToolPermissioner creates a NeedPermissioner for an HTTP tool.
func NewHTTPToolPermissioner(name, desc, method string) *HTTPToolPermissioner {
	return &HTTPToolPermissioner{toolName: name, toolDesc: desc, method: method}
}

// NeedPermission implements permission.NeedPermissioner.
func (p *HTTPToolPermissioner) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	url := getString(args, "url")

	var action string
	if p.method == "GET" {
		action = "search"
	} else {
		action = "network"
	}

	return &permission.PermissionRequest{
		Action:      action,
		Content:     p.method + " " + url,
		ToolName:    p.toolName,
		ToolDesc:    p.toolDesc,
		ArgsSummary: truncateJSON(marshalJSON(input), 200),
	}
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

func truncateJSON(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

- [ ] **Step 2: 在 ToolRegistry 中注册权限包装器**

修改 `server/internal/eino/tools/registry.go`，在 `buildBaseTools` 或新增方法中创建带权限的工具列表：

```go
// GetAllTools returns all tools including permission-wrapped ones.
// The returned tools implement NeedPermissioner where applicable.
func (r *ToolRegistry) GetAllTools() []tool.BaseTool {
	base := r.GetBaseTools()

	// Add httprequest tools with permission wrappers.
	httpTools, err := httprequest.NewToolKit(context.Background(), &httprequest.Config{})
	if err != nil {
		// Log but continue without http tools.
		return base
	}

	all := make([]tool.BaseTool, len(base))
	copy(all, base)

	for _, t := range httpTools {
		info, _ := t.Info(context.Background())
		method := strings.ToUpper(info.Name) // "get", "post" → method name
		permWrapper := &PermissionWrapper{
			tool:         t,
			permissioner: NewHTTPToolPermissioner(info.Name, info.Desc, method),
		}
		all = append(all, permWrapper)
	}

	return all
}
```

实际上，更简洁的方式是直接在 Registry 中创建一个带有权限装饰器的工具。让我重新设计。

由于 httprequest 工具来自外部库，最简洁的方式是创建装饰器：

```go
// PermissionWrapper wraps a tool to implement NeedPermissioner.
type PermissionWrapper struct {
	tool         tool.BaseTool
	permissioner *HTTPToolPermissioner
}

func (w *PermissionWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.tool.Info(ctx)
}

func (w *PermissionWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	return w.tool.(tool.InvokableTool).InvokableRun(ctx, argumentsInJSON, opts...)
}

func (w *PermissionWrapper) NeedPermission(input any) *permission.PermissionRequest {
	return w.permissioner.NeedPermission(input)
}
```

- [ ] **Step 3: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/internal/eino/tools/http_permission.go
git commit -m "feat: add NeedPermissioner implementation for httprequest tools"
```

---

### Task 9: 为 local_backend 工具实现 NeedPermissioner

**Files:**
- Create: `server/internal/eino/tools/local_permission.go`

- [ ] **Step 1: 创建 local_permission.go**

```go
package tools

import (
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
)

// LocalBackendPermissioner implements NeedPermissioner for local backend operations.
type LocalBackendPermissioner struct{}

// NeedPermission returns permission requests for local backend operations.
func (p *LocalBackendPermissioner) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	filePath := getString(args, "file_path")
	method := getString(args, "method") // read, write, edit, execute, grep, glob

	var action string
	var content string

	switch method {
	case "read":
		action = "read"
		content = "读取文件: " + filePath
	case "write":
		action = "write"
		content = "创建文件: " + filePath
	case "edit":
		action = "write"
		content = "修改文件: " + filePath
	case "execute":
		action = "execute"
		cmd := getString(args, "command")
		content = "执行命令: " + cmd
	case "grep":
		action = "read"
		content = "搜索文件: " + filePath
	default:
		action = "read"
		content = "文件操作: " + filePath
	}

	return &permission.PermissionRequest{
		Action:   action,
		Content:  content,
		ToolName: "local_backend",
		ToolDesc: "本地文件系统操作工具",
	}
}
```

- [ ] **Step 2: 提交**

```bash
git add server/internal/eino/tools/local_permission.go
git commit -m "feat: add NeedPermissioner for local backend operations"
```

---

### Task 10: 完善 ChatService.runAgent 集成权限中间件

**Files:**
- Modify: `server/internal/service/chat.go`（修改 runAgent 方法）

- [ ] **Step 1: 修改 runAgent 构建 RootRunnerConfig 处**

在 `RootRunnerConfig` 构造中添加 `PermissionMW`：

```go
// Determine model for permission evaluator (use haiku for cost efficiency).
permEvaluator := permission.NewLLMReviewer(haikuChatModel) // create haiku model

permMW := permission.NewMiddleware(permission.MiddlewareConfig{
	DB:             s.db,
	ProjectID:      project.ID,
	UserID:         userID,
	ConversationID: conversationID,
	Evaluator:      permEvaluator,
	Tools:          s.toolRegistry.GetAllTools(), // includes permission-wrapped tools
	PushUpdate:     pushUpdate,
	NextSeq:        nextSeq,
})

runCfg := runner.RootRunnerConfig{
	// ... existing fields ...
	PermissionMW: permMW,
}
```

- [ ] **Step 2: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 3: 提交**

```bash
git add server/internal/service/chat.go
git commit -m "feat: wire permission middleware into runAgent"
```

---

### Task 11: 添加 go.mod 依赖

**Files:**
- Modify: `server/go.mod`

- [ ] **Step 1: 添加 eino-ext httprequest 依赖**

```bash
cd server && go get github.com/cloudwego/eino-ext/components/tool/httprequest@latest
```

- [ ] **Step 2: 清理依赖**

```bash
cd server && go mod tidy
```

- [ ] **Step 3: 编译验证**

```bash
cd server && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/go.mod server/go.sum
git commit -m "chore: add httprequest tool dependency"
```

---

### Task 12: 端到端集成测试

**Files:**
- Create: `server/internal/eino/permission/checker_test.go`
- Create: `server/test_permission.sh`（shell 脚本测试）

- [ ] **Step 1: 创建 checker 单元测试**

```go
package permission

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

func TestChecker_WhitelistMatch(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), nil)
	db.AutoMigrate(&model.ProjectToolPermission{})

	projectID := uuid.New()
	db.Create(&model.ProjectToolPermission{
		ProjectID: projectID,
		ToolName:  "http_get",
		Action:    "search",
		Pattern:   "GET https://api.example.com/data",
		GrantedBy: uuid.New(),
		Level:     "exact",
	})

	checker := NewChecker(db, nil, projectID, uuid.New(), 2)

	tests := []struct {
		name    string
		req     *PermissionRequest
		allowed bool
	}{
		{
			name: "exact match",
			req: &PermissionRequest{
				ToolName: "http_get", Action: "search",
				Content: "GET https://api.example.com/data",
			},
			allowed: true,
		},
		{
			name: "no match",
			req: &PermissionRequest{
				ToolName: "http_get", Action: "search",
				Content: "GET https://other.com/data",
			},
			allowed: false, // no evaluator, so falls through to level 3 default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(context.Background(), tt.req, "")
			if result.Allowed != tt.allowed {
				t.Errorf("allowed = %v, want %v", result.Allowed, tt.allowed)
			}
		})
	}
}

func TestChecker_WildcardMatch(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), nil)
	db.AutoMigrate(&model.ProjectToolPermission{})

	projectID := uuid.New()
	db.Create(&model.ProjectToolPermission{
		ProjectID: projectID,
		ToolName:  "read_file",
		Action:    "read",
		Pattern:   "读取文件: /tmp/*",
		GrantedBy: uuid.New(),
		Level:     "wildcard",
	})

	checker := NewChecker(db, nil, projectID, uuid.New(), 2)

	result := checker.Check(context.Background(), &PermissionRequest{
		ToolName: "read_file",
		Action:   "read",
		Content:  "读取文件: /tmp/test.txt",
	}, "")

	if !result.Allowed {
		t.Error("wildcard match should be allowed")
	}
}

func TestChecker_ApplyDecision(t *testing.T) {
	checker := NewChecker(nil, nil, uuid.New(), uuid.New(), 2)

	tests := []struct {
		decision string
		allowed  bool
		whitelist bool
	}{
		{"approved", true, false},
		{"approved_exact", true, true},
		{"approved_wildcard", true, true},
		{"denied", false, false},
		{"", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.decision, func(t *testing.T) {
			result := checker.Check(context.Background(), &PermissionRequest{
				ToolName: "test_tool",
				Action:   "read",
				Content:  "test",
			}, tt.decision)

			if result.Allowed != tt.allowed {
				t.Errorf("allowed = %v, want %v", result.Allowed, tt.allowed)
			}
			if result.ShouldWriteWhitelist != tt.whitelist {
				t.Errorf("ShouldWriteWhitelist = %v, want %v", result.ShouldWriteWhitelist, tt.whitelist)
			}
		})
	}
}

func TestMapAnswerToDecision(t *testing.T) {
	tests := []struct {
		answer   string
		expected Decision
	}{
		{"同意", DecisionApproved},
		{"同意并记住", DecisionApprovedExact},
		{"同意并通配记住", DecisionApprovedWildcard},
		{"拒绝", DecisionDenied},
		{"random", DecisionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.answer, func(t *testing.T) {
			if got := mapAnswerToDecision(tt.answer); got != tt.expected {
				t.Errorf("mapAnswerToDecision(%q) = %v, want %v", tt.answer, got, tt.expected)
			}
		})
	}
}
```

需要先安装 sqlite 驱动用于测试：

```bash
cd server && go get gorm.io/driver/sqlite
```

- [ ] **Step 2: 运行测试**

```bash
cd server && go test ./internal/eino/permission/... -v
```

- [ ] **Step 3: 提交**

```bash
git add server/internal/eino/permission/checker_test.go server/go.mod server/go.sum
git commit -m "test: add unit tests for permission checker"
```

---

### Task 13: 前端 — PermissionRequestCard 组件

**Files:**
- Create: `web/src/components/chat/PermissionRequestCard.tsx`

- [ ] **Step 1: 创建 PermissionRequestCard 组件**

```tsx
'use client';

import React from 'react';
import { Card, Tag, Button, Space, Typography } from 'antd';
import { CheckOutlined, CloseOutlined, SaveOutlined, GlobalOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';

const { Text, Paragraph } = Typography;

interface PermissionRequestCardProps {
  permissionId: string;
  toolName: string;
  action: string;
  content: string;
  toolDesc?: string;
  safetyLevel: number;
  safetyReason: string;
  onAnswer: (decision: string) => void;
  loading?: boolean;
}

const safetyColors: Record<number, string> = {
  1: 'green',
  2: 'blue',
  3: 'orange',
  4: 'red',
};

const safetyLabels: Record<number, string> = {
  1: '安全',
  2: '低风险',
  3: '中风险',
  4: '高风险',
};

const actionLabels: Record<string, string> = {
  read: '读取',
  write: '写入',
  execute: '执行',
  search: '搜索',
  network: '网络请求',
};

export const PermissionRequestCard: React.FC<PermissionRequestCardProps> = ({
  permissionId,
  toolName,
  action,
  content,
  toolDesc,
  safetyLevel,
  safetyReason,
  onAnswer,
  loading = false,
}) => {
  const t = useTranslations('permission');

  return (
    <Card
      size="small"
      title={
        <Space>
          <Tag color={safetyColors[safetyLevel] || 'default'}>
            {safetyLabels[safetyLevel] || `等级 ${safetyLevel}`}
          </Tag>
          <Text strong>{toolName}</Text>
        </Space>
      }
      extra={
        <Tag>{actionLabels[action] || action}</Tag>
      }
    >
      {toolDesc && (
        <Paragraph type="secondary" style={{ marginBottom: 8 }}>
          {toolDesc}
        </Paragraph>
      )}

      <Paragraph style={{ marginBottom: 8 }}>
        <Text strong>{t('operation')}: </Text>
        <Text code>{content}</Text>
      </Paragraph>

      <Paragraph type="secondary" style={{ marginBottom: 12 }}>
        <Text strong>{t('aiAssessment')}: </Text>
        {safetyReason}
      </Paragraph>

      <Space wrap>
        <Button
          type="primary"
          icon={<CheckOutlined />}
          onClick={() => onAnswer('approved')}
          loading={loading}
        >
          {t('approve')}
        </Button>
        <Button
          icon={<SaveOutlined />}
          onClick={() => onAnswer('approved_exact')}
          loading={loading}
        >
          {t('approveAndRemember')}
        </Button>
        <Button
          icon={<GlobalOutlined />}
          onClick={() => onAnswer('approved_wildcard')}
          loading={loading}
        >
          {t('approveWildcard')}
        </Button>
        <Button
          danger
          icon={<CloseOutlined />}
          onClick={() => onAnswer('denied')}
          loading={loading}
        >
          {t('deny')}
        </Button>
      </Space>
    </Card>
  );
};
```

- [ ] **Step 2: 添加 i18n 文案**

在 `web/src/locales/zh.json` 中添加：

```json
{
  "permission": {
    "operation": "操作",
    "aiAssessment": "AI 评估",
    "approve": "同意",
    "approveAndRemember": "同意并记住",
    "approveWildcard": "同意并通配记住",
    "deny": "拒绝"
  }
}
```

在 `web/src/locales/en.json` 中添加：

```json
{
  "permission": {
    "operation": "Operation",
    "aiAssessment": "AI Assessment",
    "approve": "Approve",
    "approveAndRemember": "Approve & Remember",
    "approveWildcard": "Approve & Wildcard",
    "deny": "Deny"
  }
}
```

- [ ] **Step 3: 提交**

```bash
git add web/src/components/chat/PermissionRequestCard.tsx web/src/locales/zh.json web/src/locales/en.json
git commit -m "feat: add PermissionRequestCard component with i18n"
```

---

### Task 14: 前端 — 集成 Update 处理和消息流

**Files:**
- Modify: `web/src/lib/updateDispatcher.ts`（处理新 Update 类型）
- Modify: `web/src/app/[locale]/chat/[convId]/page.tsx`（在消息流中渲染权限卡片）

- [ ] **Step 1: 在 updateDispatcher.ts 中处理新 Update 类型**

在 UpdateDispatcher 的 update 处理逻辑中，追加：

```typescript
case 'permission.pending':
  // Store permission request in local state / IndexedDB
  // Notify subscribers for the conversation topic
  notifySubscribers('conv:' + payload.conversation_id, update);
  break;
case 'permission.decided':
  // Update the permission request status
  notifySubscribers('conv:' + payload.conversation_id, update);
  break;
```

- [ ] **Step 2: 在聊天页面中渲染权限卡片**

在消息列表组件中，当收到 `permission.pending` Update 时，渲染 `PermissionRequestCard`。用户点击按钮后调用 API：

```typescript
async function handlePermissionAnswer(permissionId: string, decision: string) {
  await fetch(`/api/conversations/${convId}/permissions/${permissionId}/answer`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      checkpoint_id: checkpointId,
      interrupt_id: interruptId,
      decision,
    }),
  });
}
```

- [ ] **Step 3: 提交**

```bash
git add web/src/lib/updateDispatcher.ts web/src/app/\[locale\]/chat/\[convId\]/page.tsx
git commit -m "feat: integrate permission updates into chat flow"
```

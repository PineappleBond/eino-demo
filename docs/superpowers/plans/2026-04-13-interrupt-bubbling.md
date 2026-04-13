# 中断冒泡 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在父对话页面展示子对话的 pending 中断（权限请求和 HITL 问答），用户可以直接在父对话处理

**Architecture:** 权限记录保留在子对话上（不破坏 checkpoint/resume），通过 `GET /conversations/:id/sub-interrupts` 聚合查询接口获取子中断，前端复用现有组件渲染

**Tech Stack:** Go, Gin, GORM, Next.js, React, Ant Design

---

## Task 1: OpenAPI spec — 新增 sub-interrupts 端点

**Files:**

- Modify: `openapi/spec.yaml:384-422` (在 branch 端点之后添加)
- Modify: `server/internal/types/types.go` — 自动生成
- Modify: `web/src/types/api.d.ts` — 自动生成

- [ ] **Step 1: 在 spec.yaml 的 /conversations/{id}/branch 之后添加 /conversations/{id}/sub-interrupts 端点**

在 spec.yaml 第 423 行（branch 端点结束后）添加：

```yaml
  # ── Sub-Interrupts ──
  /conversations/{id}/sub-interrupts:
    get:
      summary: List pending interrupts from sub-conversations
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: List of pending sub-conversation interrupts
          content:
            application/json:
              schema:
                type: object
                required: [ permissions, hitls ]
                properties:
                  permissions:
                    type: array
                    items:
                      $ref: '#/components/schemas/SubInterruptPermission'
                  hitls:
                    type: array
                    items:
                      $ref: '#/components/schemas/SubInterruptHitl'
```

- [ ] **Step 2: 在 spec.yaml 的 schemas 部分添加 SubInterruptPermission 和 SubInterruptHitl 类型**

在 schemas 部分末尾（BranchConversationResponse 之后）添加：

```yaml
    SubInterruptPermission:
      type: object
      required: [ id, conversation_id, tool_name, action, content, safety_level ]
      properties:
        id:
          type: string
          format: uuid
        conversation_id:
          type: string
          format: uuid
          description: The sub-conversation that triggered this interrupt
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
        safety_reason:
          type: string
        checkpoint_id:
          type: string
        interrupt_id:
          type: string

    SubInterruptHitl:
      type: object
      required: [ id, conversation_id, question, choices, answer_type ]
      properties:
        id:
          type: string
          format: uuid
        conversation_id:
          type: string
          format: uuid
          description: The sub-conversation that triggered this interrupt
        question:
          type: string
        choices:
          type: array
          items:
            type: object
            required: [ title ]
            properties:
              title:
                type: string
              desc:
                type: string
        answer_type:
          type: string
          enum: [ single, multi, text ]
        checkpoint_id:
          type: string
        interrupt_id:
          type: string
```

- [ ] **Step 3: 重新生成类型文件**

```bash
cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev && bash openapi/generate.sh
```

Expected: `server/internal/types/types.go` 和 `web/src/types/api.d.ts` 更新，无报错

- [ ] **Step 4: 提交**

```bash
git add openapi/spec.yaml server/internal/types/types.go web/src/types/api.d.ts
git commit -m "feat: add GET /conversations/:id/sub-interrupts endpoint to OpenAPI spec"
```

## Task 2: Service 层 — GetSubInterrupts 方法

**Files:**
- Modify: `server/internal/service/conversation.go` — 新增方法
- Test: `server/internal/service/conversation_test.go` — 新增测试

- [ ] **Step 1: 写失败的测试**

在 `server/internal/service/conversation_test.go` 添加：

```go
func TestConversationService_GetSubInterrupts_NoSubs(t *testing.T) {
	db := setupTestDB(t)
	svc := NewConversationService(db, zap.NewNop())

	// Create a parent conversation
	parentConv := model.Conversation{
		UserID:  testUUID(),
		Title:   "Parent",
		Status:  "active",
		Mode:    "edit_automatically",
	}
	db.Create(&parentConv)

	perms, hitls, err := svc.GetSubInterrupts(context.Background(), testUUID(), parentConv.ID)
	if err != nil {
		t.Fatalf("GetSubInterrupts() error: %v", err)
	}
	if len(perms) != 0 || len(hitls) != 0 {
		t.Errorf("expected empty lists, got %d perms, %d hitls", len(perms), len(hitls))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd server && go test ./internal/service/... -run TestConversationService_GetSubInterrupts_NoSubs -v
```

Expected: FAIL — `svc.GetSubInterrupts undefined`

- [ ] **Step 3: 实现 GetSubInterrupts 方法**

在 `server/internal/service/conversation.go` 末尾添加：

```go
// SubInterruptResult holds pending permissions and HITLs from sub-conversations.
type SubInterruptResult struct {
	Permissions []model.HumanInPermission
	HITLs       []model.HumanInTheLoop
}

// GetSubInterrupts returns all pending permissions and HITLs from sub-conversations
// of the given conversation. This enables the parent conversation page to display
// and resolve sub-conversation interrupts.
func (s *ConversationService) GetSubInterrupts(
	ctx context.Context,
	userID, conversationID uuid.UUID,
) (*SubInterruptResult, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Find all sub-conversations
	var subConvs []model.Conversation
	if err := s.db.WithContext(ctx).Where("parent_conversation_id = ? AND user_id = ?", conversationID, userID).
		Find(&subConvs).Error; err != nil {
		return nil, fmt.Errorf("failed to find sub-conversations: %w", err)
	}

	result := &SubInterruptResult{}
	if len(subConvs) == 0 {
		return result, nil
	}

	// 3. Collect sub-conversation IDs
	subConvIDs := make([]uuid.UUID, len(subConvs))
	for i, sc := range subConvs {
		subConvIDs[i] = sc.ID
	}

	// 4. Query pending permissions from sub-conversations
	if err := s.db.WithContext(ctx).
		Where("conversation_id IN ? AND status = 'pending'", subConvIDs).
		Order("created_at DESC").
		Find(&result.Permissions).Error; err != nil {
		return nil, fmt.Errorf("failed to query sub permissions: %w", err)
	}

	// 5. Query pending HITLs from sub-conversations
	if err := s.db.WithContext(ctx).
		Where("conversation_id IN ? AND status = 'pending'", subConvIDs).
		Order("created_at DESC").
		Find(&result.HITLs).Error; err != nil {
		return nil, fmt.Errorf("failed to query sub hitls: %w", err)
	}

	return result, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
cd server && go test ./internal/service/... -run TestConversationService_GetSubInterrupts -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/service/conversation.go server/internal/service/conversation_test.go
git commit -m "feat: add GetSubInterrupts service method for sub-conversation interrupt aggregation"
```

## Task 3: Handler 层 — GET /conversations/:id/sub-interrupts 端点

**Files:**
- Modify: `server/internal/handler/conversation.go` — 新增路由
- Modify: `server/internal/handler/chat.go` — 参考现有 permission/HITL answer 端点的模式

- [ ] **Step 1: 在 conversation.go 的 RegisterConversationRoutes 中添加路由**

在 `server/internal/handler/conversation.go` 的 `RegisterConversationRoutes` 函数中，添加新的路由处理（在现有的分支路由之后添加）：

```go
	// List pending interrupts from sub-conversations
	api.GET("/conversations/:id/sub-interrupts", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("list sub-interrupts: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		result, err := svc.GetSubInterrupts(c.Request.Context(), userID, conversationID)
		if err != nil {
			if errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			} else {
				log.Error("list sub-interrupts failed",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list sub-interrupts")
			}
			return
		}

		// Convert to response shape
		type subPermResponse struct {
			ID           string `json:"id"`
			ConversationID string `json:"conversation_id"`
			ToolName     string `json:"tool_name"`
			Action       string `json:"action"`
			Content      string `json:"content"`
			ToolDesc     string `json:"tool_desc,omitempty"`
			ArgsSummary  string `json:"args_summary,omitempty"`
			SafetyLevel  int    `json:"safety_level"`
			SafetyReason string `json:"safety_reason"`
			CheckpointID string `json:"checkpoint_id,omitempty"`
			InterruptID  string `json:"interrupt_id,omitempty"`
		}

		type subHitlResponse struct {
			ID            string `json:"id"`
			ConversationID string `json:"conversation_id"`
			Question      string `json:"question"`
			Choices       any    `json:"choices"`
			AnswerType    string `json:"answer_type"`
			CheckpointID  string `json:"checkpoint_id,omitempty"`
			InterruptID   string `json:"interrupt_id,omitempty"`
		}

		perms := make([]subPermResponse, len(result.Permissions))
		for i, p := range result.Permissions {
			perms[i] = subPermResponse{
				ID:             p.ID.String(),
				ConversationID: p.ConversationID.String(),
				ToolName:       p.ToolName,
				Action:         p.Action,
				Content:        p.Content,
				ToolDesc:       p.ToolDesc,
				ArgsSummary:    p.ArgsSummary,
				SafetyLevel:    p.SafetyLevel,
				SafetyReason:   p.SafetyReason,
				CheckpointID:   p.CheckpointID,
				InterruptID:    p.InterruptID,
			}
		}

		hitls := make([]subHitlResponse, len(result.HITLs))
		for i, h := range result.HITLs {
			hitls[i] = subHitlResponse{
				ID:             h.ID.String(),
				ConversationID: h.ConversationID.String(),
				Question:       h.Question,
				Choices:        h.Choices,
				AnswerType:     h.AnswerType,
				CheckpointID:   h.CheckpointID,
				InterruptID:    h.InterruptID,
			}
		}

		respondJSON(c, http.StatusOK, gin.H{
			"permissions": perms,
			"hitls":       hitls,
		})
	})
```

- [ ] **Step 2: 编译确认**

```bash
cd server && go build ./...
```

Expected: zero errors

- [ ] **Step 3: 提交**

```bash
git add server/internal/handler/conversation.go
git commit -m "feat: wire GET /conversations/:id/sub-interrupts endpoint"
```

## Task 4: 权限中间件 — 创建中断时赋值 SourceConversationID

**Files:**
- Modify: `server/internal/eino/permission/middleware.go:356-358` (interruptForPermission 方法)
- Modify: `server/internal/eino/permission/checker.go:163-180` (CreatePendingPerm 函数)

- [ ] **Step 1: 修改 CreatePendingPerm 接收 SourceConversationID 参数**

在 `server/internal/eino/permission/checker.go` 中，找到 `CreatePendingPerm` 函数签名（约第 163 行），修改为：

```go
// CreatePendingPerm creates a pending HumanInPermission record.
func CreatePendingPerm(
	db *gorm.DB,
	conversationID uuid.UUID,
	toolName, action, content, toolDesc, argsSummary string,
	safetyLevel int,
	safetyReason string,
	sourceConversationID *uuid.UUID,
) (*model.HumanInPermission, error) {
	perm := &model.HumanInPermission{
		ConversationID:     conversationID,
		SourceConversationID: sourceConversationID,
		ToolName:           toolName,
		Action:             action,
		Content:            content,
		ToolDesc:           toolDesc,
		ArgsSummary:        argsSummary,
		SafetyLevel:        safetyLevel,
		SafetyReason:       safetyReason,
		Status:             "pending",
	}
	if err := db.Create(perm).Error; err != nil {
		return nil, err
	}
	return perm, nil
}
```

- [ ] **Step 2: 更新 checker.go 中 WriteWhitelist 的调用**

检查 checker.go 中是否有其他地方调用 CreatePendingPerm，如果有，更新调用传入 `nil`。

- [ ] **Step 3: 修改 middleware.go 中 CreatePendingPerm 的调用**

在 `middleware.go` 中，找到两处 `CreatePendingPerm` 调用：

1. `checkPermissionAskBeforeEdits` 方法（约第 235 行）：

```go
perm, err := CreatePendingPerm(m.cfg.DB, m.cfg.ConversationID, req.ToolName, req.Action, req.Content, req.ToolDesc, req.ArgsSummary, 0, "ask_before_edits 模式：每次操作都需要确认", &m.cfg.ConversationID)
```

2. `interruptForPermission` 方法（约第 357 行）：

```go
perm, err := CreatePendingPerm(m.cfg.DB, m.cfg.ConversationID, req.ToolName, req.Action, req.Content, req.ToolDesc, req.ArgsSummary, eval.Level, eval.Reason, &m.cfg.ConversationID)
```

- [ ] **Step 4: 更新 rootrunner_callbacks.go 中 HITL 创建**

在 `server/internal/eino/runner/rootrunner_callbacks.go` 的 `OnInterrupted` 方法中（约第 809 行），HITL 创建时添加 SourceConversationID：

```go
hitl := model.HumanInTheLoop{
	ConversationID:       c.cfg.ConversationID,
	SourceConversationID: &c.cfg.ConversationID,
	CheckpointID:         c.cfg.ConversationID.String(),
	InterruptID:          interruptID,
	Question:             hitlData.Question,
	Choices:              model.JSONMap{"choices": choicesAny},
	AnswerType:           hitlData.AnswerType,
	Status:               "pending",
}
```

同时更新第 789-795 行的 permission 更新查询，增加 `source_conversation_id` 条件：

```go
c.cfg.DB.WithContext(ctx).Table("human_in_permissions").
	Where("conversation_id = ? AND checkpoint_id = '' AND interrupt_id = '' AND status = 'pending'",
		c.cfg.ConversationID).
	Updates(map[string]interface{}{
		"checkpoint_id":          checkpointKey,
		"interrupt_id":           interruptID,
		"source_conversation_id": c.cfg.ConversationID.String(),
	})
```

- [ ] **Step 5: 编译确认**

```bash
cd server && go build ./...
```

Expected: zero errors

- [ ] **Step 6: 提交**

```bash
git add server/internal/eino/permission/middleware.go server/internal/eino/permission/checker.go server/internal/eino/runner/rootrunner_callbacks.go
git commit -m "feat: set SourceConversationID when creating interrupt records"
```

## Task 5: 前端 — 获取并渲染子中断

**Files:**
- Modify: `web/src/app/[locale]/project/[id]/chat/[convId]/page.tsx`
- Modify: `web/src/types/api.d.ts` — 自动生成（Task 1 已完成）

- [ ] **Step 1: 在 ChatState 中添加子中断状态**

在 `page.tsx` 的 `ChatState` 接口中添加：

```typescript
interface ChatState {
  // ... existing fields ...
  // Sub-conversation interrupts
  subInterruptPermissions: Map<string, components['schemas']['SubInterruptPermission']>; // permId -> payload
  subInterruptHitLs: Map<string, components['schemas']['SubInterruptHitl']>; // hitlId -> payload
  subInterruptFetching: boolean;
  // For HITL modal shared between parent and sub
  activeSubHitl: components['schemas']['SubInterruptHitl'] | null;
}
```

在 `ChatAction` 类型中添加：

```typescript
  | { type: 'SET_SUB_INTERRUPTS'; payload: { permissions: components['schemas']['SubInterruptPermission'][]; hitls: components['schemas']['SubInterruptHitl'][] } }
  | { type: 'SET_SUB_HITL_MODAL'; payload: components['schemas']['SubInterruptHitl'] | null }
  | { type: 'REMOVE_SUB_PERMISSION'; payload: string } // permId
  | { type: 'REMOVE_SUB_HITL'; payload: string } // hitlId
```

在 `chatReducer` 中添加处理：

```typescript
    case 'SET_SUB_INTERRUPTS': {
      const permMap = new Map<string, components['schemas']['SubInterruptPermission']>();
      for (const p of action.payload.permissions) {
        permMap.set(p.id, p);
      }
      const hitlMap = new Map<string, components['schemas']['SubInterruptHitl']>();
      for (const h of action.payload.hitls) {
        hitlMap.set(h.id, h);
      }
      return {
        ...state,
        subInterruptPermissions: permMap,
        subInterruptHitLs: hitlMap,
        subInterruptFetching: false,
      };
    }
    case 'SET_SUB_HITL_MODAL':
      return { ...state, activeSubHitl: action.payload };
    case 'REMOVE_SUB_PERMISSION': {
      const newPerms = new Map(state.subInterruptPermissions);
      newPerms.delete(action.payload);
      return { ...state, subInterruptPermissions: newPerms };
    }
    case 'REMOVE_SUB_HITL': {
      const newHitls = new Map(state.subInterruptHitLs);
      newHitls.delete(action.payload);
      return { ...state, subInterruptHitLs: newHitls, activeSubHitl: null };
    }
```

在 `initialState` 中添加：

```typescript
  subInterruptPermissions: new Map(),
  subInterruptHitLs: new Map(),
  subInterruptFetching: false,
  activeSubHitl: null,
```

- [ ] **Step 2: 添加获取子中断的 useEffect**

在页面组件中，添加获取子中断的 effect（在现有的 members 获取 effect 之后）：

```typescript
  // ─── Fetch sub-conversation interrupts ───

  useEffect(() => {
    api.get<{ permissions: any[]; hitls: any[] }>(
      `/conversations/${convId}/sub-interrupts`
    )
      .then((data) => {
        dispatch({
          type: 'SET_SUB_INTERRUPTS',
          payload: { permissions: data.permissions, hitls: data.hitls },
        });
      })
      .catch(() => {
        dispatch({
          type: 'SET_SUB_INTERRUPTS',
          payload: { permissions: [], hitls: [] },
        });
      });
  }, [convId]);
```

- [ ] **Step 3: 渲染子权限卡片**

在页面 JSX 中，在输入框上方（现有的 `pendingPermissions` 渲染区域之后）添加子权限渲染：

```tsx
{/* Sub-conversation permission requests */}
{Array.from(state.subInterruptPermissions.values()).map((perm) => (
  <div key={perm.id} style={{ margin: '0 16px', opacity: 0.85 }}>
    <div style={{ fontSize: 11, color: '#999', marginBottom: 4 }}>
      来自子对话 ({perm.conversation_id.slice(0, 8)})
    </div>
    <PermissionRequestCard
      permission={{
        conversation_id: perm.conversation_id,
        permission_id: perm.id,
        tool_name: perm.tool_name,
        action: perm.action,
        content: perm.content,
        tool_desc: perm.tool_desc,
        args_summary: perm.args_summary,
        safety_level: perm.safety_level,
        safety_reason: perm.safety_reason,
        checkpoint_id: perm.checkpoint_id,
        interrupt_id: perm.interrupt_id,
        seq: 0,
      }}
      onAnswer={async (decision: string) => {
        try {
          await api.post(`/conversations/${perm.conversation_id}/permissions/${perm.id}/answer`, {
            checkpoint_id: perm.checkpoint_id || '',
            interrupt_id: perm.interrupt_id || '',
            decision,
          });
          dispatch({ type: 'REMOVE_SUB_PERMISSION', payload: perm.id });
        } catch {
          message.error('Failed to answer permission');
        }
      }}
    />
  </div>
))}
```

- [ ] **Step 4: 渲染子 HITL 弹窗**

将 `Spin, Result, App` 改为 `Spin, Result, App, Card`，并从 antd 导入 `Typography`：

```tsx
import { Spin, Result, App, Card, Typography } from 'antd';
const { Text } = Typography;
```

在页面 JSX 末尾，在现有的 `<HitlModal>` 之后添加子 HITL 渲染：

```tsx
<HitlModal
  open={state.activeSubHitl !== null}
  question={state.activeSubHitl?.question || ''}
  choices={state.activeSubHitl?.choices || []}
  answerType={(state.activeSubHitl?.answer_type as 'single' | 'multi' | 'text') || 'text'}
  onAnswer={async (answer: string) => {
    if (!state.activeSubHitl) return;
    try {
      await api.post(`/conversations/${state.activeSubHitl.conversation_id}/answer`, {
        checkpoint_id: state.activeSubHitl.checkpoint_id || '',
        interrupt_id: state.activeSubHitl.interrupt_id || '',
        answer,
      });
      dispatch({ type: 'REMOVE_SUB_HITL', payload: state.activeSubHitl.id });
      dispatch({ type: 'SET_SUB_HITL_MODAL', payload: null });
    } catch {
      message.error('Failed to answer question');
    }
  }}
/>
```

在子权限卡片的 `PermissionRequestCard` 中，添加点击功能，对于 HITL 类型的子中断，需要点击后弹窗。修改 HITL 子中断的渲染为可点击卡片，点击后设置 `activeSubHitl`：

```tsx
{/* Sub-conversation HITL requests */}
{Array.from(state.subInterruptHitLs.values()).map((hitl) => (
  <div key={hitl.id} style={{ margin: '0 16px', opacity: 0.85 }}>
    <div style={{ fontSize: 11, color: '#999', marginBottom: 4 }}>
      来自子对话 ({hitl.conversation_id.slice(0, 8)})
    </div>
    <Card
      size="small"
      style={{ cursor: 'pointer' }}
      onClick={() => dispatch({ type: 'SET_SUB_HITL_MODAL', payload: hitl })}
    >
      <Text strong>子对话提问:</Text> {hitl.question}
    </Card>
  </div>
))}
```

- [ ] **Step 5: 编译确认**

```bash
cd web && npx tsc --noEmit
```

Expected: zero errors

- [ ] **Step 6: 提交**

```bash
git add web/src/app/\[locale\]/project/\[id\]/chat/\[convId\]/page.tsx
git commit -m "feat: display and resolve sub-conversation interrupts on parent chat page"
```

---

## 文件改动总结

| 文件 | 操作 |
|------|------|
| `openapi/spec.yaml` | 新增 /conversations/:id/sub-interrupts 端点 + SubInterruptPermission/SubInterruptHitl schema |
| `server/internal/types/types.go` | 自动生成 |
| `web/src/types/api.d.ts` | 自动生成 |
| `server/internal/service/conversation.go` | 新增 GetSubInterrupts 方法 |
| `server/internal/service/conversation_test.go` | 新增测试 |
| `server/internal/handler/conversation.go` | 新增 GET /conversations/:id/sub-interrupts 路由 |
| `server/internal/eino/permission/checker.go` | CreatePendingPerm 增加 sourceConversationID 参数 |
| `server/internal/eino/permission/middleware.go` | 调用处传入 &m.cfg.ConversationID |
| `server/internal/eino/runner/rootrunner_callbacks.go` | HITL 创建和 permission 更新时设置 SourceConversationID |
| `web/src/app/[locale]/project/[id]/chat/[convId]/page.tsx` | 获取子中断、渲染子权限卡片和子 HITL 弹窗 |

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/tools"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// TruncatedContent returns the first maxLen characters of s.
func TruncatedContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RunCallbackConfig holds the dependencies needed by callback implementations.
type RunCallbackConfig struct {
	UserID         uuid.UUID
	ConversationID uuid.UUID
	DB             *gorm.DB
	Log            *zap.Logger
	NextSeq        func(ctx context.Context, userID uuid.UUID) (int64, error)
	PushUpdate     func(userID uuid.UUID, update model.UserUpdate)
	ParentCtx      context.Context // parent context (not cancelled), used for lifecycle methods

	// JSONLLogger is optional. When set, every callback also writes a JSONL entry.
	// Used by sub-agents to produce a structured log file.
	JSONLLogger *JSONLLogger

	// OnComplete is optional. Called from OnEnd (success=true) and OnError (success=false).
	// Used by sub-agents to write a summary back to the parent conversation.
	OnComplete func(success bool, summary string)
}

// msgTracker tracks a single message being streamed or written.
type msgTracker struct {
	addr      string
	role      string // "assistant" or "tool"
	toolName  string // only for tool messages
	senderID  string // agent name or tool name
	completed bool

	msgID uuid.UUID
	seq   int64

	content strings.Builder
	reason  strings.Builder

	// tool_calling: only for tool messages
	toolInput  map[string]any // parsed from ArgumentsInJSON
	toolOutput string         // tool result
}

// RootRunnerCallbacks implements RootRunnerCallback. It bridges Eino callbacks
// to WebSocket Update events and persists messages/checkpoints to PostgreSQL in real time.
type RootRunnerCallbacks struct {
	cfg      RunCallbackConfig
	mu       sync.Mutex                // guards trackers map and DB writes
	trackers map[[2]string]*msgTracker // keyed by [2]string{addrStr, role}

	// Accumulated token counts across all LLM calls in this run.
	totalPromptTokens     int
	totalCompletionTokens int
}

// NewRootRunnerCallbacks creates a callback handler for one agent run.
func NewRootRunnerCallbacks(cfg RunCallbackConfig) *RootRunnerCallbacks {
	return &RootRunnerCallbacks{
		cfg:      cfg,
		trackers: make(map[[2]string]*msgTracker),
	}
}

// markTrackerStopped patches an in-progress message with finish_reason="error" (stopped by user).
func (c *RootRunnerCallbacks) markTrackerStopped(t *msgTracker) error {
	if t.completed || t.msgID == uuid.Nil {
		return nil
	}
	t.completed = true
	finishReason := "error"
	if err := c.cfg.DB.WithContext(c.cfg.ParentCtx).Model(&model.Message{}).
		Where("id = ?", t.msgID).
		Updates(map[string]interface{}{
			"finish_reason": finishReason,
		}).Error; err != nil {
		c.cfg.Log.Error("failed to mark message as stopped", zap.Error(err))
		return err
	}
	return nil
}

// Stop marks all in-progress trackers as stopped and pushes a message.stop update.
// If all trackers is already complete, no update is pushed (no-op).
func (c *RootRunnerCallbacks) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cfg.ParentCtx == nil {
		c.cfg.ParentCtx = context.Background()
	}

	// Mark all in-progress messages as stopped
	stoppedAny := false
	for _, t := range c.trackers {
		if !t.completed {
			c.markTrackerStopped(t)
			stoppedAny = true
		}
	}

	// If all trackers were already complete, skip — nothing to stop.
	if !stoppedAny {
		return
	}

	// Push message.stop update for the assistant message
	seq, err := c.cfg.NextSeq(c.cfg.ParentCtx, c.cfg.UserID)
	if err != nil {
		c.cfg.Log.Error("seq assignment failed for stop", zap.Error(err))
		return
	}

	payload := model.JSONMap{
		"conversation_id": c.cfg.ConversationID.String(),
		"seq":             seq,
	}
	for _, t := range c.trackers {
		if t.role == "assistant" {
			payload["message_id"] = t.msgID.String()
			break
		}
	}

	update := model.UserUpdate{
		UserID:  c.cfg.UserID,
		Seq:     seq,
		Type:    "message.stop",
		Payload: payload,
	}
	if dbErr := c.cfg.DB.WithContext(c.cfg.ParentCtx).Create(&update).Error; dbErr != nil {
		c.cfg.Log.Error("failed to persist stop update", zap.Error(dbErr))
		return
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)
}

// getOrCreateTracker returns the tracker for this addr.
// If the existing tracker is completed, a new one is created.
func (c *RootRunnerCallbacks) getOrCreateTracker(addrStr string, role string, toolName string) *msgTracker {
	key := [2]string{addrStr, role}
	t, ok := c.trackers[key]
	if ok {
		return t
	}

	// Determine sender_id from addr
	senderID := addrStr // default to full addr as sender identity

	t = &msgTracker{
		addr:     addrStr,
		role:     role,
		toolName: toolName,
		senderID: senderID,
	}
	c.trackers[key] = t
	return t
}

// getOrCreateAssistantTracker returns or creates a tracker for this addr.
// If the addr's tracker is already completed, a new one is created (for a new response).
func (c *RootRunnerCallbacks) getOrCreateAssistantTracker(addrStr string) *msgTracker {
	key := [2]string{addrStr, "assistant"}
	t, ok := c.trackers[key]
	if ok {
		return t
	}

	t = &msgTracker{
		addr:     addrStr,
		role:     "assistant",
		senderID: "agent:root",
	}
	c.trackers[key] = t
	return t
}

// getOrCreateAssistantTracker returns or creates a tracker for this addr.
// If the addr's tracker is already completed, a new one is created (for a new response).
func (c *RootRunnerCallbacks) deleteTracker(addrStr string, role string) {
	key := [2]string{addrStr, role}
	delete(c.trackers, key)
}

// insertMessage creates a message in DB and assigns seq.
func (c *RootRunnerCallbacks) insertMessage(ctx context.Context, t *msgTracker, initialContent string, initialReason string) error {
	seq, err := c.cfg.NextSeq(ctx, c.cfg.UserID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}
	t.seq = seq
	t.msgID = uuid.New()

	msg := model.Message{
		ConversationID: c.cfg.ConversationID,
		SenderRole:     t.role,
		SenderID:       t.senderID,
		Content:        initialContent,
		ReasonContent:  initialReason,
		Seq:            seq,
		Metadata:       model.JSONMap{},
	}
	msg.ID = t.msgID
	if t.role == "tool" {
		msg.Metadata = model.JSONMap{
			"tool_name": t.toolName,
			"addr":      t.addr,
		}
		if t.toolInput != nil {
			msg.ToolCalling = model.JSONMap{
				"input":  t.toolInput,
				"output": t.toolOutput,
			}
		}
	}
	if err := c.cfg.DB.WithContext(ctx).Create(&msg).Error; err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	// Push message.new update
	update := model.UserUpdate{
		UserID: c.cfg.UserID,
		Seq:    seq,
		Type:   "message.new",
		Payload: model.JSONMap{
			"conversation_id": c.cfg.ConversationID.String(),
			"message_id":      t.msgID.String(),
			"seq":             seq,
			"role":            t.role,
			"sender_id":       t.senderID,
			"addr":            t.addr,
		},
	}
	if t.role == "tool" {
		update.Payload["tool_name"] = t.toolName
		if t.toolInput != nil {
			update.Payload["tool_calling"] = model.JSONMap{
				"input":  t.toolInput,
				"output": t.toolOutput,
			}
		}
	}
	if err := c.cfg.DB.WithContext(ctx).Create(&update).Error; err != nil {
		c.cfg.Log.Error("failed to persist message.new update", zap.Error(err))
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)

	return nil
}

// updateMessage patches content into an existing message.
func (c *RootRunnerCallbacks) updateMessage(ctx context.Context, t *msgTracker, content string, reasonContent string) error {
	updates := map[string]interface{}{}
	if content != "" {
		updates["content"] = content
	}
	if reasonContent != "" {
		updates["reason_content"] = reasonContent
	}
	if len(updates) == 0 {
		return nil
	}
	return c.cfg.DB.WithContext(ctx).Model(&model.Message{}).Where("id = ?", t.msgID).Updates(updates).Error
}

// completeMessage marks a message as finished with final content.
func (c *RootRunnerCallbacks) completeMessage(ctx context.Context, t *msgTracker, content string, reasonContent string, usage *schema.TokenUsage) error {
	if content == "" {
		content = t.content.String()
	}
	if reasonContent == "" {
		reasonContent = t.reason.String()
	}
	t.completed = true
	finishReason := "stop"

	updates := map[string]interface{}{
		"content":        content,
		"reason_content": reasonContent,
		"finish_reason":  finishReason,
	}
	if usage != nil {
		updates["token_prompt"] = usage.PromptTokens
		updates["token_completion"] = usage.CompletionTokens
		// Accumulate across all LLM calls in this run
		c.totalPromptTokens += usage.PromptTokens
		c.totalCompletionTokens += usage.CompletionTokens
	}

	if err := c.cfg.DB.WithContext(ctx).Model(&model.Message{}).Where("id = ?", t.msgID).Updates(updates).Error; err != nil {
		c.cfg.Log.Error("failed to complete message", zap.Error(err))
		return err
	}

	// Push message.done update
	seq, err := c.cfg.NextSeq(ctx, c.cfg.UserID)
	if err != nil {
		c.cfg.Log.Error("seq assignment failed", zap.Error(err))
	} else {
		update := model.UserUpdate{
			UserID: c.cfg.UserID,
			Seq:    seq,
			Type:   "message.done",
			Payload: model.JSONMap{
				"conversation_id":   c.cfg.ConversationID.String(),
				"message_id":        t.msgID.String(),
				"role":              t.role,
				"sender_id":         t.senderID,
				"content":           content,
				"reasoning_content": reasonContent,
				"addr":              t.addr,
				"seq":               seq,
			},
		}
		if err := c.cfg.DB.WithContext(ctx).Create(&update).Error; err != nil {
			c.cfg.Log.Error("failed to persist message.done update", zap.Error(err))
		}
		c.cfg.PushUpdate(c.cfg.UserID, update)
	}

	return nil
}

// ---- RootRunnerHandlerCallback methods ----

func (c *RootRunnerCallbacks) OnInputToolCalling(ctx context.Context, info *callbacks.RunInfo, addr compose.Address, input callbacks.CallbackInput) {
	addrStr := AddrString(addr)
	c.cfg.Log.Info("tool calling",
		zap.String("tool", info.Name),
		zap.String("addr", addrStr),
	)

	// JSONL logging for sub-agents
	if c.cfg.JSONLLogger != nil {
		toolInput := tool.ConvCallbackInput(input)
		inputStr := ""
		if toolInput != nil {
			inputStr = toolInput.ArgumentsInJSON
		}
		_ = c.cfg.JSONLLogger.Log(JSONLLogEntry{
			Type:      "message.tool_call",
			Tool:      info.Name,
			ToolInput: inputStr,
		})
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a tool message immediately
	t := c.getOrCreateTracker(addrStr, "tool", info.Name)
	if t.msgID == uuid.Nil {
		t.senderID = info.Name

		// Parse tool input from callback input
		toolInput := tool.ConvCallbackInput(input)
		if toolInput != nil && toolInput.ArgumentsInJSON != "" {
			var args map[string]any
			if err := json.Unmarshal([]byte(toolInput.ArgumentsInJSON), &args); err == nil {
				t.toolInput = args
			}
		}

		if err := c.insertMessage(ctx, t, "", ""); err != nil {
			c.cfg.Log.Error("failed to insert tool message", zap.Error(err))
			return
		}
	} else {
		// Update tool input on existing tracker (e.g. re-entry)
		toolInput := tool.ConvCallbackInput(input)
		if toolInput != nil && toolInput.ArgumentsInJSON != "" {
			var args map[string]any
			if err := json.Unmarshal([]byte(toolInput.ArgumentsInJSON), &args); err == nil {
				t.toolInput = args
			}
		}
	}
}

func (c *RootRunnerCallbacks) OnOutputToolCalling(ctx context.Context, info *callbacks.RunInfo, addr compose.Address, output callbacks.CallbackOutput) {
	addrStr := AddrString(addr)

	// JSONL logging for sub-agents
	if c.cfg.JSONLLogger != nil {
		var resultStr string
		if output != nil {
			toolOutput := tool.ConvCallbackOutput(output)
			if toolOutput != nil {
				resultStr = toolOutput.Response
			}
		}
		_ = c.cfg.JSONLLogger.Log(JSONLLogEntry{
			Type:       "message.tool_call",
			Tool:       info.Name,
			ToolOutput: TruncatedContent(resultStr, 500),
			Status:     "completed",
		})
	}

	c.mu.Lock()
	defer func() {
		c.deleteTracker(addrStr, "tool")
		c.mu.Unlock()
	}()

	t := c.getOrCreateAssistantTracker(addrStr)
	if t.msgID != uuid.Nil {
		c.completeMessage(ctx, t, "", "", nil)
		c.deleteTracker(addrStr, "assistant")
	}

	t, ok := c.trackers[[2]string{addrStr, "tool"}]
	if !ok {
		c.cfg.Log.Warn("no tracker for tool output", zap.String("addr", addrStr))
		return
	}

	// Extract tool result from output
	var resultStr string
	if output != nil {
		toolOutput := tool.ConvCallbackOutput(output)
		if toolOutput != nil {
			resultStr = toolOutput.Response
		}
		if resultStr == "" {
			resultStr = fmt.Sprintf("%v", output)
		}
	}

	t.toolOutput = resultStr
	t.completed = true

	// Save tool_calling to DB
	toolCalling := model.JSONMap{
		"input":  t.toolInput,
		"output": resultStr,
	}
	finishReason := "stop"
	c.cfg.DB.WithContext(ctx).Model(&model.Message{}).
		Where("id = ?", t.msgID).
		Updates(map[string]interface{}{
			"content":        resultStr,
			"reason_content": "",
			"finish_reason":  finishReason,
			"tool_calling":   toolCalling,
		})

	// Push message.done update
	seq, err := c.cfg.NextSeq(ctx, c.cfg.UserID)
	if err != nil {
		c.cfg.Log.Error("seq assignment failed", zap.Error(err))
		return
	}

	update := model.UserUpdate{
		UserID: c.cfg.UserID,
		Seq:    seq,
		Type:   "message.tool_call",
		Payload: model.JSONMap{
			"conversation_id": c.cfg.ConversationID.String(),
			"message_id":      t.msgID.String(),
			"tool_name":       info.Name,
			"content":         TruncatedContent(resultStr, 200),
			"seq":             seq,
			"addr":            addrStr,
			"tool_calling":    toolCalling,
		},
	}
	if dbErr := c.cfg.DB.WithContext(ctx).Create(&update).Error; dbErr != nil {
		c.cfg.Log.Error("failed to persist tool_call update", zap.Error(dbErr))
		return
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)
}

// ensureAssistantMessage returns the assistant tracker for this run,
// creating the tracker and inserting the message in DB if it doesn't exist yet.
// The tracker's msgID is set before this returns, so callers can include it
// in streaming delta payloads.
//
// It first checks for any existing incomplete assistant tracker (regardless of
// addr) to avoid creating multiple messages when the callback address changes
// between OnThinking and OnOutputting.
func (c *RootRunnerCallbacks) ensureAssistantMessage(ctx context.Context, addrStr string) *msgTracker {
	t := c.getOrCreateAssistantTracker(addrStr)
	if t.msgID == uuid.Nil {
		if err := c.insertMessage(ctx, t, "", ""); err != nil {
			c.cfg.Log.Error("failed to insert assistant message", zap.Error(err))
			return nil
		}
	}
	return t
}

func (c *RootRunnerCallbacks) OnThinking(ctx context.Context, role schema.RoleType, addr compose.Address, reasoningContent string) {
	addrStr := AddrString(addr)
	c.cfg.Log.Debug("thinking",
		zap.String("addr", addrStr),
		zap.Int("content_len", len(reasoningContent)),
	)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure message exists first so we have a message_id for the delta
	t := c.ensureAssistantMessage(ctx, addrStr)
	if t == nil {
		return
	}

	// Push streaming delta with message_id for frontend routing
	update := model.UserUpdate{
		UserID: c.cfg.UserID,
		Seq:    0,
		Type:   "message.thinking",
		Payload: model.JSONMap{
			"conversation_id": c.cfg.ConversationID.String(),
			"message_id":      t.msgID.String(),
			"delta":           reasoningContent,
			"addr":            addrStr,
		},
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)

	// Accumulate reasoning on the tracker
	t.reason.WriteString(reasoningContent)
	c.updateMessage(ctx, t, "", t.reason.String())
}

func (c *RootRunnerCallbacks) OnOutputting(ctx context.Context, role schema.RoleType, addr compose.Address, content string) {
	addrStr := AddrString(addr)
	c.cfg.Log.Debug("outputting",
		zap.String("addr", addrStr),
		zap.Int("content_len", len(content)),
	)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure message exists first so we have a message_id for the delta
	t := c.ensureAssistantMessage(ctx, addrStr)
	if t == nil {
		return
	}

	// Push streaming delta with message_id for frontend routing
	update := model.UserUpdate{
		UserID: c.cfg.UserID,
		Seq:    0,
		Type:   "message.delta",
		Payload: model.JSONMap{
			"conversation_id": c.cfg.ConversationID.String(),
			"message_id":      t.msgID.String(),
			"delta":           content,
			"addr":            addrStr,
		},
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)

	// Accumulate content on the tracker
	t.content.WriteString(content)
	c.updateMessage(ctx, t, t.content.String(), "")
}

func (c *RootRunnerCallbacks) OnCompleted(ctx context.Context, role schema.RoleType, addr compose.Address, reasoningContent string, outputContent string, usage *schema.TokenUsage) {
	addrStr := AddrString(addr)
	c.cfg.Log.Info("completed",
		zap.String("addr", addrStr),
		zap.Int("content_len", len(outputContent)),
	)

	// JSONL logging for sub-agents
	if c.cfg.JSONLLogger != nil {
		tokenPrompt := int64(0)
		tokenCompletion := int64(0)
		if usage != nil {
			tokenPrompt = int64(usage.PromptTokens)
			tokenCompletion = int64(usage.CompletionTokens)
		}
		_ = c.cfg.JSONLLogger.Log(JSONLLogEntry{
			Type:            "message.done",
			Role:            "assistant",
			Content:         TruncatedContent(outputContent, 500),
			TokenPrompt:     tokenPrompt,
			TokenCompletion: tokenCompletion,
		})
	}

	c.mu.Lock()

	defer func() {
		c.deleteTracker(addrStr, "assistant")
		c.mu.Unlock()
	}()

	t := c.ensureAssistantMessage(ctx, addrStr)
	if t == nil {
		return
	}

	c.completeMessage(ctx, t, outputContent, reasoningContent, usage)
}

// ---- RootRunnerCallback lifecycle methods ----

func (c *RootRunnerCallbacks) OnError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// JSONL logging for sub-agents
	if c.cfg.JSONLLogger != nil {
		_ = c.cfg.JSONLLogger.Log(JSONLLogEntry{
			Type:  "error",
			Error: err.Error(),
		})
	}

	if c.cfg.ParentCtx == nil {
		c.cfg.ParentCtx = context.Background()
	}

	seq, seqErr := c.cfg.NextSeq(c.cfg.ParentCtx, c.cfg.UserID)
	if seqErr != nil {
		c.cfg.Log.Error("seq assignment failed", zap.Error(seqErr))
		// Still trigger OnComplete for sub-agents even if seq failed
		if c.cfg.OnComplete != nil {
			c.cfg.OnComplete(false, "agent error: "+err.Error())
		}
		return
	}

	payload := model.JSONMap{
		"conversation_id": c.cfg.ConversationID.String(),
		"error":           err.Error(),
		"seq":             seq,
	}
	for _, t := range c.trackers {
		if t.role == "assistant" {
			payload["message_id"] = t.msgID.String()
			break
		}
	}

	update := model.UserUpdate{
		UserID:  c.cfg.UserID,
		Seq:     seq,
		Type:    "message.error",
		Payload: payload,
	}
	if dbErr := c.cfg.DB.WithContext(c.cfg.ParentCtx).Create(&update).Error; dbErr != nil {
		c.cfg.Log.Error("failed to persist error update", zap.Error(dbErr))
	} else {
		c.cfg.PushUpdate(c.cfg.UserID, update)
	}

	// Notify sub-agent completion callback
	if c.cfg.OnComplete != nil {
		c.cfg.OnComplete(false, "agent error: "+err.Error())
	}
}

func (c *RootRunnerCallbacks) OnEnd() {
	c.mu.Lock()
	defer c.mu.Unlock()

	completed := 0
	for _, t := range c.trackers {
		if t.completed {
			completed++
		}
	}

	c.cfg.Log.Info("agent run ended",
		zap.String("conv", c.cfg.ConversationID.String()),
		zap.Int("total_messages", len(c.trackers)),
		zap.Int("completed_messages", completed),
	)

	// JSONL logging for sub-agents
	if c.cfg.JSONLLogger != nil {
		_ = c.cfg.JSONLLogger.Log(JSONLLogEntry{
			Type:   "message.done",
			Status: "completed",
			Content: fmt.Sprintf("agent run ended: %d/%d messages completed",
				completed, len(c.trackers)),
		})
	}

	// Bump conversation's updated_at
	if c.cfg.ParentCtx == nil {
		c.cfg.ParentCtx = context.Background()
	}
	c.cfg.DB.WithContext(c.cfg.ParentCtx).Model(&model.Conversation{}).
		Where("id = ?", c.cfg.ConversationID).
		Updates(map[string]interface{}{
			"updated_at":       NowFunc(),
			"token_prompt":     c.totalPromptTokens,
			"token_completion": c.totalCompletionTokens,
		})

	// Notify sub-agent completion callback
	if c.cfg.OnComplete != nil {
		summary := fmt.Sprintf("子对话已完成，共 %d 条消息", completed)
		c.cfg.OnComplete(true, summary)
	}
}

func (c *RootRunnerCallbacks) OnInterrupted(info *adk.InterruptInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Extract checkpoint ID and interrupt ID from interrupt contexts
	var checkpointID string
	var interruptID string
	var hitlData *hitlInterruptData

	if len(info.InterruptContexts) > 0 {
		// Use the last context (most specific) for checkpoint/interrupt IDs
		rootCtx := info.InterruptContexts[0]
		checkpointID = rootCtx.ID

		// Find the most specific (deepest) context that has Info — this is the actual interrupt source.
		for i := len(info.InterruptContexts) - 1; i >= 0; i-- {
			ctx := info.InterruptContexts[i]
			if ctx.Info != nil {
				if i > 0 || interruptID == "" {
					interruptID = ctx.ID
				}
				if data := parseHitLInterruptData(ctx.Info); data != nil {
					hitlData = data
					interruptID = ctx.ID
					break
				}
			}
		}
		if interruptID == "" {
			interruptID = checkpointID
		}
	}

	// Mark all in-progress messages as stopped
	if c.cfg.ParentCtx == nil {
		c.cfg.ParentCtx = context.Background()
	}
	for _, t := range c.trackers {
		if !t.completed {
			c.markTrackerStopped(t)
		}
	}

	ctx := c.cfg.ParentCtx

	// If this is a permission interrupt (tool_name present but no HITL data),
	// update the HumanInPermission record with the actual Eino checkpoint/interrupt IDs.
	if hitlData == nil && len(info.InterruptContexts) > 0 {
		// Check if any interrupt context contains a permission_request type.
		for _, ictx := range info.InterruptContexts {
			if m, ok := ictx.Info.(map[string]any); ok {
				if typ, _ := m["type"].(string); typ == "permission_request" {
					// Use conversationID as the checkpoint key (same key passed to
					// RootRunner.Run via WithCheckPointID). InterruptContexts[0].ID
					// is an address-derived string, not the store key.
					checkpointKey := c.cfg.ConversationID.String()
					c.cfg.DB.WithContext(ctx).Table("human_in_permissions").
						Where("conversation_id = ? AND checkpoint_id = '' AND interrupt_id = '' AND status = 'pending'",
							c.cfg.ConversationID).
						Updates(map[string]interface{}{
							"checkpoint_id":          checkpointKey,
							"interrupt_id":           interruptID,
							"source_conversation_id": c.cfg.ConversationID.String(),
						})
					break
				}
			}
		}
	}

	// If this is an ask_user_question interrupt, create a HITL record
	if hitlData != nil {
		// Convert to []any to ensure correct JSON serialization by GORM/pgx.
		choicesAny := make([]any, len(hitlData.Choices))
		for i, c := range hitlData.Choices {
			choicesAny[i] = c
		}
		hitl := model.HumanInTheLoop{
			ConversationID:       c.cfg.ConversationID,
			SourceConversationID: &c.cfg.ConversationID,
			CheckpointID:         c.cfg.ConversationID.String(),
			InterruptID:    interruptID,
			Question:       hitlData.Question,
			Choices:        model.JSONMap{"choices": choicesAny},
			AnswerType:     hitlData.AnswerType,
			Status:         "pending",
		}
		if err := c.cfg.DB.WithContext(ctx).Create(&hitl).Error; err != nil {
			c.cfg.Log.Error("failed to persist HITL record", zap.Error(err))
		} else {
			// Push human_in_the_loop.created Update
			seq, err := c.cfg.NextSeq(ctx, c.cfg.UserID)
			if err != nil {
				c.cfg.Log.Error("seq assignment failed for HITL", zap.Error(err))
			} else {
				payload := model.JSONMap{
					"conversation_id": c.cfg.ConversationID.String(),
					"id":              hitl.ID.String(),
					"checkpoint_id":   hitl.CheckpointID,
					"interrupt_id":    hitl.InterruptID,
					"question":        hitlData.Question,
					"choices":         choicesAny,
					"answer_type":     hitlData.AnswerType,
					"seq":             seq,
				}
				update := model.UserUpdate{
					UserID:  c.cfg.UserID,
					Seq:     seq,
					Type:    "human_in_the_loop.created",
					Payload: payload,
				}
				if dbErr := c.cfg.DB.WithContext(ctx).Create(&update).Error; dbErr != nil {
					c.cfg.Log.Error("failed to persist HITL created update", zap.Error(dbErr))
				} else {
					c.cfg.PushUpdate(c.cfg.UserID, update)
				}
			}
		}
	}

	seq, err := c.cfg.NextSeq(ctx, c.cfg.UserID)
	if err != nil {
		c.cfg.Log.Error("seq assignment failed", zap.Error(err))
		return
	}

	payload := model.JSONMap{
		"conversation_id": c.cfg.ConversationID.String(),
		"error":           "Agent interrupted",
		"checkpoint_id":   checkpointID,
		"seq":             seq,
	}
	for _, t := range c.trackers {
		if t.role == "assistant" {
			payload["message_id"] = t.msgID.String()
			break
		}
	}

	update := model.UserUpdate{
		UserID:  c.cfg.UserID,
		Seq:     seq,
		Type:    "message.error",
		Payload: payload,
	}
	if err := c.cfg.DB.WithContext(ctx).Create(&update).Error; err != nil {
		c.cfg.Log.Error("failed to persist interrupted update", zap.Error(err))
		return
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)
	c.cfg.Log.Info("agent interrupted",
		zap.String("checkpoint", checkpointID),
		zap.String("conv", c.cfg.ConversationID.String()),
	)
}

// hitlInterruptData holds parsed data from an ask_user_question interrupt.
type hitlInterruptData struct {
	Type       string
	Question   string
	Choices    []map[string]any // each map has "title" and optionally "desc"
	AnswerType string
}

// parseHitLInterruptData extracts HITL data from interrupt info if it's an ask_user_question type.
func parseHitLInterruptData(info any) *hitlInterruptData {
	m, ok := info.(map[string]any)
	if !ok {
		return nil
	}
	typ, _ := m["type"].(string)
	if typ != "ask_user_question" {
		return nil
	}
	data := &hitlInterruptData{Type: typ}
	if q, ok := m["question"].(string); ok {
		data.Question = q
	}
	if at, ok := m["answer_type"].(string); ok {
		data.AnswerType = at
	}
	if choices, ok := m["choices"].([]any); ok {
		for _, c := range choices {
			if cm, ok := c.(map[string]any); ok {
				if title, ok := cm["title"].(string); ok {
					data.Choices = append(data.Choices, cm)
					_ = title
				}
			}
		}
	}
	if choices, ok := m["choices"].([]tools.HitlChoice); ok {
		for _, cm := range choices {
			data.Choices = append(data.Choices, map[string]any{
				"title": cm.Title,
				"desc":  cm.Desc,
			})
		}
	}
	return data
}

// ---- adk.CheckPointStore implementation ----

func (c *RootRunnerCallbacks) Get(ctx context.Context, checkpointID string) ([]byte, bool, error) {
	var cp model.Checkpoint
	err := c.cfg.DB.WithContext(ctx).
		Where("conversation_id = ? AND node_key = ?", c.cfg.ConversationID, checkpointID).
		First(&cp).Error
	if err == gorm.ErrRecordNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("checkpoint get failed: %w", err)
	}
	return cp.State, true, nil
}

func (c *RootRunnerCallbacks) Set(ctx context.Context, checkpointID string, data []byte) error {
	var cp model.Checkpoint
	err := c.cfg.DB.WithContext(ctx).
		Where("conversation_id = ? AND node_key = ?", c.cfg.ConversationID, checkpointID).
		First(&cp).Error

	cp.ConversationID = c.cfg.ConversationID
	cp.NodeKey = checkpointID
	cp.State = data

	if err == gorm.ErrRecordNotFound {
		cp.ID = uuid.New()
		err := c.cfg.DB.WithContext(ctx).Create(&cp).Error
		return err
	}
	if err != nil {
		return fmt.Errorf("checkpoint set failed: %w", err)
	}
	return c.cfg.DB.WithContext(ctx).Save(&cp).Error
}

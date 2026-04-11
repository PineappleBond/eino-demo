package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
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
}

// RootRunnerCallbacks implements RootRunnerCallback. It bridges Eino callbacks
// to WebSocket Update events and persists messages/checkpoints to PostgreSQL in real time.
type RootRunnerCallbacks struct {
	cfg      RunCallbackConfig
	mu       sync.Mutex             // guards trackers map and DB writes
	trackers map[string]*msgTracker // keyed by addr string (tool messages) or "assistant"
}

// NewRootRunnerCallbacks creates a callback handler for one agent run.
func NewRootRunnerCallbacks(cfg RunCallbackConfig) *RootRunnerCallbacks {
	return &RootRunnerCallbacks{
		cfg:      cfg,
		trackers: make(map[string]*msgTracker),
	}
}

// markTrackerStopped patches an in-progress message with finish_reason="stopped".
func (c *RootRunnerCallbacks) markTrackerStopped(t *msgTracker) error {
	if t.completed || t.msgID == uuid.Nil {
		return nil
	}
	t.completed = true
	finishReason := "stopped"
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
func (c *RootRunnerCallbacks) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cfg.ParentCtx == nil {
		c.cfg.ParentCtx = context.Background()
	}

	// Mark all in-progress messages as stopped
	for _, t := range c.trackers {
		if !t.completed {
			c.markTrackerStopped(t)
		}
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
	t, ok := c.trackers[addrStr]
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
	c.trackers[addrStr] = t
	return t
}

// getOrCreateAssistantTracker returns or creates a tracker for this addr.
// If the addr's tracker is already completed, a new one is created (for a new response).
func (c *RootRunnerCallbacks) getOrCreateAssistantTracker(addrStr string) *msgTracker {
	t, ok := c.trackers[addrStr]
	if ok {
		return t
	}

	t = &msgTracker{
		addr:     addrStr,
		role:     "assistant",
		senderID: "agent:root",
	}
	c.trackers[addrStr] = t
	return t
}

// findIncompleteAssistant returns any incomplete assistant tracker, regardless of addr.
// This prevents creating multiple assistant messages when addr changes between callbacks.
func (c *RootRunnerCallbacks) findIncompleteAssistant() *msgTracker {
	for _, t := range c.trackers {
		if t.role == "assistant" && t.msgID == uuid.Nil {
			return t
		}
	}
	return nil
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

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a tool message immediately
	t := c.getOrCreateTracker(addrStr, "tool", info.Name)
	if t.msgID == uuid.Nil {
		t.senderID = info.Name
		if err := c.insertMessage(ctx, t, "", ""); err != nil {
			c.cfg.Log.Error("failed to insert tool message", zap.Error(err))
			return
		}
	}
}

func (c *RootRunnerCallbacks) OnOutputToolCalling(ctx context.Context, info *callbacks.RunInfo, addr compose.Address, output callbacks.CallbackOutput) {
	addrStr := AddrString(addr)
	c.mu.Lock()
	defer c.mu.Unlock()

	t, ok := c.trackers[addrStr]
	if !ok {
		c.cfg.Log.Warn("no tracker for tool output", zap.String("addr", addrStr))
		return
	}

	// Extract tool result from output
	var resultStr string
	if output != nil {
		resultStr = fmt.Sprintf("%v", output)
	}

	// Complete the tool message with final result
	t.completed = true
	finishReason := "stop"
	c.cfg.DB.WithContext(ctx).Model(&model.Message{}).
		Where("id = ?", t.msgID).
		Updates(map[string]interface{}{
			"content":        resultStr,
			"reason_content": "",
			"finish_reason":  finishReason,
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
	// Reuse any incomplete assistant tracker first
	if existing := c.findIncompleteAssistant(); existing != nil {
		return existing
	}

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

	c.mu.Lock()
	defer c.mu.Unlock()

	t := c.getOrCreateAssistantTracker(addrStr)
	if t.msgID == uuid.Nil {
		// Should not normally happen, but handle it
		if err := c.insertMessage(ctx, t, outputContent, reasoningContent); err != nil {
			c.cfg.Log.Error("failed to insert assistant message on completed", zap.Error(err))
			return
		}
	}

	c.completeMessage(ctx, t, outputContent, reasoningContent, usage)
}

// ---- RootRunnerCallback lifecycle methods ----

func (c *RootRunnerCallbacks) OnError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cfg.ParentCtx == nil {
		c.cfg.ParentCtx = context.Background()
	}

	seq, seqErr := c.cfg.NextSeq(c.cfg.ParentCtx, c.cfg.UserID)
	if seqErr != nil {
		c.cfg.Log.Error("seq assignment failed", zap.Error(seqErr))
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
		return
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)
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

	// Bump conversation's updated_at
	if c.cfg.ParentCtx == nil {
		c.cfg.ParentCtx = context.Background()
	}
	c.cfg.DB.WithContext(c.cfg.ParentCtx).Model(&model.Conversation{}).
		Where("id = ?", c.cfg.ConversationID).
		Update("updated_at", NowFunc())
}

func (c *RootRunnerCallbacks) OnInterrupted(info *adk.InterruptInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Extract checkpoint ID from interrupt contexts
	var checkpointID string
	if len(info.InterruptContexts) > 0 {
		checkpointID = info.InterruptContexts[0].ID
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

	seq, err := c.cfg.NextSeq(c.cfg.ParentCtx, c.cfg.UserID)
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
	if err := c.cfg.DB.WithContext(c.cfg.ParentCtx).Create(&update).Error; err != nil {
		c.cfg.Log.Error("failed to persist interrupted update", zap.Error(err))
		return
	}
	c.cfg.PushUpdate(c.cfg.UserID, update)
	c.cfg.Log.Info("agent interrupted",
		zap.String("checkpoint", checkpointID),
		zap.String("conv", c.cfg.ConversationID.String()),
	)
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
		return c.cfg.DB.WithContext(ctx).Create(&cp).Error
	}
	if err != nil {
		return fmt.Errorf("checkpoint set failed: %w", err)
	}
	return c.cfg.DB.WithContext(ctx).Save(&cp).Error
}

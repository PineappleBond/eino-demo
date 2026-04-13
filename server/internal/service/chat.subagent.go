package service

import (
	"context"
	"fmt"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RunSubAgent spawns a sub-agent for a sub-conversation.
// This is the SpawnSubAgentFunc callback wired by the DI module.
func (s *ChatService) RunSubAgent(
	parentCtx context.Context,
	parentConvID, childConvID uuid.UUID,
	prompt, logPath string,
) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("RunSubAgent panic recovered", zap.Any("recover", r))
		}
	}()

	// 1. Get child conversation
	var childConv model.Conversation
	if err := s.db.WithContext(parentCtx).Where("id = ?", childConvID).First(&childConv).Error; err != nil {
		s.log.Error("RunSubAgent: sub-conversation not found", zap.Error(err))
		s.writeSubAgentResultToParent(parentCtx, parentConvID, childConvID, false, "子对话不存在", nil)
		return
	}

	userID := childConv.UserID

	// 2. Create JSONL logger
	logger, err := runner.NewJSONLLogger(logPath)
	if err != nil {
		s.log.Error("RunSubAgent: failed to create JSONL logger", zap.Error(err))
		s.writeSubAgentResultToParent(parentCtx, parentConvID, childConvID, false, "failed to create log file", nil)
		return
	}

	// 3. Create the initial user message in DB so runAgent can load it.
	// Without this, the sub-agent sees an empty conversation (no user prompt).
	seq, err := s.wsManager.NextSeq(parentCtx, userID)
	if err != nil {
		s.log.Error("RunSubAgent: seq assignment failed", zap.Error(err))
		seq = 1
	}

	userMsg := model.Message{
		ConversationID: childConvID,
		Seq:            seq,
		SenderRole:     "user",
		SenderID:       userID.String(),
		Content:        prompt,
		Metadata:       model.JSONMap{"source": "sub-agent"},
	}
	if err := s.db.WithContext(parentCtx).Create(&userMsg).Error; err != nil {
		s.log.Error("RunSubAgent: failed to create initial user message", zap.Error(err))
		s.writeSubAgentResultToParent(parentCtx, parentConvID, childConvID, false, "failed to create initial message", nil)
		return
	}

	// Log initial user message to JSONL
	_ = logger.Log(runner.JSONLLogEntry{
		Type:    "message.new",
		Role:    "user",
		Content: prompt,
	})

	// 4. Register a completion callback that writes results to the parent conversation
	s.mu.Lock()
	s.runCompleteCallbacks[childConvID] = func(success bool, summary string) {
		s.writeSubAgentResultToParent(parentCtx, parentConvID, childConvID, success, summary, logger)
	}
	s.mu.Unlock()

	// 5. Reuse runAgent with the child conversation ID.
	// Use a clean context (context.Background) so the sub-agent gets its own
	// Eino callback chain. If we inherited parentCtx, the Eino callback manager
	// from the parent runner would be copied into the sub-agent's context,
	// causing sub-agent LLM events to trigger the parent's handler and write
	// messages to the parent conversation.
	subCtx := context.Background()
	s.runAgent(subCtx, userID, childConvID, prompt, s.wsManager.NextSeq, func(userID uuid.UUID, update model.UserUpdate) {
		// For sub-agent runs, we push updates via wsManager directly.
		// The wsManager expects the converted update format.
		wsUpdate := s.convertFn(update)
		s.wsManager.PushToUserConnections(userID, wsUpdate)
	}, parentConvID)
}

// writeSubAgentResultToParent writes a system message to the parent conversation
// notifying about sub-agent completion or failure.
func (s *ChatService) writeSubAgentResultToParent(
	ctx context.Context,
	parentConvID, childConvID uuid.UUID,
	success bool,
	summary string,
	logger *runner.JSONLLogger,
) {
	// Close logger if provided
	if logger != nil {
		_ = logger.Log(runner.JSONLLogEntry{
			Type:    "message.done",
			Role:    "assistant",
			Content: summary,
			Status:  map[bool]string{true: "completed", false: "failed"}[success],
		})
		_ = logger.Close()
	}

	// Get parent conversation to find user_id
	var parentConv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ?", parentConvID).First(&parentConv).Error; err != nil {
		s.log.Error("writeSubAgentResultToParent: parent conversation not found", zap.Error(err))
		return
	}

	// 1. Create system message in parent conversation
	statusText := map[bool]string{true: "已完成", false: "已失败"}[success]
	content := fmt.Sprintf("子对话 %s %s。摘要：%s", childConvID.String()[:8], statusText, summary)

	msg := model.Message{
		ConversationID: parentConvID,
		SenderRole:     "system",
		SenderID:       "sub-agent",
		Content:        content,
		Metadata: model.JSONMap{
			"child_conversation_id": childConvID.String(),
			"success":               success,
		},
	}
	if err := s.db.WithContext(ctx).Create(&msg).Error; err != nil {
		s.log.Error("writeSubAgentResultToParent: failed to create message", zap.Error(err))
		return
	}

	// 2. Push update to parent conversation via WS
	seq, err := s.wsManager.NextSeq(ctx, parentConv.UserID)
	if err != nil {
		s.log.Error("writeSubAgentResultToParent: seq assignment failed", zap.Error(err))
		return
	}

	payload := model.JSONMap{
		"conversation_id":       parentConvID.String(),
		"message_id":            msg.ID.String(),
		"seq":                   seq,
		"role":                  "system",
		"sender_id":             "sub-agent",
		"content":               content,
		"child_conversation_id": childConvID.String(),
		"success":               success,
	}
	update := model.UserUpdate{
		UserID:  parentConv.UserID,
		Seq:     seq,
		Type:    "sub_agent.completed",
		Payload: payload,
	}
	if err := s.db.WithContext(ctx).Create(&update).Error; err != nil {
		s.log.Error("writeSubAgentResultToParent: failed to create update", zap.Error(err))
		return
	}

	wsUpdate := s.convertFn(update)
	s.wsManager.PushToUserConnections(parentConv.UserID, wsUpdate)

	s.log.Info("writeSubAgentResultToParent: posted to parent",
		zap.String("parent_conv", parentConvID.String()),
		zap.String("child_conv", childConvID.String()),
		zap.Bool("success", success),
	)
}

// sendMessageWithRole is the shared implementation for creating messages with any role.
func (s *ChatService) sendMessageWithRole(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	content, senderRole, senderID string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*SendMessageResponse, error) {
	// 1. Verify conversation ownership before allocating seq
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Allocate seq after ownership confirmed
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var messageID uuid.UUID
	var isFirstMessage bool

	// 3. Create message + user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Check if this is truly the first message inside the transaction to avoid
		// race conditions where concurrent requests both see messageCount == 0.
		var count int64
		if err := tx.Model(&model.Message{}).Where("conversation_id = ?", conversationID).Count(&count).Error; err != nil {
			s.log.Error("sendMessageWithRole: failed to count messages in transaction", zap.Error(err))
		}
		isFirstMessage = count == 0

		msg := model.Message{
			ConversationID: conversationID,
			SenderRole:     senderRole,
			SenderID:       senderID,
			Content:        content,
			Metadata:       model.JSONMap{"source": senderRole},
			Seq:            seq,
		}
		if err := tx.Create(&msg).Error; err != nil {
			return err
		}
		messageID = msg.ID

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "message.new",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"message_id":      messageID.String(),
				"role":            senderRole,
				"sender_id":       senderID,
				"addr":            "",
				"content":         content,
				"seq":             seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// 4. Push to WebSocket connections
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "message.new",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"role":            senderRole,
			"sender_id":       senderID,
			"addr":            "",
			"content":         content,
			"seq":             seq,
		},
	})

	// --- Run the AI agent asynchronously ---
	// If the conversation already has an active agent running, enqueue the message
	// instead of starting a new run. The agent's contextInjectionMiddleware will
	// drain pending messages at the next model call boundary.
	// Use IsRunning (not IsActive) to avoid enqueuing to a session that is
	// stopping but not yet cleaned up.
	if s.runSessionMgr.IsRunning(conversationID) {
		s.messageQueue.Enqueue(conversationID, content)
		s.log.Info("sendMessageWithRole: enqueued message for active agent",
			zap.String("conv", conversationID.String()),
		)
	} else {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					s.log.Error("runAgent panic recovered", zap.Any("recover", r))
				}
			}()
			s.runAgent(context.WithoutCancel(ctx), userID, conversationID, content, nextSeq, pushUpdate)
		}()
	}

	// --- Generate title for first message asynchronously ---
	if isFirstMessage {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					s.log.Error("runTitleAgent panic recovered", zap.Any("recover", r))
				}
			}()
			s.runTitleAgent(context.WithoutCancel(ctx), userID, conversationID, content, nextSeq, pushUpdate)
		}()
	}

	return &SendMessageResponse{
		ConversationID: conversationID,
		MessageID:      messageID,
		Seq:            seq,
	}, nil
}

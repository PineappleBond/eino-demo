package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/tools"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/templates"
	svcutils "github.com/PineappleBond/eino-demo-dev/server/internal/utils"
)

// agentRunTimeout is the maximum duration for a single agent execution event loop.
const agentRunTimeout = 10 * time.Minute

// ChatService handles sending messages and streaming AI responses.
type ChatService struct {
	db             *gorm.DB
	log            *zap.Logger
	modelProvider  *eino.ModelProvider
	toolRegistry   *tools.ToolRegistry
	runSessionMgr  *runner.RunSessionManager
	messageQueue   *runner.MessageQueue
	compressionSvc *CompressionService
}

// NewChatService creates a ChatService.
func NewChatService(
	db *gorm.DB,
	log *zap.Logger,
	modelProvider *eino.ModelProvider,
	toolRegistry *tools.ToolRegistry,
	runSessionMgr *runner.RunSessionManager,
	messageQueue *runner.MessageQueue,
	compressionSvc *CompressionService,
) *ChatService {
	return &ChatService{
		db:             db,
		log:            log,
		modelProvider:  modelProvider,
		toolRegistry:   toolRegistry,
		runSessionMgr:  runSessionMgr,
		messageQueue:   messageQueue,
		compressionSvc: compressionSvc,
	}
}

// SendMessageRequest holds the fields for sending a message.
type SendMessageRequest struct {
	Content string `json:"content"`
}

// SendMessageResponse is the synchronous return from sending a message.
type SendMessageResponse struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
	Seq            int64     `json:"seq"`
}

// NextSeqFunc is a function that assigns the next sequence number for a user.
type NextSeqFunc func(ctx context.Context, userID uuid.UUID) (int64, error)

// PushUpdateFunc pushes an update to a user's WebSocket connections.
type PushUpdateFunc func(userID uuid.UUID, update model.UserUpdate)

// CompleteSendMessage handles the full send flow: service logic + seq + update + push + AI agent run.
// Wraps user message creation + user_update in a single transaction for atomicity.
func (s *ChatService) CompleteSendMessage(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req SendMessageRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*SendMessageResponse, error) {
	// 1. Verify conversation ownership before allocating seq
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// 2. Check if this is the first message
	var messageCount int64
	if err := s.db.Model(&model.Message{}).Where("conversation_id = ?", conversationID).Count(&messageCount).Error; err != nil {
		s.log.Error("CompleteSendMessage: failed to count messages", zap.Error(err))
	}
	isFirstMessage := messageCount == 0

	// 3. Allocate seq after ownership confirmed
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var messageID uuid.UUID

	// 3. Create user message + user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		msg := model.Message{
			ConversationID: conversationID,
			SenderRole:     "user",
			SenderID:       userID.String(),
			Content:        req.Content,
			Metadata:       model.JSONMap{},
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
				"role":            "user",
				"content":         req.Content,
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
			"role":            "user",
			"content":         req.Content,
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
		s.messageQueue.Enqueue(conversationID, req.Content)
		s.log.Info("CompleteSendMessage: enqueued message for active agent",
			zap.String("conv", conversationID.String()),
		)
	} else {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					s.log.Error("runAgent panic recovered", zap.Any("recover", r))
				}
			}()
			s.runAgent(context.WithoutCancel(ctx), userID, conversationID, req.Content, nextSeq, pushUpdate)
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
			s.runTitleAgent(context.WithoutCancel(ctx), userID, conversationID, req.Content, nextSeq, pushUpdate)
		}()
	}

	return &SendMessageResponse{
		ConversationID: conversationID,
		MessageID:      messageID,
		Seq:            seq,
	}, nil
}

// runAgent starts the AI agent in a background goroutine and streams results via WS.
func (s *ChatService) runAgent(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	userContent string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) {
	// 1. Resolve template config: conversation → project → template
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		s.log.Error("runAgent: conversation not found", zap.Error(err))
		return
	}

	var project model.Project
	if err := s.db.Where("id = ?", conv.ProjectID).First(&project).Error; err != nil {
		s.log.Error("runAgent: project not found", zap.Error(err))
		return
	}

	tpl, ok := templates.Get(project.TemplateID)
	if !ok {
		s.log.Warn("runAgent: template not found, using fallback", zap.String("template_id", project.TemplateID))
		tpl, ok = templates.Get("01")
		if !ok {
			s.log.Error("runAgent: no fallback template available")
			return
		}
	}

	// Extract system prompt from first agent in template
	systemPrompt := ""
	if len(tpl.Agents) > 0 {
		systemPrompt = tpl.Agents[0].SystemPrompt
	}

	// Get model tier from project config (default: sonnet)
	modelTier := "sonnet"
	if tier, ok := project.Config["model_tier"].(string); ok && tier != "" {
		modelTier = tier
	}

	// 2. Create cancellable context
	runCtx, cancel := context.WithCancel(ctx)

	// 3. Build callback handler (creates messages on first chunk)
	callbackCfg := runner.RunCallbackConfig{
		UserID:         userID,
		ConversationID: conversationID,
		DB:             s.db,
		Log:            s.log,
		NextSeq:        nextSeq,
		PushUpdate:     pushUpdate,
		ParentCtx:      ctx, // parent context, not cancelled — used for lifecycle methods
	}
	callbacks := runner.NewRootRunnerCallbacks(callbackCfg)

	// 4. Register session (stop func marks in-progress messages as stopped in DB)
	// TryStart is atomic: if another runAgent already registered, this returns false
	// and we exit gracefully to prevent concurrent agent runs.
	if !s.runSessionMgr.TryStart(conversationID, cancel, callbacks.Stop) {
		s.log.Info("runAgent: another agent run already registered, skipping",
			zap.String("conv", conversationID.String()),
		)
		cancel()
		return
	}

	// 5. Token counter — created once, reused for both pre-execution check and runtime overflow detection.
	// Limit: 80K tokens — beyond this, compaction is recommended.
	const maxPromptTokens = 80000
	var tokenCounter *svcutils.TokenCounter
	if tc, err := svcutils.NewTokenCounter(modelTier); err != nil {
		s.log.Warn("runAgent: failed to create token counter, disabling token checks", zap.Error(err))
	} else {
		tokenCounter = tc
	}

	runCfg := runner.RootRunnerConfig{
		ModelProvider:    s.modelProvider,
		ModelTier:        modelTier,
		SystemPrompt:     systemPrompt,
		Tools:            s.toolRegistry.GetBaseTools(),
		MaxIteration:     100,
		ConversationID:   conversationID,
		MessageQueue:     s.messageQueue,
		ReductionEnabled: true,
		TokenCheck: &runner.TokenCheckConfig{
			Counter:   tokenCounter,
			MaxTokens: maxPromptTokens,
			OnOverflow: func(tokenCount int) {
				s.log.Warn("runAgent: token overflow detected during agent run",
					zap.Int("tokens", tokenCount),
					zap.Int("limit", maxPromptTokens),
					zap.String("conv", conversationID.String()),
				)
				// Do NOT call callbacks.Stop() here — the event loop handles
				// stopping after catching TokenOverflowError. Calling Stop()
				// here would emit a duplicate message.stop update.
			},
		},
		SummarizationCallback: func(cbCtx context.Context, compressedMsgCount int) {
			s.log.Info("runAgent: summarization callback triggered",
				zap.Int("compressed_count", compressedMsgCount),
				zap.String("conv", conversationID.String()),
			)
			// The summarization middleware already compressed state.Messages in memory.
			// We need to sync the compression to DB: insert summary message, update min_seq.
			// This is done asynchronously to not block the agent.
			go func() {
				if err := s.compressionSvc.SyncSummarizationToDB(
					context.WithoutCancel(cbCtx),
					userID, conversationID, compressedMsgCount,
					nextSeq, pushUpdate,
				); err != nil {
					s.log.Error("runAgent: failed to sync summarization to DB", zap.Error(err))
				}
			}()
		},
	}

	rootRunner, err := runner.NewRootRunner(runCtx, runCfg, callbacks)
	if err != nil {
		s.log.Error("runAgent: failed to create runner", zap.Error(err))
		cancel()
		s.runSessionMgr.Cleanup(conversationID)
		return
	}

	// 5. Create handler
	handler := runner.NewRootRunnerHandler(callbacks, nil)

	// 7. Check for existing checkpoint to resume
	checkpointID, hasCheckpoint := s.runSessionMgr.GetCheckpointID(conversationID)

	var iter *adk.AsyncIterator[*adk.AgentEvent]
	// Load conversation history as context: user, assistant, and system messages
	// Only load messages from min_seq onwards (earlier messages have been compressed)
	var historyMessages []model.Message
	if err := s.db.
		Select("sender_role", "content").
		Where("conversation_id = ? AND seq >= ? AND sender_role IN ?",
			conversationID, conv.MinSeq, []string{"user", "assistant", "system"}).
		Order("seq ASC").
		Find(&historyMessages).Error; err != nil {
		s.log.Error("runAgent: failed to load conversation history", zap.Error(err))
	}

	// Build message list from history + current user message
	var messages []*schema.Message
	for _, msg := range historyMessages {
		switch msg.SenderRole {
		case "user":
			messages = append(messages, schema.UserMessage(msg.Content))
		case "assistant":
			messages = append(messages, schema.AssistantMessage(msg.Content, nil))
		case "system":
			messages = append(messages, schema.SystemMessage(msg.Content))
		}
	}

	// Check prompt token count before running agent to prevent context overflow.
	if len(messages) > 0 && tokenCounter != nil {
		roles := make([]string, 0, len(messages))
		contents := make([]string, 0, len(messages))
		for _, m := range messages {
			roles = append(roles, string(m.Role))
			contents = append(contents, m.Content)
		}
		promptTokens := tokenCounter.CountMessages(roles, contents)
		s.log.Info("runAgent: prompt token estimate",
			zap.Int("tokens", promptTokens),
			zap.Int("limit", maxPromptTokens),
			zap.Int("messages", len(messages)),
		)

		// Update conversation token_prompt field and push notification
		seq, seqErr := nextSeq(ctx, userID)
		if seqErr != nil {
			s.log.Error("runAgent: seq assignment failed for token update", zap.Error(seqErr))
		} else {
			if err := s.db.WithContext(ctx).Model(&model.Conversation{}).
				Where("id = ?", conversationID).
				Updates(map[string]interface{}{
					"token_prompt": promptTokens,
				}).Error; err != nil {
				s.log.Error("runAgent: failed to update conversation token_prompt", zap.Error(err))
			}

			update := model.UserUpdate{
				UserID: userID,
				Seq:    seq,
				Type:   "conversation.updated",
				Payload: model.JSONMap{
					"conversation_id": conversationID.String(),
					"token_prompt":    promptTokens,
					"seq":             seq,
				},
			}
			if dbErr := s.db.WithContext(ctx).Create(&update).Error; dbErr != nil {
				s.log.Error("failed to persist token update", zap.Error(dbErr))
			}
			pushUpdate(userID, update)
		}

		if promptTokens > maxPromptTokens {
			s.log.Warn("runAgent: prompt exceeds token limit, suggesting compaction",
				zap.Int("tokens", promptTokens),
				zap.Int("limit", maxPromptTokens),
			)
			// Push error update instead of running agent
			errSeq, err := nextSeq(ctx, userID)
			if err != nil {
				s.log.Error("runAgent: seq assignment failed", zap.Error(err))
			} else {
				errUpdate := model.UserUpdate{
					UserID: userID,
					Seq:    errSeq,
					Type:   "message.error",
					Payload: model.JSONMap{
						"conversation_id": conversationID.String(),
						"error":           fmt.Sprintf("对话历史过长（约 %d tokens），超出限制 %d tokens。请先压缩后再发送。", promptTokens, maxPromptTokens),
					},
				}
				if dbErr := s.db.WithContext(ctx).Create(&errUpdate).Error; dbErr != nil {
					s.log.Error("failed to persist token-limit error update", zap.Error(dbErr))
				}
				pushUpdate(userID, errUpdate)
			}
			cancel()
			s.runSessionMgr.Cleanup(conversationID)
			return
		}
	}

	if hasCheckpoint {
		iter, err = rootRunner.Resume(runCtx, checkpointID, handler)
		if err != nil {
			s.log.Error("runAgent: resume failed, falling back to fresh run", zap.Error(err), zap.String("checkpoint", checkpointID))
			iter = rootRunner.Run(runCtx, messages, "", handler)
		}
	} else {
		iter = rootRunner.Run(runCtx, messages, "", handler)
	}

	// 8. Consume events in goroutine with timeout protection and panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("runAgent event loop panic recovered", zap.Any("recover", r))
				callbacks.Stop()
				s.runSessionMgr.Cleanup(conversationID)
				s.messageQueue.Cleanup(conversationID)
				s.runSessionMgr.Done(conversationID)
				cancel()
			}
		}()
		defer func() {
			// Before cleaning up, check for pending messages that were enqueued
			// after the last Drain. This happens when the agent finishes
			// naturally before consuming all queued messages.
			if s.messageQueue.HasPending(conversationID) {
				s.log.Info("runAgent: pending messages remain after agent end, spawning new run",
					zap.String("conv", conversationID.String()),
				)
				s.runSessionMgr.Cleanup(conversationID)
				s.messageQueue.Cleanup(conversationID)
				s.runSessionMgr.Done(conversationID)
				cancel()
				go func() {
					defer func() {
						if r := recover(); r != nil {
							s.log.Error("runAgent (defer spawn) panic recovered", zap.Any("recover", r))
						}
					}()
					s.runAgent(context.WithoutCancel(ctx), userID, conversationID, userContent, nextSeq, pushUpdate)
				}()
				return
			}
			s.runSessionMgr.Cleanup(conversationID)
			s.messageQueue.Cleanup(conversationID)
			s.runSessionMgr.Done(conversationID)
		}()
		defer cancel()

		timer := time.NewTimer(agentRunTimeout)
		defer timer.Stop()

		done := make(chan struct{})
		go func() {
			for {
				event, ok := iter.Next()
				if !ok {
					callbacks.OnEnd()
					break
				}
				if event == nil {
					continue
				}
				if event.Err != nil {
					// Check for our TokenOverflowError first (carries drained messages).
					var overflowErr *runner.TokenOverflowError
					if errors.As(event.Err, &overflowErr) {
						s.log.Warn("runAgent: token overflow detected, compressing and restarting",
							zap.Int("tokens", overflowErr.TokenCount),
							zap.Int("limit", overflowErr.MaxTokens),
							zap.String("conv", conversationID.String()),
							zap.Int("drained_messages", len(overflowErr.DrainedMessages)),
						)
						callbacks.Stop()

						// Re-enqueue drained messages before compression, so they survive
						// the fresh run after the event loop exits.
						for _, content := range overflowErr.DrainedMessages {
							s.messageQueue.Enqueue(conversationID, content)
						}

						// Compress in-place. After compression, the DB has updated min_seq
						// and a summary message. The fresh runAgent will load from DB
						// with seq >= min_seq.
						if err := s.compressionSvc.CompressAndResume(
							context.WithoutCancel(ctx), userID, conversationID, nextSeq, pushUpdate,
						); err != nil {
							s.log.Error("runAgent: compress failed before restart", zap.Error(err))
							// Fall through to error push below
						} else {
							s.log.Info("runAgent: compression complete, restarting agent",
								zap.String("conv", conversationID.String()),
							)
							// Restart agent with same params. The event loop defer will
							// cleanup the old session; the new runAgent will create a fresh one.
							// Clear stale checkpoint to force a fresh Run (not Resume).
							s.runSessionMgr.ClearCheckpointID(conversationID)
							go func() {
								defer func() {
									if r := recover(); r != nil {
										s.log.Error("runAgent (post-compress) panic recovered", zap.Any("recover", r))
									}
								}()
								s.runAgent(context.WithoutCancel(ctx), userID, conversationID, userContent, nextSeq, pushUpdate)
							}()
							return
						}
					} else if isContextOverflowError(event.Err) {
						s.log.Warn("runAgent: context overflow detected (API-level), stopping agent",
							zap.Error(event.Err),
							zap.String("conv", conversationID.String()),
						)
						callbacks.Stop()
					} else {
						callbacks.OnError(event.Err)
						break
					}
					// Push user-friendly error message (for both overflow types)
					errSeq, err := nextSeq(ctx, userID)
					if err != nil {
						s.log.Error("runAgent: seq assignment failed for overflow error", zap.Error(err))
					} else {
						errUpdate := model.UserUpdate{
							UserID: userID,
							Seq:    errSeq,
							Type:   "message.error",
							Payload: model.JSONMap{
								"conversation_id": conversationID.String(),
								"error":           "对话上下文超出限制，请先压缩对话历史后再继续。",
							},
						}
						if dbErr := s.db.WithContext(ctx).Create(&errUpdate).Error; dbErr != nil {
							s.log.Error("failed to persist overflow error update", zap.Error(dbErr))
						}
						pushUpdate(userID, errUpdate)
					}
					break
				}
				if event.Action != nil && event.Action.Interrupted != nil {
					// Store checkpoint ID for resume
					if len(event.Action.Interrupted.InterruptContexts) > 0 {
						cpID := event.Action.Interrupted.InterruptContexts[0].ID
						s.runSessionMgr.SetCheckpointID(conversationID, cpID)
					}
					callbacks.OnInterrupted(event.Action.Interrupted)
					break
				}
			}
			close(done)
		}()

		select {
		case <-done:
			// Normal completion
		case <-timer.C:
			s.log.Warn("runAgent: event loop timed out", zap.Duration("timeout", agentRunTimeout))
		}
	}()
}

// StopMessage marks the latest AI message as stopped and cancels the running agent.
func (s *ChatService) StopMessage(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// Verify conversation exists and belongs to user
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	// 1. Cancel the running agent
	s.runSessionMgr.Stop(conversationID)

	// 2. Assign seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Create user_update for the stop event
	update := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "message.stop",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
		},
	}
	if err := s.db.Create(&update).Error; err != nil {
		return err
	}

	pushUpdate(userID, update)

	return nil
}

// GetConversationMessagesRequest holds the query params for listing messages.
type GetConversationMessagesRequest struct {
	AfterSeq int64 `form:"after_seq"`
}

// GetConversationMessages returns messages for a conversation, optionally after a seq.
func (s *ChatService) GetConversationMessages(userID, conversationID uuid.UUID, req GetConversationMessagesRequest) ([]model.Message, error) {
	// Verify conversation exists and belongs to user
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	query := s.db.Where("conversation_id = ?", conversationID).Order("seq ASC")
	if req.AfterSeq > 0 {
		query = query.Where("seq > ?", req.AfterSeq)
	}

	var messages []model.Message
	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

// StartCompaction compresses the conversation in-place.
// If an agent is actively running, it stops it first.
func (s *ChatService) StartCompaction(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	// 2. Stop running agent if any
	if s.runSessionMgr.IsRunning(conversationID) {
		s.runSessionMgr.Stop(conversationID)
	}

	// 3. Compress in-place
	if _, err := s.compressionSvc.CompressConversation(ctx, userID, conversationID, nextSeq, pushUpdate); err != nil {
		return fmt.Errorf("failed to compress conversation: %w", err)
	}

	return nil
}

// isContextOverflowError checks if an error indicates context length exceeded.
func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, p := range []string{
		"context_length_exceeded",
		"prompt_too_long",
		"context_overflow",
		"maximum context length",
		"input is too long",
		"too many tokens",
		"token overflow", // TokenOverflowError from contextInjectionMiddleware
	} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

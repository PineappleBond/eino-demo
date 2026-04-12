package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
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
	// evalModelConfig holds haiku model config for the permission evaluator.
	evalModelConfig *openai.ChatModelConfig
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
	// Create haiku model config for permission evaluator (safety assessment)
	mc := modelProvider.GetModel("haiku")
	evalCfg := &openai.ChatModelConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
	}

	return &ChatService{
		db:             db,
		log:            log,
		modelProvider:  modelProvider,
		toolRegistry:   toolRegistry,
		runSessionMgr:  runSessionMgr,
		messageQueue:   messageQueue,
		compressionSvc: compressionSvc,
		evalModelConfig: evalCfg,
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
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	return s.sendMessageWithRole(ctx, userID, conversationID, req.Content, "user", userID.String(), nextSeq, pushUpdate)
}

// CompleteToolMessage creates a tool-role message and triggers the AI agent.
// Used by cron task execution and other system-triggered messages.
func (s *ChatService) CompleteToolMessage(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	content string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*SendMessageResponse, error) {
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	return s.sendMessageWithRole(ctx, userID, conversationID, content, "tool", "cron_scheduler", nextSeq, pushUpdate)
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
		// Another agent run is active — enqueue the message so it will be
		// consumed by the context injection middleware at the next model call
		// boundary, or by the deferred pending-check when the current run ends.
		s.messageQueue.Enqueue(conversationID, userContent)
		s.log.Info("runAgent: another agent run active, enqueued message",
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

	// Max iteration scales with model tier: haiku for simple tasks, opus for
	// complex reasoning chains.
	maxIteration := map[string]int{"haiku": 20, "sonnet": 50, "opus": 100}[modelTier]
	if maxIteration == 0 {
		maxIteration = 50 // default to sonnet
	}

	// Extract workspace info for evaluator and runner config
	workspaceDir := ""
	if wd, ok := project.Config["workspace_dir"].(string); ok && wd != "" {
		workspaceDir = wd
	}

	isGitRepo := false
	if workspaceDir != "" {
		if _, err := os.Stat(filepath.Join(workspaceDir, ".git")); err == nil {
			isGitRepo = true
		}
	}

	runCfg := runner.RootRunnerConfig{
		ModelProvider: s.modelProvider,
		ModelTier:     modelTier,
		SystemPrompt:  systemPrompt,
		Tools: func() []tool.BaseTool {
			s.toolRegistry.SetConversationID(conversationID)
			s.toolRegistry.SetWorkspaceDir(workspaceDir)
			tools := s.toolRegistry.GetBaseTools()
			// Append filesystem and HTTP tools (they implement NeedPermissioner).
			tools = append(tools, s.toolRegistry.GetPermissionTools()...)
			return tools
		}(),
		MaxIteration:     maxIteration,
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
		PermissionMW: func() *permission.Middleware {
			// Create evaluator here where workspace info is available.
			var evaluator permission.SafetyEvaluator
			evalModel, err := openai.NewChatModel(context.Background(), s.evalModelConfig)
			if err != nil {
				s.log.Warn("runAgent: failed to create permission evaluator model, using fallback", zap.Error(err))
			} else {
				evaluator = permission.NewLLMReviewer(evalModel, workspaceDir, isGitRepo)
			}

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
			}
			return permission.NewMiddleware(mwCfg)
		}(),
		WorkspaceDir: workspaceDir,
		IsGitRepo:    isGitRepo,
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

	// 6. Load conversation history — shared function handles tool_calling and tool messages.
	messages, err := loadConversationMessages(runCtx, s.db, conversationID, conv.MinSeq)
	if err != nil {
		s.log.Error("runAgent: failed to load conversation history", zap.Error(err))
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

	}

	if hasCheckpoint {
		resumeParams, hasResumeParams := s.runSessionMgr.GetResumeParams(conversationID)
		if hasResumeParams {
			s.log.Info("runAgent: resuming with params",
				zap.String("checkpoint", checkpointID),
				zap.Int("targets", len(resumeParams.Targets)),
			)
			// Clear resume params so they're only used once
			s.runSessionMgr.ClearResumeParams(conversationID)
			iter, err = rootRunner.ResumeWithParams(runCtx, checkpointID, resumeParams, handler)
			if err != nil {
				s.log.Error("runAgent: resumeWithParams failed, falling back to fresh run", zap.Error(err), zap.String("checkpoint", checkpointID))
				iter = rootRunner.Run(runCtx, messages, "", handler)
			}
		} else {
			iter, err = rootRunner.Resume(runCtx, checkpointID, handler)
			if err != nil {
				s.log.Error("runAgent: resume failed, falling back to fresh run", zap.Error(err), zap.String("checkpoint", checkpointID))
				iter = rootRunner.Run(runCtx, messages, "", handler)
			}
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
							// Restart agent with same params. Must Stop + Cleanup the
							// old session first — the old event loop is still running
							// (we are inside it) and its session is still in the map.
							// TryStart in the new runAgent would fail if we don't
							// clean up first.
							go func() {
								defer func() {
									if r := recover(); r != nil {
										s.log.Error("runAgent (post-compress) panic recovered", zap.Any("recover", r))
									}
								}()
								// Stop the old run and wait for its event loop to drain.
								// This is safe to call from within the old event loop
								// goroutine — it cancels the context and waits up to 2s.
								s.runSessionMgr.Stop(conversationID)
								s.runSessionMgr.Cleanup(conversationID)
								s.runSessionMgr.ClearCheckpointID(conversationID)
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
								"error":           ErrContextLimitExceeded.Error(),
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
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found: %w", ErrConversationNotFound)
	}

	// 1. Check if agent is running before stopping
	if !s.runSessionMgr.IsRunning(conversationID) {
		return nil
	}

	// 2. Cancel the running agent
	s.runSessionMgr.Stop(conversationID)

	// 3. Assign seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// 4. Create user_update for the stop event
	update := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "message.stop",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
		},
	}
	if err := s.db.WithContext(ctx).Create(&update).Error; err != nil {
		return err
	}

	pushUpdate(userID, update)

	return nil
}

// AnswerQuestionRequest holds the fields for answering an interrupted question.
type AnswerQuestionRequest struct {
	CheckpointID string `json:"checkpoint_id"`
	InterruptID  string `json:"interrupt_id"`
	Answer       string `json:"answer"`
}

// AnswerQuestion resumes an interrupted agent run with the user's answer.
func (s *ChatService) AnswerQuestion(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req AnswerQuestionRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify conversation ownership
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found: %w", ErrConversationNotFound)
	}

	// 2. Find the pending HITL record
	var hitl model.HumanInTheLoop
	if err := s.db.WithContext(ctx).Where("conversation_id = ? AND checkpoint_id = ? AND interrupt_id = ? AND status = 'pending'",
		conversationID, req.CheckpointID, req.InterruptID).First(&hitl).Error; err != nil {
		return fmt.Errorf("pending HITL request not found: %w", ErrHITLNotFound)
	}

	// 3. Update HITL to answered
	answerJSON := model.JSONMap{"text": req.Answer}
	if err := s.db.WithContext(ctx).Model(&hitl).Updates(map[string]interface{}{
		"status": "answered",
		"answer": answerJSON,
	}).Error; err != nil {
		return fmt.Errorf("failed to update HITL: %w", err)
	}

	// 4. Push human_in_the_loop.answered Update
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		s.log.Error("seq assignment failed", zap.Error(err))
	} else {
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "human_in_the_loop.answered",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"id":              hitl.ID.String(),
				"answer":          req.Answer,
				"seq":             seq,
			},
		}
		if dbErr := s.db.WithContext(ctx).Create(&update).Error; dbErr != nil {
			s.log.Error("failed to persist HITL answered update", zap.Error(dbErr))
		} else {
			pushUpdate(userID, update)
		}
	}

	// 5. Insert a user message recording the HITL answer
	answerSeq, answerSeqErr := nextSeq(ctx, userID)
	if answerSeqErr != nil {
		s.log.Error("seq assignment failed for HITL answer message", zap.Error(answerSeqErr))
	}

	var answerMsgID uuid.UUID
	if answerSeq > 0 {
		answerMsg := model.Message{
			ConversationID: conversationID,
			SenderRole:     "user",
			SenderID:       userID.String(),
			Content:        req.Answer,
			Metadata:       model.JSONMap{"type": "hitl_answer", "hitl_id": hitl.ID.String()},
			Seq:            answerSeq,
		}
		if err := s.db.WithContext(ctx).Create(&answerMsg).Error; err != nil {
			s.log.Error("failed to create HITL answer message", zap.Error(err))
		} else {
			answerMsgID = answerMsg.ID
			update := model.UserUpdate{
				UserID: userID,
				Seq:    answerSeq,
				Type:   "message.new",
				Payload: model.JSONMap{
					"conversation_id": conversationID.String(),
					"message_id":      answerMsgID.String(),
					"seq":             answerSeq,
					"role":            "user",
					"sender_id":       userID.String(),
					"addr":            "",
					"content":         req.Answer,
				},
			}
			if dbErr := s.db.WithContext(ctx).Create(&update).Error; dbErr != nil {
				s.log.Error("failed to persist HITL answer message update", zap.Error(dbErr))
			} else {
				pushUpdate(userID, update)
			}
		}
	}

	// 6. Set resume params and trigger a new agent run.
	// The runAgent method detects ResumeParams and uses ResumeWithParams.
	s.runSessionMgr.SetResumeParams(conversationID, &adk.ResumeParams{
		Targets: map[string]any{
			req.InterruptID: req.Answer,
		},
	})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("AnswerQuestion runAgent panic recovered", zap.Any("recover", r))
			}
		}()
		s.runAgent(context.WithoutCancel(ctx), userID, conversationID, "", nextSeq, pushUpdate)
	}()

	return nil
}

// ListPendingHITL returns pending human-in-the-loop requests for a conversation.
func (s *ChatService) ListPendingHITL(userID, conversationID uuid.UUID) ([]model.HumanInTheLoop, error) {
	ctx := context.Background()
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	var hitls []model.HumanInTheLoop
	if err := s.db.WithContext(ctx).Where("conversation_id = ? AND status = ?", conversationID, "pending").Order("created_at ASC").Find(&hitls).Error; err != nil {
		return nil, fmt.Errorf("failed to list pending HITL: %w", err)
	}
	return hitls, nil
}

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
	permissionID uuid.UUID,
	req AnswerPermissionRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify conversation ownership and get project ID.
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found: %w", ErrConversationNotFound)
	}

	var project model.Project
	if err := s.db.WithContext(ctx).Where("id = ?", conv.ProjectID).First(&project).Error; err != nil {
		return fmt.Errorf("project not found")
	}

	// 2. Find the pending permission record by ID.
	var perm model.HumanInPermission
	if err := s.db.WithContext(ctx).Where("id = ? AND status = 'pending'", permissionID).First(&perm).Error; err != nil {
		return fmt.Errorf("pending permission request not found: %w", ErrPermissionNotFound)
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
	if err := s.db.WithContext(ctx).Model(&perm).Updates(map[string]interface{}{
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
		s.db.WithContext(ctx).Table("project_tool_permissions").
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
			if err := s.db.WithContext(ctx).Create(&wp).Error; err != nil {
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
		if dbErr := s.db.WithContext(ctx).Create(&update).Error; dbErr != nil {
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

// ListPendingPermissions returns permission requests for a conversation.
func (s *ChatService) ListPendingPermissions(userID, conversationID uuid.UUID, status string) ([]model.HumanInPermission, error) {
	ctx := context.Background()
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	var perms []model.HumanInPermission
	query := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Order("created_at ASC")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return perms, nil
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
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
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

// StartCompaction stops the running agent (if any) and launches compression
// asynchronously. Returns immediately; progress is reported via Update events
// (conversation.compacting → conversation.compressed).
func (s *ChatService) StartCompaction(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found: %w", ErrConversationNotFound)
	}

	// 2. Stop running agent if any
	if s.runSessionMgr.IsRunning(conversationID) {
		s.runSessionMgr.Stop(conversationID)
	}

	// 3. Launch compression asynchronously — returns immediately
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("StartCompaction: panic recovered", zap.Any("recover", r))
			}
		}()
		if _, err := s.compressionSvc.CompressConversation(
			context.WithoutCancel(ctx), userID, conversationID, nextSeq, pushUpdate,
		); err != nil {
			s.log.Error("StartCompaction: compression failed", zap.Error(err))
		}
	}()

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

// loadConversationMessages loads DB messages for a conversation (seq >= minSeq)
// and converts them to []*schema.Message, preserving tool call history.
//
// It loads user, assistant, tool, and system messages. For assistant messages
// it parses tool_calls from the JSONB ToolCalling field. For tool messages it
// matches each call to its response by positional order, pairing the output
// with the tool_call_id so the LLM sees the complete call → result chain.
func loadConversationMessages(ctx context.Context, db *gorm.DB, conversationID uuid.UUID, minSeq int64) ([]*schema.Message, error) {
	var msgs []model.Message
	if err := db.WithContext(ctx).
		Select("sender_role", "content", "reason_content", "tool_calling", "metadata").
		Where("conversation_id = ? AND seq >= ? AND sender_role IN ?",
			conversationID, minSeq, []string{"user", "assistant", "tool", "system"}).
		Order("seq ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("failed to load conversation history: %w", err)
	}

	schemaMsgs := make([]*schema.Message, 0, len(msgs))
	// Track pending tool_call_ids to pair with subsequent tool messages.
	// Messages are in seq order, so tool calls appear before their results.
	var pendingToolCallIDs []string

	for _, m := range msgs {
		switch m.SenderRole {
		case "user":
			schemaMsgs = append(schemaMsgs, schema.UserMessage(m.Content))
		case "assistant":
			toolCalls := parseToolCalls(m.ToolCalling)
			asstMsg := schema.AssistantMessage(m.Content, toolCalls)
			if m.ReasonContent != "" {
				asstMsg.ReasoningContent = m.ReasonContent
			}
			schemaMsgs = append(schemaMsgs, asstMsg)
			// Collect tool_call_ids from this assistant message for pairing.
			for _, tc := range toolCalls {
				if tc.ID != "" {
					pendingToolCallIDs = append(pendingToolCallIDs, tc.ID)
				}
			}
		case "tool":
			// Tool message: content is the output, metadata has tool_name.
			// Pair with the next pending tool_call_id by position.
			toolCallID := ""
			if len(pendingToolCallIDs) > 0 {
				toolCallID = pendingToolCallIDs[0]
				pendingToolCallIDs = pendingToolCallIDs[1:]
			}
			content := m.Content
			if content == "" {
				// Fallback to tool_calling output if content is empty.
				if m.ToolCalling != nil {
					if out, ok := m.ToolCalling["output"].(string); ok {
						content = out
					}
				}
			}
			schemaMsgs = append(schemaMsgs, schema.ToolMessage(content, toolCallID))
		case "system":
			schemaMsgs = append(schemaMsgs, schema.SystemMessage(m.Content))
		}
	}

	return schemaMsgs, nil
}

// parseToolCalls extracts schema.ToolCall slice from a JSONB tool_calling field.
// The DB stores {"tool_calls": [...]}.
func parseToolCalls(toolCalling model.JSONMap) []schema.ToolCall {
	if toolCalling == nil {
		return nil
	}
	raw, ok := toolCalling["tool_calls"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var toolCalls []schema.ToolCall
	_ = json.Unmarshal(b, &toolCalls)
	return toolCalls
}

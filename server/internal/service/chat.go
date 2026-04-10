package service

import (
	"context"
	"fmt"
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
)

// agentRunTimeout is the maximum duration for a single agent execution event loop.
const agentRunTimeout = 10 * time.Minute

// ChatService handles sending messages and streaming AI responses.
type ChatService struct {
	db            *gorm.DB
	log           *zap.Logger
	modelProvider *eino.ModelProvider
	toolRegistry  *tools.ToolRegistry
	runSessionMgr *runner.RunSessionManager
}

// NewChatService creates a ChatService.
func NewChatService(
	db *gorm.DB,
	log *zap.Logger,
	modelProvider *eino.ModelProvider,
	toolRegistry *tools.ToolRegistry,
	runSessionMgr *runner.RunSessionManager,
) *ChatService {
	return &ChatService{
		db:            db,
		log:           log,
		modelProvider: modelProvider,
		toolRegistry:  toolRegistry,
		runSessionMgr: runSessionMgr,
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

	// 2. Allocate seq after ownership confirmed
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
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("runAgent panic recovered", zap.Any("recover", r))
			}
		}()
		s.runAgent(context.WithoutCancel(ctx), userID, conversationID, req.Content, nextSeq, pushUpdate)
	}()

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
	s.runSessionMgr.Start(conversationID, cancel, callbacks.Stop)

	// 5. Build run config
	runCfg := runner.RootRunnerConfig{
		ModelProvider: s.modelProvider,
		ModelTier:     modelTier,
		SystemPrompt:  systemPrompt,
		Tools:         s.toolRegistry.GetBaseTools(),
		MaxIteration:  100,
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
	if hasCheckpoint {
		iter, err = rootRunner.Resume(runCtx, checkpointID, handler)
		if err != nil {
			s.log.Error("runAgent: resume failed, falling back to fresh run", zap.Error(err), zap.String("checkpoint", checkpointID))
			iter = rootRunner.Run(runCtx, []*schema.Message{
				schema.UserMessage(userContent),
			}, "", handler)
		}
	} else {
		iter = rootRunner.Run(runCtx, []*schema.Message{
			schema.UserMessage(userContent),
		}, "", handler)
	}

	// 8. Consume events in goroutine with timeout protection and panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("runAgent event loop panic recovered", zap.Any("recover", r))
				s.runSessionMgr.Cleanup(conversationID)
				s.runSessionMgr.Done(conversationID)
				cancel()
			}
		}()
		defer func() {
			s.runSessionMgr.Cleanup(conversationID)
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
					callbacks.OnError(event.Err)
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

package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/tools"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
	openai "github.com/cloudwego/eino-ext/components/model/openai"
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

	// wsManager is used by sub-agents to push completion updates to parent conversations.
	wsManager interface {
		NextSeq(ctx context.Context, userID uuid.UUID) (int64, error)
		PushToUserConnections(userID uuid.UUID, update types.Update)
	}
	// convertFn converts model.UserUpdate to types.Update format.
	convertFn func(update model.UserUpdate) types.Update

	// mu guards runCompleteCallbacks
	mu sync.Mutex
	// runCompleteCallbacks maps conversationID → completion callback for sub-agent writeback.
	runCompleteCallbacks map[uuid.UUID]func(success bool, summary string)
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
	wsManager interface {
		NextSeq(ctx context.Context, userID uuid.UUID) (int64, error)
		PushToUserConnections(userID uuid.UUID, update types.Update)
	},
	convertFn func(update model.UserUpdate) types.Update,
) *ChatService {
	// Create haiku model config for permission evaluator (safety assessment)
	mc := modelProvider.GetModel("haiku")
	evalCfg := &openai.ChatModelConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
	}

	return &ChatService{
		db:                   db,
		log:                  log,
		modelProvider:        modelProvider,
		toolRegistry:         toolRegistry,
		runSessionMgr:        runSessionMgr,
		messageQueue:         messageQueue,
		compressionSvc:       compressionSvc,
		evalModelConfig:      evalCfg,
		wsManager:            wsManager,
		convertFn:            convertFn,
		runCompleteCallbacks: make(map[uuid.UUID]func(bool, string)),
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

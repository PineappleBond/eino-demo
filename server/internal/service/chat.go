package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// ChatService handles sending messages and streaming AI responses.
type ChatService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewChatService creates a ChatService.
func NewChatService(db *gorm.DB, log *zap.Logger) *ChatService {
	return &ChatService{db: db, log: log}
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

// SendMessage saves a user message, assigns seq, creates user_update, pushes via WS.
func (s *ChatService) SendMessage(userID, conversationID uuid.UUID, req SendMessageRequest) (*SendMessageResponse, error) {
	// Verify conversation exists and belongs to user
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// Create user message
	msg := model.Message{
		ConversationID: conversationID,
		SenderRole:     "user",
		SenderID:       userID.String(),
		Content:        req.Content,
		Metadata:       model.JSONMap{},
	}
	if err := s.db.Create(&msg).Error; err != nil {
		return nil, err
	}

	// Assign seq via Redis (through caller-provided nextSeq function)
	// Seq assignment, user_update creation, and WS push are handled by the caller
	// because they need access to the WebSocket Manager.

	// Return the response — caller fills in Seq after assignment
	return &SendMessageResponse{
		ConversationID: conversationID,
		MessageID:      msg.ID,
	}, nil
}

// NextSeqFunc is a function that assigns the next sequence number for a user.
type NextSeqFunc func(ctx context.Context, userID uuid.UUID) (int64, error)

// PushUpdateFunc pushes an update to a user's WebSocket connections.
type PushUpdateFunc func(userID uuid.UUID, update model.UserUpdate)

// CompleteSendMessage handles the full send flow: service logic + seq + update + push.
// This is called from the handler which has access to the WS Manager.
func (s *ChatService) CompleteSendMessage(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req SendMessageRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*SendMessageResponse, error) {
	resp, err := s.SendMessage(userID, conversationID, req)
	if err != nil {
		return nil, err
	}

	// Assign seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}
	resp.Seq = seq

	// Create user_update
	update := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "message.new",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
			"message_id":      resp.MessageID.String(),
			"role":            "user",
			"content":         req.Content,
			"seq":             seq,
		},
	}
	if err := s.db.Create(&update).Error; err != nil {
		return nil, err
	}

	// Push to WebSocket connections
	pushUpdate(userID, update)

	return resp, nil
}

// StopMessage marks the latest AI message as stopped.
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

	// Assign seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// Create user_update for the stop event
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

package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// ConversationService handles conversation CRUD logic.
type ConversationService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewConversationService creates a ConversationService.
func NewConversationService(db *gorm.DB, log *zap.Logger) *ConversationService {
	return &ConversationService{db: db, log: log}
}

// ListConversations returns all conversations for a project, scoped to user.
func (s *ConversationService) ListConversations(userID, projectID uuid.UUID) ([]model.Conversation, error) {
	var conversations []model.Conversation
	if err := s.db.Where("user_id = ? AND project_id = ?", userID, projectID).
		Order("updated_at DESC").Find(&conversations).Error; err != nil {
		return nil, err
	}
	return conversations, nil
}

// CreateConversationRequest holds the fields for creating a conversation.
type CreateConversationRequest struct {
	Title string `json:"title"`
}

// CreateConversation creates a new conversation within a project.
func (s *ConversationService) CreateConversation(userID, projectID uuid.UUID, req CreateConversationRequest) (*model.Conversation, error) {
	// Verify project exists and belongs to user
	var project model.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		return nil, fmt.Errorf("project not found")
	}

	conversation := model.Conversation{
		ProjectID: projectID,
		UserID:    userID,
		Title:     req.Title,
		Status:    "active",
	}
	if conversation.Title == "" {
		conversation.Title = "New Conversation"
	}

	if err := s.db.Create(&conversation).Error; err != nil {
		return nil, err
	}

	// Add user as conversation member
	member := model.ConversationMember{
		ConversationID: conversation.ID,
		MemberType:     "user",
		MemberID:       userID.String(),
		MemberName:     "User",
		IsOwner:        true,
	}
	if err := s.db.Create(&member).Error; err != nil {
		return nil, err
	}

	return &conversation, nil
}

// CompleteCreateConversation handles conversation creation with seq assignment and WS push.
func (s *ConversationService) CompleteCreateConversation(
	ctx context.Context,
	userID, projectID uuid.UUID,
	req CreateConversationRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	conv, err := s.CreateConversation(userID, projectID, req)
	if err != nil {
		return nil, err
	}

	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	update := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.created",
		Payload: model.JSONMap{
			"id":         conv.ID.String(),
			"project_id": projectID.String(),
			"title":      conv.Title,
			"status":     conv.Status,
			"seq":        seq,
		},
	}
	if err := s.db.Create(&update).Error; err != nil {
		return nil, err
	}

	pushUpdate(userID, update)
	return conv, nil
}

// GetConversation returns a single conversation by ID, scoped to user.
func (s *ConversationService) GetConversation(userID, conversationID uuid.UUID) (*model.Conversation, error) {
	var conversation model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conversation).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}
	return &conversation, nil
}

// DeleteConversation deletes a conversation and its messages (CASCADE).
func (s *ConversationService) DeleteConversation(userID, conversationID uuid.UUID) error {
	result := s.db.Where("id = ? AND user_id = ?", conversationID, userID).Delete(&model.Conversation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("conversation not found")
	}
	return nil
}

// CompleteDeleteConversation handles conversation deletion with seq assignment and WS push.
func (s *ConversationService) CompleteDeleteConversation(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// Verify conversation exists and belongs to user before deleting
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	if err := s.DeleteConversation(userID, conversationID); err != nil {
		return err
	}

	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	update := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.deleted",
		Payload: model.JSONMap{
			"id":         conversationID.String(),
			"project_id": conv.ProjectID.String(),
			"seq":        seq,
		},
	}
	if err := s.db.Create(&update).Error; err != nil {
		return err
	}

	pushUpdate(userID, update)
	return nil
}

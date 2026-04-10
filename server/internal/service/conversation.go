package service

import (
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

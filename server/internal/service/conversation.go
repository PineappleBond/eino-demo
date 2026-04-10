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

// CompleteCreateConversation handles conversation creation with seq assignment, user_update, and WS push.
// Creates both the conversation and user_update in a single transaction for atomicity.
func (s *ConversationService) CompleteCreateConversation(
	ctx context.Context,
	userID, projectID uuid.UUID,
	req CreateConversationRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	// 1. Allocate seq before transaction
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var conv *model.Conversation

	// 2. Create conversation + user member + user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Verify project exists and belongs to user
		var project model.Project
		if err := tx.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
			return fmt.Errorf("project not found")
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

		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		// Add user as conversation member
		member := model.ConversationMember{
			ConversationID: conversation.ID,
			MemberType:     "user",
			MemberID:       userID.String(),
			MemberName:     "User",
			IsOwner:        true,
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}

		// Create user_update in the same transaction
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.created",
			Payload: model.JSONMap{
				"id":         conversation.ID.String(),
				"project_id": projectID.String(),
				"title":      conversation.Title,
				"status":     conversation.Status,
				"seq":        seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}

		conv = &conversation
		return nil
	})
	if err != nil {
		return nil, err
	}

	pushUpdate(userID, model.UserUpdate{
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
	})
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

// ListMembers returns all members of a conversation.
func (s *ConversationService) ListMembers(conversationID uuid.UUID) ([]model.ConversationMember, error) {
	var members []model.ConversationMember
	if err := s.db.Where("conversation_id = ?", conversationID).Order("is_owner DESC").Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
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

// CompleteDeleteConversation handles conversation deletion with seq assignment, user_update, and WS push.
// Verifies ownership before allocating seq, wraps delete + user_update in a single transaction.
func (s *ConversationService) CompleteDeleteConversation(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify ownership first — before allocating seq
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	// 2. Allocate seq after ownership confirmed
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Delete conversation + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND user_id = ?", conversationID, userID).Delete(&model.Conversation{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("conversation not found")
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
		if err := tx.Create(&update).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.deleted",
		Payload: model.JSONMap{
			"id":         conversationID.String(),
			"project_id": conv.ProjectID.String(),
			"seq":        seq,
		},
	})
	return nil
}

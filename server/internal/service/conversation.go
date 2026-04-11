package service

import (
	"context"
	"fmt"
	"strings"

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

// ListMembers returns all members of a conversation, scoped to user.
func (s *ConversationService) ListMembers(userID, conversationID uuid.UUID) ([]model.ConversationMember, error) {
	// Verify the conversation belongs to the user
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	var members []model.ConversationMember
	if err := s.db.Where("conversation_id = ?", conversationID).Order("is_owner DESC").Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

// CompactConversation summarizes a conversation and emits compacting/compacted updates.
// Creates a new conversation with the summary and marks the old one as compacted.
func (s *ConversationService) CompactConversation(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	// 2. Allocate seq for compacting event
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Emit conversation.compacting
	compactingUpdate := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.compacting",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
			"seq":             seq,
		},
	}
	if err := s.db.WithContext(ctx).Create(&compactingUpdate).Error; err != nil {
		s.log.Error("failed to persist compacting update", zap.Error(err))
	}
	pushUpdate(userID, compactingUpdate)

	// 4. Fetch messages for summarization (caller should handle actual LLM summarization)
	var messages []model.Message
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("seq ASC").Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	// Build summary from messages
	var summary strings.Builder
	for _, m := range messages {
		if m.SenderRole == "user" {
			summary.WriteString("User: " + m.Content + "\n")
		} else if m.SenderRole == "assistant" {
			summary.WriteString("Assistant: " + m.Content + "\n")
		}
	}

	// 5. Mark old conversation as compacted
	compactedSeq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var newConv *model.Conversation

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Update old conversation status
		if err := tx.Model(&conv).Updates(map[string]interface{}{
			"status":  "compacted",
			"summary": summary.String(),
		}).Error; err != nil {
			return err
		}

		// Create new conversation with summary as first message
		newConv = &model.Conversation{
			ProjectID: conv.ProjectID,
			UserID:    userID,
			Title:     "Continued from: " + conv.Title,
			Status:    "active",
		}
		if err := tx.Create(newConv).Error; err != nil {
			return err
		}

		// Add user as member
		member := model.ConversationMember{
			ConversationID: newConv.ID,
			MemberType:     "user",
			MemberID:       userID.String(),
			MemberName:     "User",
			IsOwner:        true,
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}

		// Emit conversation.compacted
		compactedUpdate := model.UserUpdate{
			UserID: userID,
			Seq:    compactedSeq,
			Type:   "conversation.compacted",
			Payload: model.JSONMap{
				"old_conv_id": conversationID.String(),
				"new_conv_id": newConv.ID.String(),
				"project_id":  conv.ProjectID.String(),
				"seq":         compactedSeq,
			},
		}
		if err := tx.Create(&compactedUpdate).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    compactedSeq,
		Type:   "conversation.compacted",
		Payload: model.JSONMap{
			"old_conv_id": conversationID.String(),
			"new_conv_id": newConv.ID.String(),
			"project_id":  conv.ProjectID.String(),
			"seq":         compactedSeq,
		},
	})

	return newConv, nil
}
// RenameConversationRequest holds the fields for renaming a conversation.
type RenameConversationRequest struct {
	Title string `json:"title"`
}

// CompleteRenameConversation handles conversation rename with seq assignment, user_update, and WS push.
func (s *ConversationService) CompleteRenameConversation(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req RenameConversationRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	// 2. Allocate seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Update title + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&conv).Update("title", req.Title).Error; err != nil {
			return err
		}
		conv.Title = req.Title

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.updated",
			Payload: model.JSONMap{
				"id":         conversationID.String(),
				"project_id": conv.ProjectID.String(),
				"title":      conv.Title,
				"seq":        seq,
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

	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.updated",
		Payload: model.JSONMap{
			"id":         conversationID.String(),
			"project_id": conv.ProjectID.String(),
			"title":      conv.Title,
			"seq":        seq,
		},
	})
	return &conv, nil
}

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

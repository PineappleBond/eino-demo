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
		s.log.Error("list conversations: query failed",
			zap.String("user_id", userID.String()),
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
		return nil, err
	}
	return conversations, nil
}

// CreateConversationRequest holds the fields for creating a conversation.
type CreateConversationRequest struct {
	Title string `json:"title"`
	Mode  string `json:"mode"`
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
			s.log.Error("create conversation: project not found",
				zap.String("user_id", userID.String()),
				zap.String("project_id", projectID.String()),
			)
			return fmt.Errorf("project not found")
		}

		conversation := model.Conversation{
			ProjectID: projectID,
			UserID:    userID,
			Title:     req.Title,
			Status:    "active",
			Mode:      req.Mode,
		}
		if conversation.Title == "" {
			conversation.Title = "New Conversation"
		}
		if conversation.Mode == "" {
			conversation.Mode = "ask_before_edits"
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
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}
	return &conversation, nil
}

// ListMembers returns all members of a conversation, scoped to user.
func (s *ConversationService) ListMembers(userID, conversationID uuid.UUID) ([]model.ConversationMember, error) {
	// Verify the conversation belongs to the user
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
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
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
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
		s.log.Error("rename conversation: not found",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
		)
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
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
		s.log.Error("rename conversation: transaction failed",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
			zap.Error(err),
		)
		return nil, err
	}
	s.log.Info("conversation renamed",
		zap.String("user_id", userID.String()),
		zap.String("conv_id", conversationID.String()),
		zap.String("title", conv.Title),
	)
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

// UpdateConversationModeRequest holds the fields for updating a conversation mode.
type UpdateConversationModeRequest struct {
	Mode string `json:"mode"`
}

// CompleteUpdateMode handles conversation mode update with seq assignment, user_update, and WS push.
func (s *ConversationService) CompleteUpdateMode(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req UpdateConversationModeRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		s.log.Error("update conversation mode: not found",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
		)
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Validate mode
	validModes := map[string]bool{
		"ask_before_edits":   true,
		"edit_automatically": true,
		"bypass_permissions": true,
		"plan_mode":          true,
	}
	if !validModes[req.Mode] {
		return nil, fmt.Errorf("invalid mode: %s", req.Mode)
	}

	// 3. Allocate seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	// 4. Update mode + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&conv).Update("mode", req.Mode).Error; err != nil {
			return err
		}
		conv.Mode = req.Mode

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.updated",
			Payload: model.JSONMap{
				"id":         conversationID.String(),
				"project_id": conv.ProjectID.String(),
				"title":      conv.Title,
				"mode":       conv.Mode,
				"seq":        seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		s.log.Error("update conversation mode: transaction failed",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
			zap.Error(err),
		)
		return nil, err
	}
	s.log.Info("conversation mode updated",
		zap.String("user_id", userID.String()),
		zap.String("conv_id", conversationID.String()),
		zap.String("mode", conv.Mode),
	)
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.updated",
		Payload: model.JSONMap{
			"id":         conversationID.String(),
			"project_id": conv.ProjectID.String(),
			"title":      conv.Title,
			"mode":       conv.Mode,
			"seq":        seq,
		},
	})
	return &conv, nil
}

// UpdateConversationStatusRequest holds the fields for updating a conversation status.
type UpdateConversationStatusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus updates a conversation's status and emits appropriate Update events.
// When status is "archived", it sends both conversation.updated and conversation.archived events.
func (s *ConversationService) UpdateStatus(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req UpdateConversationStatusRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		s.log.Error("update conversation status: not found",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
		)
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Allocate seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Update status + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&conv).Update("status", req.Status).Error; err != nil {
			return err
		}
		conv.Status = req.Status

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.updated",
			Payload: model.JSONMap{
				"id":         conversationID.String(),
				"project_id": conv.ProjectID.String(),
				"title":      conv.Title,
				"status":     conv.Status,
				"seq":        seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}

		// If archiving, also send conversation.archived event
		if req.Status == "archived" {
			archivedSeq, seqErr := nextSeq(ctx, userID)
			if seqErr != nil {
				return fmt.Errorf("seq assignment failed for archived: %w", seqErr)
			}
			archivedUpdate := model.UserUpdate{
				UserID: userID,
				Seq:    archivedSeq,
				Type:   "conversation.archived",
				Payload: model.JSONMap{
					"conversation_id": conversationID.String(),
					"seq":             archivedSeq,
				},
			}
			if err := tx.Create(&archivedUpdate).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("conversation status updated",
		zap.String("user_id", userID.String()),
		zap.String("conv_id", conversationID.String()),
		zap.String("old_status", conv.Status),
		zap.String("new_status", req.Status),
	)

	// 4. Push conversation.updated
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.updated",
		Payload: model.JSONMap{
			"id":         conversationID.String(),
			"project_id": conv.ProjectID.String(),
			"title":      conv.Title,
			"status":     conv.Status,
			"seq":        seq,
		},
	})

	// 5. Push conversation.archived if applicable
	if req.Status == "archived" {
		archivedSeq, err := nextSeq(ctx, userID)
		if err != nil {
			s.log.Error("seq assignment failed for archived push", zap.Error(err))
		} else if archivedSeq > 0 {
			pushUpdate(userID, model.UserUpdate{
				UserID: userID,
				Seq:    archivedSeq,
				Type:   "conversation.archived",
				Payload: model.JSONMap{
					"conversation_id": conversationID.String(),
					"seq":             archivedSeq,
				},
			})
		}
	}

	return &conv, nil
}

// BranchConversationRequest holds the fields for branching a conversation.
type BranchConversationRequest struct {
	InputSeq int64 `json:"input_seq"`
}

// BranchConversation creates a new conversation by copying messages up to input_seq
// from the source conversation. The new conversation inherits the Mode but is
// independent (no parent link).
func (s *ConversationService) BranchConversation(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req BranchConversationRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Conversation, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Validate input_seq
	if req.InputSeq > conv.LatestMessageSeq {
		return nil, fmt.Errorf("input_seq %d exceeds latest message seq %d", req.InputSeq, conv.LatestMessageSeq)
	}

	// 3. Fetch messages up to input_seq
	var messages []model.Message
	if err := s.db.Where("conversation_id = ? AND seq <= ?", conversationID, req.InputSeq).
		Order("seq ASC").Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages found up to seq %d", req.InputSeq)
	}

	// 4. Allocate seq for conversation.created event
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var newConv *model.Conversation

	// 5. Create new conversation + copy messages + user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create the branched conversation
		newConv = &model.Conversation{
			ProjectID: conv.ProjectID,
			UserID:    userID,
			Title:     "Branch of: " + conv.Title,
			Status:    "active",
			Mode:      conv.Mode, // Inherit mode
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

		// Copy messages with reassigned seq
		for i, msg := range messages {
			newMsg := model.Message{
				ConversationID:   newConv.ID,
				Seq:              int64(i + 1),
				SenderRole:       msg.SenderRole,
				SenderID:         msg.SenderID,
				Content:          msg.Content,
				ReasonContent:    msg.ReasonContent,
				ReplyToSeq:       msg.ReplyToSeq,
				MentionedMembers: msg.MentionedMembers,
				Metadata:         msg.Metadata,
				FinishReason:     msg.FinishReason,
				ErrorMessage:     msg.ErrorMessage,
				DurationMs:       msg.DurationMs,
				TokenPrompt:      msg.TokenPrompt,
				TokenCompletion:  msg.TokenCompletion,
				ToolCalling:      msg.ToolCalling,
			}
			if err := tx.Create(&newMsg).Error; err != nil {
				return err
			}
		}

		// Update new conversation's message count and latest seq
		if err := tx.Model(newConv).Updates(map[string]interface{}{
			"message_count":      len(messages),
			"latest_message_seq": len(messages),
		}).Error; err != nil {
			return err
		}

		// Create user_update
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.created",
			Payload: model.JSONMap{
				"id":         newConv.ID.String(),
				"project_id": newConv.ProjectID.String(),
				"title":      newConv.Title,
				"status":     newConv.Status,
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

	// 6. Push the update so frontend sees the new conversation
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.created",
		Payload: model.JSONMap{
			"id":         newConv.ID.String(),
			"project_id": newConv.ProjectID.String(),
			"title":      newConv.Title,
			"status":     newConv.Status,
			"seq":        seq,
		},
	})

	s.log.Info("conversation branched",
		zap.String("user_id", userID.String()),
		zap.String("source_conv_id", conversationID.String()),
		zap.String("new_conv_id", newConv.ID.String()),
		zap.Int64("input_seq", req.InputSeq),
		zap.Int("messages_copied", len(messages)),
	)

	return newConv, nil
}

func (s *ConversationService) DeleteConversation(userID, conversationID uuid.UUID) error {
	result := s.db.Where("id = ? AND user_id = ?", conversationID, userID).Delete(&model.Conversation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("get conversation: %w", ErrConversationNotFound)
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
		s.log.Error("delete conversation: not found",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
		)
		return fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Allocate seq after ownership confirmed
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		s.log.Error("delete conversation: seq assignment failed",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Delete conversation + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND user_id = ?", conversationID, userID).Delete(&model.Conversation{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("get conversation: %w", ErrConversationNotFound)
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

	s.log.Info("conversation deleted",
		zap.String("user_id", userID.String()),
		zap.String("conv_id", conversationID.String()),
	)
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

// SubInterruptResult holds pending permissions and HITLs from sub-conversations.
type SubInterruptResult struct {
	Permissions []model.HumanInPermission
	HITLs       []model.HumanInTheLoop
}

// GetSubInterrupts returns all pending permissions and HITLs from sub-conversations
// of the given conversation. This enables the parent conversation page to display
// and resolve sub-conversation interrupts.
func (s *ConversationService) GetSubInterrupts(
	ctx context.Context,
	userID, conversationID uuid.UUID,
) (*SubInterruptResult, error) {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Find all sub-conversations
	var subConvs []model.Conversation
	if err := s.db.WithContext(ctx).Where("parent_conversation_id = ? AND user_id = ?", conversationID, userID).
		Find(&subConvs).Error; err != nil {
		return nil, fmt.Errorf("failed to find sub-conversations: %w", err)
	}

	result := &SubInterruptResult{}
	if len(subConvs) == 0 {
		return result, nil
	}

	// 3. Collect sub-conversation IDs
	subConvIDs := make([]uuid.UUID, len(subConvs))
	for i, sc := range subConvs {
		subConvIDs[i] = sc.ID
	}

	// 4. Query pending permissions from sub-conversations
	if err := s.db.WithContext(ctx).
		Where("source_conversation_id IN ? AND status = 'pending'", subConvIDs).
		Order("created_at DESC").
		Find(&result.Permissions).Error; err != nil {
		return nil, fmt.Errorf("failed to query sub permissions: %w", err)
	}

	// 5. Query pending HITLs from sub-conversations
	if err := s.db.WithContext(ctx).
		Where("source_conversation_id IN ? AND status = 'pending'", subConvIDs).
		Order("created_at DESC").
		Find(&result.HITLs).Error; err != nil {
		return nil, fmt.Errorf("failed to query sub hitls: %w", err)
	}

	return result, nil
}

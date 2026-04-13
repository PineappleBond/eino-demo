package service

import (
	"context"
	"fmt"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

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
	if _, err := s.verifyConversationOwnedByUser(ctx, conversationID, userID); err != nil {
		return err
	}

	// 2. Atomically transition the HITL record from pending to answered.
	// Uses RowsAffected to detect double-resume (concurrent requests).
	result := s.db.WithContext(ctx).Model(&model.HumanInTheLoop{}).
		Where("conversation_id = ? AND checkpoint_id = ? AND interrupt_id = ? AND status = 'pending'",
			conversationID, req.CheckpointID, req.InterruptID).
		Updates(map[string]interface{}{
			"status": "answered",
			"answer": model.JSONMap{"text": req.Answer},
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update HITL: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("HITL request already resolved: %w", ErrHITLNotFound)
	}

	// Load the record for display in WS events.
	var hitl model.HumanInTheLoop
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND checkpoint_id = ? AND interrupt_id = ?",
			conversationID, req.CheckpointID, req.InterruptID).
		First(&hitl).Error; err != nil {
		return fmt.Errorf("HITL record not found after update: %w", ErrHITLNotFound)
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

	// 6. Resume agent with the user's answer.
	s.resumeAgent(ctx, userID, conversationID, req.InterruptID, req.Answer, nextSeq, pushUpdate)

	return nil
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
	conv, err := s.verifyConversationOwnedByUser(ctx, conversationID, userID)
	if err != nil {
		return err
	}

	var project model.Project
	if err := s.db.WithContext(ctx).Where("id = ?", conv.ProjectID).First(&project).Error; err != nil {
		return fmt.Errorf("project not found")
	}

	// 2. Validate decision before mutating state.
	validDecisions := map[string]bool{
		"approved": true, "approved_exact": true,
		"approved_wildcard": true, "denied": true,
	}
	if !validDecisions[req.Decision] {
		return fmt.Errorf("invalid decision: %s", req.Decision)
	}

	// 3. Atomically transition the permission record from pending to answered.
	// Uses RowsAffected to detect double-resume (concurrent requests).
	result := s.db.WithContext(ctx).Model(&model.HumanInPermission{}).
		Where("id = ? AND conversation_id = ? AND status = 'pending'", permissionID, conversationID).
		Updates(map[string]interface{}{"status": "answered", "decision": req.Decision})
	if result.Error != nil {
		return fmt.Errorf("update permission: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("permission request already resolved: %w", ErrPermissionNotFound)
	}

	// Load the record for display in WS events.
	var perm model.HumanInPermission
	if err := s.db.WithContext(ctx).Where("id = ?", permissionID).First(&perm).Error; err != nil {
		return fmt.Errorf("permission record not found after update: %w", ErrPermissionNotFound)
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

	// 7. Resume agent with the user's decision.
	s.resumeAgent(ctx, userID, conversationID, req.InterruptID, req.Decision, nextSeq, pushUpdate)

	return nil
}

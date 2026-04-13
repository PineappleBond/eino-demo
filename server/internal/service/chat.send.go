package service

import (
	"context"
	"fmt"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/google/uuid"
)

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

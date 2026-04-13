package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// isContextOverflowError checks if an error indicates context length exceeded.
func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, p := range []string{
		"context_length_exceeded",
		"prompt_too_long",
		"context_overflow",
		"maximum context length",
		"input is too long",
		"too many tokens",
		"token overflow", // TokenOverflowError from contextInjectionMiddleware
	} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// loadConversationMessages loads DB messages for a conversation (seq >= minSeq)
// and converts them to []*schema.Message, preserving tool call history.
//
// It loads user, assistant, tool, and system messages. For assistant messages
// it parses tool_calls from the JSONB ToolCalling field. For tool messages it
// matches each call to its response by positional order, pairing the output
// with the tool_call_id so the LLM sees the complete call → result chain.
func loadConversationMessages(ctx context.Context, db *gorm.DB, conversationID uuid.UUID, minSeq int64) ([]*schema.Message, error) {
	var msgs []model.Message
	if err := db.WithContext(ctx).
		Select("sender_role", "content", "reason_content", "tool_calling", "metadata").
		Where("conversation_id = ? AND seq >= ? AND sender_role IN ?",
			conversationID, minSeq, []string{"user", "assistant", "tool", "system"}).
		Order("seq ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("failed to load conversation history: %w", err)
	}

	schemaMsgs := make([]*schema.Message, 0, len(msgs))
	// Track pending tool_call_ids to pair with subsequent tool messages.
	// Messages are in seq order, so tool calls appear before their results.
	var pendingToolCallIDs []string

	for _, m := range msgs {
		switch m.SenderRole {
		case "user":
			schemaMsgs = append(schemaMsgs, schema.UserMessage(m.Content))
		case "assistant":
			toolCalls := parseToolCalls(m.ToolCalling)
			asstMsg := schema.AssistantMessage(m.Content, toolCalls)
			if m.ReasonContent != "" {
				asstMsg.ReasoningContent = m.ReasonContent
			}
			schemaMsgs = append(schemaMsgs, asstMsg)
			// Collect tool_call_ids from this assistant message for pairing.
			for _, tc := range toolCalls {
				if tc.ID != "" {
					pendingToolCallIDs = append(pendingToolCallIDs, tc.ID)
				}
			}
		case "tool":
			// Tool message: content is the output, metadata has tool_name.
			// Pair with the next pending tool_call_id by position.
			toolCallID := ""
			if len(pendingToolCallIDs) > 0 {
				toolCallID = pendingToolCallIDs[0]
				pendingToolCallIDs = pendingToolCallIDs[1:]
			}
			content := m.Content
			if content == "" {
				// Fallback to tool_calling output if content is empty.
				if m.ToolCalling != nil {
					if out, ok := m.ToolCalling["output"].(string); ok {
						content = out
					}
				}
			}
			schemaMsgs = append(schemaMsgs, schema.ToolMessage(content, toolCallID))
		case "system":
			schemaMsgs = append(schemaMsgs, schema.SystemMessage(m.Content))
		}
	}

	return schemaMsgs, nil
}

// parseToolCalls extracts schema.ToolCall slice from a JSONB tool_calling field.
// The DB stores {"tool_calls": [...]}.
func parseToolCalls(toolCalling model.JSONMap) []schema.ToolCall {
	if toolCalling == nil {
		return nil
	}
	raw, ok := toolCalling["tool_calls"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var toolCalls []schema.ToolCall
	_ = json.Unmarshal(b, &toolCalls)
	return toolCalls
}

// verifyConversationOwnedByUser checks that the conversation belongs to the user.
// Returns the conversation on success, or an error if not found.
func (s *ChatService) verifyConversationOwnedByUser(ctx context.Context, conversationID, userID uuid.UUID) (*model.Conversation, error) {
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found: %w", ErrConversationNotFound)
	}
	return &conv, nil
}

package tools

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AskUserQuestionTool struct {
	ConversationID uuid.UUID
	db             *gorm.DB
	// push?
}

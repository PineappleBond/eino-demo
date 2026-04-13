package model

import (
	"time"

	"github.com/google/uuid"
)

// Todo represents a todo item scoped to a conversation.
// Managed by the agent via the todo_write tool or by users via HTTP endpoints.
type Todo struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	Content        string    `gorm:"type:text;not null"`
	Completed      bool      `gorm:"not null;default:false"`
	Metadata       JSONMap   `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (Todo) TableName() string { return "todos" }

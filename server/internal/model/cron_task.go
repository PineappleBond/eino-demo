package model

import (
	"time"

	"github.com/google/uuid"
)

// CronTask represents a scheduled task scoped to a conversation.
// When triggered, it sends a message to the conversation, triggering the AI agent.
type CronTask struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	Content        string    `gorm:"type:text;not null"`
	SenderRole     string    `gorm:"type:text;not null;default:'user'"`
	Schedule       string    `gorm:"type:text;not null"`
	NextRunAt      time.Time `gorm:"index"`
	Status         string    `gorm:"type:varchar(20);not null;default:'pending'"`
}

func (CronTask) TableName() string { return "cron_tasks" }

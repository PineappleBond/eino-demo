package model

import (
	"time"

	"github.com/google/uuid"
)

// HumanInTheLoop represents a pending question from the agent to the user.
// Created when the agent calls the ask_user_question tool and interrupts execution.
type HumanInTheLoop struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	CheckpointID   string    `gorm:"type:varchar(255);not null"`
	InterruptID    string    `gorm:"type:varchar(255);not null"`
	Question       string    `gorm:"type:text;not null"`
	Choices        JSONMap   `gorm:"type:jsonb;not null;default:'[]'"`
	AnswerType     string    `gorm:"type:varchar(10);not null;default:'text'"` // single|multi|text
	Answer         JSONMap   `gorm:"type:jsonb;default:null"`
	Status         string    `gorm:"type:varchar(20);not null;default:'pending'"` // pending|answered|expired
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (HumanInTheLoop) TableName() string { return "human_in_the_loops" }

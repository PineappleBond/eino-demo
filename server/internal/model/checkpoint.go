package model

import (
	"github.com/google/uuid"
)

// Checkpoint stores interrupt/resume state for long-running agent executions.
type Checkpoint struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	MessageID      uuid.UUID `gorm:"type:uuid;not null;index:idx_checkpoint_message;constraint:OnDelete:CASCADE"`
	MessageSeq     int64     `gorm:"not null"`
	NodeKey        string    `gorm:"type:varchar(128);not null"`
	State          []byte    `gorm:"type:bytea;not null"`
}

func (Checkpoint) TableName() string { return "checkpoints" }

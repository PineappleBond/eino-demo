package model

import (
	"time"

	"github.com/google/uuid"
)

// HumanInPermission represents a pending tool permission request awaiting user approval.
type HumanInPermission struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	CheckpointID   string    `gorm:"type:varchar(255);not null"`
	InterruptID    string    `gorm:"type:varchar(255);not null"`
	ToolName       string    `gorm:"type:varchar(128);not null"`
	Action         string    `gorm:"type:varchar(64);not null"`
	Content        string    `gorm:"type:text;not null"`
	ToolDesc       string    `gorm:"type:text;not null"`
	ArgsSummary    string    `gorm:"type:text;not null"`
	SafetyLevel    int       `gorm:"not null"`
	SafetyReason   string    `gorm:"type:text;not null"`
	Decision       string      `gorm:"type:varchar(20);default:null"`
	Status         string      `gorm:"type:varchar(20);not null;default:'pending'"`
	CreatedAt      time.Time   `gorm:"autoCreateTime"`
	// SourceConversationID is set when this permission originates from a sub-agent.
	// It points to the child conversation that triggered the interrupt.
	SourceConversationID *uuid.UUID `gorm:"type:uuid"`
}

func (HumanInPermission) TableName() string { return "human_in_permissions" }

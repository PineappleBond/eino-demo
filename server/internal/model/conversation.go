package model

import (
	"time"

	"github.com/google/uuid"
)

// Conversation represents a single chat session within a project.
type Conversation struct {
	BaseModel
	ProjectID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID               uuid.UUID  `gorm:"type:uuid;not null;index"`
	Title                string     `gorm:"type:varchar(512);not null;default:''"`
	Summary              string     `gorm:"type:text;not null;default:''"`
	Status               string     `gorm:"type:varchar(20);not null;default:'active';index:idx_user_status"`
	LastMessagePreview   string     `gorm:"type:text"`
	MessageCount         int        `gorm:"not null;default:0"`
	LatestMessageSeq     int64      `gorm:"not null;default:0"`
	TokenPrompt          int64      `gorm:"not null;default:0"`
	TokenCompletion      int64      `gorm:"not null;default:0"`
	MemberCount          int        `gorm:"not null;default:0"`
	ParentConversationID *uuid.UUID `gorm:"type:uuid"`
	UpdatedAt            time.Time  `gorm:"not null;autoUpdateTime;index"`
}

func (Conversation) TableName() string { return "conversations" }

// ConversationMember tracks who/what agents are in a conversation.
type ConversationMember struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index"`
	MemberType     string    `gorm:"type:varchar(10);not null"` // "user" or "agent"
	MemberID       string    `gorm:"type:varchar(255);not null"`
	MemberName     string    `gorm:"type:varchar(255);not null;default:''"`
	IsOwner        bool      `gorm:"not null;default:false"`
}

func (ConversationMember) TableName() string { return "conversation_members" }

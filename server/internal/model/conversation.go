package model

import (
	"time"

	"github.com/google/uuid"
)

// Conversation represents a single chat session within a project.
type Conversation struct {
	BaseModel
	ProjectID            uuid.UUID  `gorm:"type:uuid;not null;index:idx_conv_project;constraint:OnDelete:CASCADE"`
	UserID               uuid.UUID  `gorm:"type:uuid;not null;index:idx_conv_user,idx_conv_user_status;constraint:OnDelete:CASCADE"`
	Title                string     `gorm:"type:varchar(512);not null;default:''"`
	Summary              string     `gorm:"type:text;not null;default:''"`
	Status               string     `gorm:"type:varchar(20);not null;default:'active';index:idx_conv_user_status"`
	LastMessagePreview   string     `gorm:"type:text"`
	MessageCount         int        `gorm:"not null;default:0"`
	LatestMessageSeq     int64      `gorm:"not null;default:0"`
	TokenPrompt          int64      `gorm:"not null;default:0"`
	TokenCompletion      int64      `gorm:"not null;default:0"`
	MinSeq               int64      `gorm:"not null;default:0"` // Messages below this seq have been compressed
	MemberCount          int        `gorm:"not null;default:0"`
	ParentConversationID *uuid.UUID `gorm:"type:uuid"`
	UpdatedAt            time.Time  `gorm:"not null;autoUpdateTime;index"`
	CheckpointID         string     `gorm:"type:varchar(255);not null;default:''"` // Eino checkpoint for resume
	Mode                 string     `gorm:"type:varchar(30);not null;default:'ask_before_edits'"` // conversation interaction mode
}

func (Conversation) TableName() string { return "conversations" }

// ConversationMember tracks who/what agents are in a conversation.
type ConversationMember struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_conv_member;index:idx_conv_member_conv;constraint:OnDelete:CASCADE"`
	MemberType     string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_conv_member"`
	MemberID       string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_conv_member"`
	MemberName     string    `gorm:"type:varchar(255);not null;default:''"`
	IsOwner        bool      `gorm:"not null;default:false"`
}

func (ConversationMember) TableName() string { return "conversation_members" }

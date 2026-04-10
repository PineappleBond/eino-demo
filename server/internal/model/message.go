package model

import (
	"github.com/google/uuid"
)

// Message is a single message in a conversation.
type Message struct {
	BaseModel
	ConversationID   uuid.UUID      `gorm:"type:uuid;not null;index:idx_conv_seq;constraint:OnDelete:CASCADE"`
	Seq              int64          `gorm:"not null;index:idx_conv_seq"`
	SenderRole       string         `gorm:"type:varchar(20);not null"`
	SenderID         string         `gorm:"type:varchar(255);not null"`
	Content          string         `gorm:"type:text;not null;default:''"`
	ReasonContent    string         `gorm:"type:text;not null;default:''"`
	ReplyToSeq       *int64         `gorm:""`
	MentionedMembers []string       `gorm:"type:text[]"`
	Metadata         JSONMap        `gorm:"type:jsonb;not null;default:'{}'"`
	FinishReason     *string        `gorm:"type:varchar(20)"`
	ErrorMessage     *string        `gorm:"type:text"`
	DurationMs       *int           `gorm:""`
	TokenPrompt      int64          `gorm:"not null;default:0"`
	TokenCompletion  int64          `gorm:"not null;default:0"`
}

func (Message) TableName() string { return "messages" }

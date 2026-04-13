package model

import (
	"time"

	"github.com/google/uuid"
)

// Message is a single message in a conversation.
type Message struct {
	BaseModel
	ConversationID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_conv_seq;constraint:OnDelete:CASCADE"`
	Seq              int64     `gorm:"not null;uniqueIndex:idx_conv_seq"`
	SenderRole       string    `gorm:"type:varchar(20);not null"`
	SenderID         string    `gorm:"type:varchar(255);not null"`
	Content          string    `gorm:"type:text;not null;default:''"`
	ReasonContent    string    `gorm:"type:text;not null;default:''"`
	ReplyToSeq       *int64    `gorm:""`
	MentionedMembers []string  `gorm:"type:text[]"` // Note: pgx driver handles []string ↔ text[] at runtime
	Metadata         JSONMap   `gorm:"type:jsonb;not null;default:'{}'"`
	FinishReason     *string   `gorm:"type:varchar(20)"`
	ErrorMessage     *string   `gorm:"type:text"`
	DurationMs       *int      `gorm:""`
	TokenPrompt      int64     `gorm:"not null;default:0"`
	TokenCompletion  int64     `gorm:"not null;default:0"`
	ToolCalling      JSONMap   `gorm:"type:jsonb;default:null"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

func (Message) TableName() string { return "messages" }

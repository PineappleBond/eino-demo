package model

import (
	"time"

	"github.com/google/uuid"
)

// UserUpdate is the global Update event log for seq-based real-time sync.
type UserUpdate struct {
	BaseModel
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index:idx_user_seq"`
	Seq       int64          `gorm:"not null;index:idx_user_seq"`
	Type      string         `gorm:"type:varchar(40);not null"`
	Payload   map[string]any `gorm:"type:jsonb;not null"`
	CreatedAt time.Time      `gorm:"not null"`
}

func (UserUpdate) TableName() string { return "user_updates" }

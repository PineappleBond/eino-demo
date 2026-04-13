package model

import (
	"github.com/google/uuid"
)

// UserUpdate is the global Update event log for seq-based real-time sync.
type UserUpdate struct {
	BaseModel
	UserID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_seq;index:idx_user_seq_lookup"`
	Seq     int64     `gorm:"not null;uniqueIndex:idx_user_seq"`
	Type    string    `gorm:"type:varchar(40);not null"`
	Payload JSONMap   `gorm:"type:jsonb;not null"`
}

func (UserUpdate) TableName() string { return "user_updates" }

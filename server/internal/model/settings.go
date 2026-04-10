package model

import (
	"time"

	"github.com/google/uuid"
)

// Settings stores user preferences.
type Settings struct {
	BaseModel
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	ModelTier string    `gorm:"type:varchar(10);not null;default:'sonnet'"`
	Locale    string    `gorm:"type:varchar(5);not null;default:'en'"`
	Theme     string    `gorm:"type:varchar(10);not null;default:'light'"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

func (Settings) TableName() string { return "settings" }

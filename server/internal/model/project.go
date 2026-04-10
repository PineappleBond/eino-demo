package model

import (
	"time"

	"github.com/google/uuid"
)

// Project is a user-created project from a template.
type Project struct {
	BaseModel
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	TemplateID string         `gorm:"type:varchar(32);not null"`
	Name       string         `gorm:"type:varchar(255);not null;default:''"`
	Config     map[string]any `gorm:"type:jsonb;not null;default:'{}'"`
	UpdatedAt  time.Time      `gorm:"not null;autoUpdateTime"`
}

func (Project) TableName() string { return "projects" }

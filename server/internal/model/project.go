package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// JSONMap is a custom type for JSONB columns.
type JSONMap map[string]any

func (j JSONMap) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(map[string]any)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		*j = make(map[string]any)
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// Project is a user-created project from a template.
type Project struct {
	BaseModel
	UserID     uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	TemplateID string    `gorm:"type:varchar(32);not null"`
	Name       string    `gorm:"type:varchar(255);not null;default:''"`
	Config     JSONMap   `gorm:"type:jsonb;not null;default:'{}'"`
	UpdatedAt  time.Time `gorm:"not null;autoUpdateTime"`
}

func (Project) TableName() string { return "projects" }

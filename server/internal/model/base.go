package model

import (
	"time"

	"github.com/google/uuid"
)

// BaseModel provides common fields for all models.
type BaseModel struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CreatedAt time.Time `gorm:"not null"`
}

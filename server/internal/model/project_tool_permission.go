package model

import (
	"github.com/google/uuid"
)

// ProjectToolPermission stores per-project tool permission whitelists.
type ProjectToolPermission struct {
	BaseModel
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	ToolName  string    `gorm:"type:varchar(128);not null;index:idx_perm_tool_action"`
	Action    string    `gorm:"type:varchar(64);not null;index:idx_perm_tool_action"`
	Pattern   string    `gorm:"type:text;not null"`
	GrantedBy uuid.UUID `gorm:"type:uuid;not null"`
	Level     string    `gorm:"type:varchar(16);not null;default:'exact'"` // "exact" | "wildcard"
}

func (ProjectToolPermission) TableName() string { return "project_tool_permissions" }

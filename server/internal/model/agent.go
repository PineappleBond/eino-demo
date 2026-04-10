package model

import (
	"github.com/google/uuid"
)

// Agent is an AI agent definition within a project.
type Agent struct {
	BaseModel
	ProjectID    uuid.UUID      `gorm:"type:uuid;not null;index"`
	AgentKey     string         `gorm:"type:varchar(64);not null"`
	AgentName    string         `gorm:"type:varchar(255);not null"`
	Description  string         `gorm:"type:text;not null;default:''"`
	Avatar       string         `gorm:"type:varchar(255);not null;default:''"`
	SystemPrompt string         `gorm:"type:text;not null;default:''"`
	Config       map[string]any `gorm:"type:jsonb;not null;default:'{}'"`
	SortOrder    int            `gorm:"not null;default:0"`
}

func (Agent) TableName() string { return "agents" }

// AgentRelationship defines parent-child links between agents.
type AgentRelationship struct {
	BaseModel
	ProjectID    uuid.UUID `gorm:"type:uuid;not null;index"`
	ParentID     uuid.UUID `gorm:"type:uuid;not null;index"`
	ChildID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Relationship string    `gorm:"type:varchar(64);not null;default:'delegates'"`
	Description  string    `gorm:"type:text;not null;default:''"`
	SortOrder    int       `gorm:"not null;default:0"`
}

func (AgentRelationship) TableName() string { return "agent_relationships" }

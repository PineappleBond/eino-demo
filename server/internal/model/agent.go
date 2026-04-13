package model

import (
	"github.com/google/uuid"
)

// Agent is an AI agent definition within a project.
type Agent struct {
	BaseModel
	ProjectID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_project_key;index:idx_agent_project;constraint:OnDelete:CASCADE"`
	AgentKey     string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_agent_project_key"`
	AgentName    string    `gorm:"type:varchar(255);not null"`
	Description  string    `gorm:"type:text;not null;default:''"`
	Avatar       string    `gorm:"type:varchar(255);not null;default:''"`
	SystemPrompt string    `gorm:"type:text;not null;default:''"`
	Config       JSONMap   `gorm:"type:jsonb;not null;default:'{}'"`
	SortOrder    int       `gorm:"not null;default:0"`
}

func (Agent) TableName() string { return "agents" }

// AgentRelationship defines parent-child links between agents.
type AgentRelationship struct {
	BaseModel
	ProjectID    uuid.UUID `gorm:"type:uuid;not null;index:idx_agent_rel_project;constraint:OnDelete:CASCADE"`
	ParentID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_rel_parent_child;index:idx_agent_rel_parent"`
	ChildID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_rel_parent_child;index:idx_agent_rel_child"`
	Relationship string    `gorm:"type:varchar(64);not null;default:'delegates'"`
	Description  string    `gorm:"type:text;not null;default:''"`
	SortOrder    int       `gorm:"not null;default:0"`
}

func (AgentRelationship) TableName() string { return "agent_relationships" }

package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/templates"
)

// TemplateService handles template listing and project creation.
type TemplateService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewTemplateService creates a TemplateService.
func NewTemplateService(db *gorm.DB, log *zap.Logger) *TemplateService {
	return &TemplateService{db: db, log: log}
}

// ListTemplates returns all template metadata.
func (s *TemplateService) ListTemplates() []templates.TemplateInfo {
	return templates.List()
}

// GetTemplate returns full template detail.
func (s *TemplateService) GetTemplate(id string) (templates.TemplateDetail, error) {
	t, ok := templates.Get(id)
	if !ok {
		return t, fmt.Errorf("template %q not found", id)
	}
	return t, nil
}

// CreateProjectFromTemplate creates a project with agents from a template.
func (s *TemplateService) CreateProjectFromTemplate(userID uuid.UUID, templateID string, name string, config map[string]any) (*model.Project, error) {
	t, ok := templates.Get(templateID)
	if !ok {
		return nil, fmt.Errorf("template %q not found", templateID)
	}

	if name == "" {
		name = t.Name
	}
	if config == nil {
		config = map[string]any{}
	}

	project := model.Project{
		UserID:     userID,
		TemplateID: templateID,
		Name:       name,
		Config:     model.JSONMap(config),
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		// Create agents from template
		agentIDMap := make(map[string]uuid.UUID) // agent_key → agent UUID
		for _, agentInfo := range t.Agents {
			agent := model.Agent{
				ProjectID:    project.ID,
				AgentKey:     agentInfo.AgentKey,
				AgentName:    agentInfo.AgentName,
				Description:  agentInfo.Description,
				SystemPrompt: agentInfo.SystemPrompt,
				Config:       model.JSONMap{"model_tier": "sonnet"},
			}
			if err := tx.Create(&agent).Error; err != nil {
				return err
			}
			agentIDMap[agentInfo.AgentKey] = agent.ID
		}

		_ = agentIDMap // Will be used for agent relationships in later templates

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &project, nil
}

// CompleteCreateProjectFromTemplate handles project creation from template with seq assignment and WS push.
func (s *TemplateService) CompleteCreateProjectFromTemplate(
	ctx context.Context,
	userID uuid.UUID,
	templateID string,
	name string,
	config map[string]any,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Project, error) {
	project, err := s.CreateProjectFromTemplate(userID, templateID, name, config)
	if err != nil {
		return nil, err
	}

	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	update := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "project.created",
		Payload: model.JSONMap{
			"id":          project.ID.String(),
			"template_id": templateID,
			"name":        project.Name,
			"seq":         seq,
		},
	}
	if err := s.db.Create(&update).Error; err != nil {
		return nil, err
	}

	pushUpdate(userID, update)
	return project, nil
}

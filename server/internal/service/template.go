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
		s.log.Warn("get template: not found", zap.String("template_id", id))
		return t, fmt.Errorf("template %q not found", id)
	}
	return t, nil
}

// CompleteCreateProjectFromTemplate handles project creation from template with seq assignment, user_update, and WS push.
// Wraps project + agents + user_update in a single transaction for atomicity.
func (s *TemplateService) CompleteCreateProjectFromTemplate(
	ctx context.Context,
	userID uuid.UUID,
	templateID string,
	name string,
	config map[string]any,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Project, error) {
	t, ok := templates.Get(templateID)
	if !ok {
		s.log.Warn("create project from template: template not found", zap.String("template_id", templateID))
		return nil, fmt.Errorf("template not found")
	}

	if name == "" {
		name = t.Name
	}
	if config == nil {
		config = map[string]any{}
	}

	// Allocate seq before transaction
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		s.log.Error("create project from template: seq assignment failed",
			zap.String("user_id", userID.String()),
			zap.String("template_id", templateID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var project *model.Project

	// Create project + agents + user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		p := model.Project{
			UserID:     userID,
			TemplateID: templateID,
			Name:       name,
			Config:     model.JSONMap(config),
		}
		if err := tx.Create(&p).Error; err != nil {
			return err
		}

		// Create agents from template
		for _, agentInfo := range t.Agents {
			agent := model.Agent{
				ProjectID:    p.ID,
				AgentKey:     agentInfo.AgentKey,
				AgentName:    agentInfo.AgentName,
				Description:  agentInfo.Description,
				SystemPrompt: agentInfo.SystemPrompt,
				Config:       model.JSONMap{"model_tier": "sonnet"},
			}
			if err := tx.Create(&agent).Error; err != nil {
				return err
			}
		}

		// Create user_update in the same transaction
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "project.created",
			Payload: model.JSONMap{
				"id":          p.ID.String(),
				"template_id": templateID,
				"name":        p.Name,
				"seq":         seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}

		project = &p
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("project created from template",
		zap.String("user_id", userID.String()),
		zap.String("template_id", templateID),
		zap.String("project_id", project.ID.String()),
	)
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "project.created",
		Payload: model.JSONMap{
			"id":          project.ID.String(),
			"template_id": templateID,
			"name":        project.Name,
			"seq":         seq,
		},
	})
	return project, nil
}

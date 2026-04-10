package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// ProjectService handles project CRUD logic.
type ProjectService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewProjectService creates a ProjectService.
func NewProjectService(db *gorm.DB, log *zap.Logger) *ProjectService {
	return &ProjectService{db: db, log: log}
}

// ListProjects returns all projects for a user.
func (s *ProjectService) ListProjects(userID uuid.UUID) ([]model.Project, error) {
	var projects []model.Project
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProject returns a single project by ID, scoped to user.
func (s *ProjectService) GetProject(userID, projectID uuid.UUID) (*model.Project, error) {
	var project model.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		return nil, fmt.Errorf("project not found")
	}
	return &project, nil
}

// UpdateProject updates a project's name and config.
func (s *ProjectService) UpdateProject(userID, projectID uuid.UUID, name string, config model.JSONMap) (*model.Project, error) {
	updates := map[string]any{}
	if name != "" {
		updates["name"] = name
	}
	if config != nil {
		updates["config"] = config
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	result := s.db.Model(&model.Project{}).Where("id = ? AND user_id = ?", projectID, userID).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("project not found")
	}
	return s.GetProject(userID, projectID)
}

// DeleteProject soft-deletes a project and all its children (CASCADE).
func (s *ProjectService) DeleteProject(userID, projectID uuid.UUID) error {
	result := s.db.Where("id = ? AND user_id = ?", projectID, userID).Delete(&model.Project{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

// CompleteDeleteProject handles project deletion with seq assignment, user_update, and WS push.
// Verifies ownership before seq allocation. Wraps delete + user_update in a single transaction.
func (s *ProjectService) CompleteDeleteProject(
	ctx context.Context,
	userID, projectID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify ownership first — before allocating seq
	var project model.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		return fmt.Errorf("project not found")
	}

	// 2. Allocate seq after ownership confirmed
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Delete project + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND user_id = ?", projectID, userID).Delete(&model.Project{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("project not found")
		}

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "project.deleted",
			Payload: model.JSONMap{
				"id":  projectID.String(),
				"seq": seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "project.deleted",
		Payload: model.JSONMap{
			"id":  projectID.String(),
			"seq": seq,
		},
	})
	return nil
}

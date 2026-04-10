package service

import (
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

package service

import (
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// SettingsService handles user settings logic.
type SettingsService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewSettingsService creates a SettingsService.
func NewSettingsService(db *gorm.DB, log *zap.Logger) *SettingsService {
	return &SettingsService{db: db, log: log}
}

// GetSettings returns user settings.
func (s *SettingsService) GetSettings(userID uuid.UUID) (*model.Settings, error) {
	var settings model.Settings
	if err := s.db.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return nil, err
	}
	return &settings, nil
}

// UpdateSettingsRequest holds the fields to update (zero values mean "don't change").
type UpdateSettingsRequest struct {
	ModelTier *string `json:"model_tier,omitempty"`
	Locale    *string `json:"locale,omitempty"`
	Theme     *string `json:"theme,omitempty"`
}

// UpdateSettings updates only the provided fields.
func (s *SettingsService) UpdateSettings(userID uuid.UUID, req UpdateSettingsRequest) (*model.Settings, error) {
	var settings model.Settings
	if err := s.db.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if req.ModelTier != nil {
		updates["model_tier"] = *req.ModelTier
	}
	if req.Locale != nil {
		updates["locale"] = *req.Locale
	}
	if req.Theme != nil {
		updates["theme"] = *req.Theme
	}
	updates["updated_at"] = time.Now()

	if len(updates) > 1 { // more than just updated_at
		s.db.Model(&settings).Updates(updates)
		s.db.Where("user_id = ?", userID).First(&settings)
	}

	return &settings, nil
}

package service

import (
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// UserService handles user-related business logic.
type UserService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewUserService creates a UserService.
func NewUserService(db *gorm.DB, log *zap.Logger) *UserService {
	return &UserService{db: db, log: log}
}

// GetMe returns the user profile with settings.
func (s *UserService) GetMe(userID uuid.UUID) (*model.User, *model.Settings, error) {
	var user model.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, nil, err
	}

	var settings model.Settings
	s.db.Where("user_id = ?", userID).FirstOrCreate(&settings, model.Settings{
		UserID:    userID,
		ModelTier: "sonnet",
		Locale:    "en",
		Theme:     "light",
	})

	return &user, &settings, nil
}

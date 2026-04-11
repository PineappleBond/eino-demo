package service

import (
	"context"
	"fmt"
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
	if err := s.db.Where("user_id = ?", userID).FirstOrCreate(&settings, model.Settings{
		UserID:    userID,
		ModelTier: "sonnet",
		Locale:    "en",
		Theme:     "light",
	}).Error; err != nil {
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

// validateSettingsRequest checks enum values for all settings fields.
func validateSettingsRequest(req UpdateSettingsRequest) error {
	if req.ModelTier != nil {
		switch *req.ModelTier {
		case "haiku", "sonnet", "opus":
		default:
			return fmt.Errorf("invalid model_tier: %q (must be haiku, sonnet, or opus)", *req.ModelTier)
		}
	}
	if req.Locale != nil {
		switch *req.Locale {
		case "en", "zh":
		default:
			return fmt.Errorf("invalid locale: %q (must be en or zh)", *req.Locale)
		}
	}
	if req.Theme != nil {
		switch *req.Theme {
		case "light", "dark":
		default:
			return fmt.Errorf("invalid theme: %q (must be light or dark)", *req.Theme)
		}
	}
	return nil
}

// UpdateSettings updates only the provided fields.
func (s *SettingsService) UpdateSettings(userID uuid.UUID, req UpdateSettingsRequest) (*model.Settings, error) {
	if err := validateSettingsRequest(req); err != nil {
		return nil, err
	}

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
		if err := s.db.Model(&settings).Updates(updates).Error; err != nil {
			return nil, err
		}
		s.db.Where("user_id = ?", userID).First(&settings)
	}

	return &settings, nil
}

// CompleteUpdateSettings handles settings update with seq assignment, user_update, and WS push.
// Wraps settings update + user_update in a single transaction for atomicity.
func (s *SettingsService) CompleteUpdateSettings(
	ctx context.Context,
	userID uuid.UUID,
	req UpdateSettingsRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*model.Settings, error) {
	// 1. Validate enum values
	if err := validateSettingsRequest(req); err != nil {
		return nil, err
	}

	// 2. Allocate seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var settings *model.Settings

	// 3. Update settings + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var s model.Settings
		if err := tx.Where("user_id = ?", userID).First(&s).Error; err != nil {
			return err
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
			if err := tx.Model(&s).Updates(updates).Error; err != nil {
				return err
			}
			tx.Where("user_id = ?", userID).First(&s)
		}

		payload := model.JSONMap{
			"seq":        seq,
			"model_tier": s.ModelTier,
			"locale":     s.Locale,
			"theme":      s.Theme,
			"changed":    make([]any, 0),
		}
		if req.ModelTier != nil {
			payload["changed"] = append(payload["changed"].([]any), "model_tier")
		}
		if req.Locale != nil {
			payload["changed"] = append(payload["changed"].([]any), "locale")
		}
		if req.Theme != nil {
			payload["changed"] = append(payload["changed"].([]any), "theme")
		}

		update := model.UserUpdate{
			UserID:  userID,
			Seq:     seq,
			Type:    "settings.changed",
			Payload: payload,
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}

		settings = &s
		return nil
	})
	if err != nil {
		return nil, err
	}

	pushUpdate(userID, model.UserUpdate{
		UserID:  userID,
		Seq:     seq,
		Type:    "settings.changed",
		Payload: makeUpdatePayloadForSettings(req, seq, settings),
	})
	return settings, nil
}

func makeUpdatePayloadForSettings(req UpdateSettingsRequest, seq int64, settings *model.Settings) model.JSONMap {
	payload := model.JSONMap{
		"seq":        seq,
		"model_tier": settings.ModelTier,
		"locale":     settings.Locale,
		"theme":      settings.Theme,
		"changed":    make([]any, 0),
	}
	if req.ModelTier != nil {
		payload["changed"] = append(payload["changed"].([]any), "model_tier")
	}
	if req.Locale != nil {
		payload["changed"] = append(payload["changed"].([]any), "locale")
	}
	if req.Theme != nil {
		payload["changed"] = append(payload["changed"].([]any), "theme")
	}
	return payload
}

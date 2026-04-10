// Package auth provides shared authentication utilities for demo-mode token resolution.
package auth

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// ResolveTokenToUser converts a demo-mode token to a user UUID, creating the user if needed.
// This is shared between the HTTP auth middleware and the WebSocket upgrade handler.
func ResolveTokenToUser(db *gorm.DB, rawToken string) (uuid.UUID, error) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return uuid.Nil, errors.New("empty token")
	}

	userID := uuid.NewSHA1(uuid.Nil, []byte(token))
	var user model.User
	result := db.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			user = model.User{Name: ""}
			if err := db.FirstOrCreate(&user, model.User{BaseModel: model.BaseModel{ID: userID}}).Error; err != nil {
				return uuid.Nil, err
			}
		} else {
			return uuid.Nil, result.Error
		}
	}
	return user.ID, nil
}

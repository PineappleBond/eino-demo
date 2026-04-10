package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
)

// RegisterUserRoutes registers GET /users/me.
func RegisterUserRoutes(api *gin.RouterGroup, svc *service.UserService) {
	api.GET("/users/me", func(c *gin.Context) {
		userID := getUserID(c)
		user, settings, err := svc.GetMe(userID)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to fetch user")
			return
		}
		respondJSON(c, 200, gin.H{
			"id":         user.ID,
			"name":       user.Name,
			"created_at": user.CreatedAt,
			"settings": gin.H{
				"model_tier": settings.ModelTier,
				"locale":     settings.Locale,
				"theme":      settings.Theme,
				"updated_at": settings.UpdatedAt,
			},
		})
	})
}

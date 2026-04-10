package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
)

// RegisterSettingsRoutes registers GET /settings and PUT /settings.
func RegisterSettingsRoutes(api *gin.RouterGroup, svc *service.SettingsService) {
	api.GET("/settings", func(c *gin.Context) {
		userID := getUserID(c)
		settings, err := svc.GetSettings(userID)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to fetch settings")
			return
		}
		respondJSON(c, 200, gin.H{
			"model_tier": settings.ModelTier,
			"locale":     settings.Locale,
			"theme":      settings.Theme,
			"updated_at": settings.UpdatedAt,
		})
	})

	api.PUT("/settings", func(c *gin.Context) {
		userID := getUserID(c)
		var req service.UpdateSettingsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		settings, err := svc.UpdateSettings(userID, req)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to update settings")
			return
		}
		respondJSON(c, 200, gin.H{
			"model_tier": settings.ModelTier,
			"locale":     settings.Locale,
			"theme":      settings.Theme,
			"updated_at": settings.UpdatedAt,
		})
	})
}

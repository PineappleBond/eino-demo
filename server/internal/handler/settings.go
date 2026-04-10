package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"
)

// RegisterSettingsRoutes registers GET /settings and PUT /settings.
func RegisterSettingsRoutes(api *gin.RouterGroup, svc *service.SettingsService, wsManager *ws.Manager) {
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
		settings, err := svc.CompleteUpdateSettings(
			c.Request.Context(),
			userID,
			req,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := ws.Update{
					Seq:     update.Seq,
					Type:    update.Type,
					Payload: update.Payload,
				}
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		)
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

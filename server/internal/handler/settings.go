package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/convert"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"
	"go.uber.org/zap"
)

// RegisterSettingsRoutes registers GET /settings and PUT /settings.
func RegisterSettingsRoutes(api *gin.RouterGroup, svc *service.SettingsService, wsManager *ws.Manager, log *zap.Logger) {
	api.GET("/settings", func(c *gin.Context) {
		userID := getUserID(c)
		settings, err := svc.GetSettings(userID)
		if err != nil {
			log.Error("get settings failed",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch settings")
			return
		}
		respondJSON(c, http.StatusOK, convert.ToSettings(*settings))
	})

	api.PUT("/settings", func(c *gin.Context) {
		userID := getUserID(c)
		var req types.PutSettingsJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn("update settings: invalid request body",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		svcReq := service.UpdateSettingsRequest{
			ModelTier: (*string)(req.ModelTier),
			Locale:    (*string)(req.Locale),
			Theme:     (*string)(req.Theme),
		}
		settings, err := svc.CompleteUpdateSettings(
			c.Request.Context(),
			userID,
			svcReq,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		)
		if err != nil {
			log.Error("update settings failed",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update settings")
			return
		}
		log.Info("settings updated",
			zap.String("user_id", userID.String()),
		)
		respondJSON(c, http.StatusOK, convert.ToSettings(*settings))
	})
}

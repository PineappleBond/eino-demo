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

// RegisterTemplateRoutes registers template endpoints.
func RegisterTemplateRoutes(api *gin.RouterGroup, svc *service.TemplateService, wsManager *ws.Manager, log *zap.Logger) {
	api.GET("/templates", func(c *gin.Context) {
		list := svc.ListTemplates()
		log.Debug("list templates", zap.Int("count", len(list)))
		result := make([]types.Template, len(list))
		for i, t := range list {
			result[i] = convert.ToTemplate(t)
		}
		respondJSON(c, http.StatusOK, result)
	})

	api.GET("/templates/:id", func(c *gin.Context) {
		id := c.Param("id")
		t, err := svc.GetTemplate(id)
		if err != nil {
			log.Warn("get template not found", zap.String("template_id", id))
			respondError(c, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		respondJSON(c, http.StatusOK, convert.ToTemplate(t.TemplateInfo))
	})

	api.POST("/templates/:id/projects", func(c *gin.Context) {
		userID := getUserID(c)
		templateID := c.Param("id")

		var req struct {
			Name   string         `json:"name"`
			Config map[string]any `json:"config"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn("create project from template: invalid request body",
				zap.String("template_id", templateID),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		cfg := model.JSONMap(req.Config)
		if cfg == nil {
			cfg = make(map[string]any)
		}

		project, err := svc.CompleteCreateProjectFromTemplate(
			c.Request.Context(),
			userID,
			templateID,
			req.Name,
			cfg,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		)
		if err != nil {
			log.Error("create project from template failed",
				zap.String("user_id", userID.String()),
				zap.String("template_id", templateID),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to create project from template")
			return
		}
		log.Info("project created from template",
			zap.String("user_id", userID.String()),
			zap.String("template_id", templateID),
			zap.String("project_id", project.ID.String()),
		)
		respondJSON(c, http.StatusCreated, convert.ToProject(*project))
	})
}

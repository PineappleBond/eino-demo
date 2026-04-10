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
)

// RegisterTemplateRoutes registers template endpoints.
func RegisterTemplateRoutes(api *gin.RouterGroup, svc *service.TemplateService, wsManager *ws.Manager) {
	api.GET("/templates", func(c *gin.Context) {
		list := svc.ListTemplates()
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
				wsUpdate := ws.Update{
					Seq:     update.Seq,
					Type:    update.Type,
					Payload: update.Payload,
				}
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		)
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to create project from template")
			return
		}
		respondJSON(c, http.StatusCreated, convert.ToProject(*project))
	})
}

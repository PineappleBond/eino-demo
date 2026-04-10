package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"
)

// RegisterTemplateRoutes registers template endpoints.
func RegisterTemplateRoutes(api *gin.RouterGroup, svc *service.TemplateService, wsManager *ws.Manager) {
	api.GET("/templates", func(c *gin.Context) {
		list := svc.ListTemplates()
		respondJSON(c, http.StatusOK, list)
	})

	api.GET("/templates/:id", func(c *gin.Context) {
		id := c.Param("id")
		t, err := svc.GetTemplate(id)
		if err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		respondJSON(c, http.StatusOK, t)
	})

	api.POST("/templates/:id/projects", func(c *gin.Context) {
		userID := getUserID(c)
		templateID := c.Param("id")

		var req struct {
			Name   string         `json:"name"`
			Config map[string]any `json:"config"`
		}
		c.ShouldBindJSON(&req)

		project, err := svc.CompleteCreateProjectFromTemplate(
			c.Request.Context(),
			userID,
			templateID,
			req.Name,
			req.Config,
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
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		respondJSON(c, http.StatusCreated, gin.H{
			"id":          project.ID,
			"user_id":     project.UserID,
			"template_id": project.TemplateID,
			"name":        project.Name,
			"config":      project.Config,
			"created_at":  project.CreatedAt,
		})
	})
}

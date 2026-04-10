package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
)

// RegisterTemplateRoutes registers template endpoints.
func RegisterTemplateRoutes(api *gin.RouterGroup, svc *service.TemplateService) {
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

		project, err := svc.CreateProjectFromTemplate(userID, templateID, req.Name, req.Config)
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
			"updated_at":  project.UpdatedAt,
		})
	})
}

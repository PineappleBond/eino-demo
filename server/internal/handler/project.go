package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
)

// RegisterProjectRoutes registers project endpoints.
func RegisterProjectRoutes(api *gin.RouterGroup, svc *service.ProjectService) {
	api.GET("/projects", func(c *gin.Context) {
		userID := getUserID(c)
		projects, err := svc.ListProjects(userID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list projects")
			return
		}
		result := make([]gin.H, len(projects))
		for i, p := range projects {
			result[i] = gin.H{
				"id":          p.ID,
				"user_id":     p.UserID,
				"template_id": p.TemplateID,
				"name":        p.Name,
				"config":      p.Config,
				"created_at":  p.CreatedAt,
				"updated_at":  p.UpdatedAt,
			}
		}
		respondJSON(c, http.StatusOK, result)
	})

	api.GET("/projects/:id", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		project, err := svc.GetProject(userID, projectID)
		if err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		respondJSON(c, http.StatusOK, gin.H{
			"id":          project.ID,
			"user_id":     project.UserID,
			"template_id": project.TemplateID,
			"name":        project.Name,
			"config":      project.Config,
			"created_at":  project.CreatedAt,
			"updated_at":  project.UpdatedAt,
		})
	})

	api.DELETE("/projects/:id", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		if err := svc.DeleteProject(userID, projectID); err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		c.JSON(http.StatusNoContent, nil)
	})
}

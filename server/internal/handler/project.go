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

// RegisterProjectRoutes registers project endpoints.
func RegisterProjectRoutes(api *gin.RouterGroup, svc *service.ProjectService, wsManager *ws.Manager, log *zap.Logger) {
	api.GET("/projects", func(c *gin.Context) {
		userID := getUserID(c)
		projects, err := svc.ListProjects(userID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list projects")
			return
		}
		result := make([]types.Project, len(projects))
		for i, p := range projects {
			result[i] = convert.ToProject(p)
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
			respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
			return
		}
		respondJSON(c, http.StatusOK, convert.ToProject(*project))
	})

	api.PUT("/projects/:id", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		var req types.PutProjectsIdJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		var name string
		if req.Name != nil {
			name = *req.Name
		}
		var config model.JSONMap
		if req.Config != nil {
			config = *req.Config
		}
		project, err := svc.UpdateProject(userID, projectID, name, config)
		if err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
			return
		}
		respondJSON(c, http.StatusOK, convert.ToProject(*project))
	})

	api.DELETE("/projects/:id", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		if err := svc.CompleteDeleteProject(
			c.Request.Context(),
			userID,
			projectID,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		); err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
			return
		}
		c.JSON(http.StatusNoContent, nil)
	})
}

package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"
)

// RegisterConversationRoutes registers conversation endpoints.
func RegisterConversationRoutes(
	api *gin.RouterGroup,
	svc *service.ConversationService,
	wsManager *ws.Manager,
) {
	api.GET("/projects/:id/conversations", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		conversations, err := svc.ListConversations(userID, projectID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list conversations")
			return
		}
		result := make([]gin.H, len(conversations))
		for i, conv := range conversations {
			result[i] = gin.H{
				"id":               conv.ID,
				"project_id":       conv.ProjectID,
				"user_id":          conv.UserID,
				"title":            conv.Title,
				"summary":          conv.Summary,
				"status":           conv.Status,
				"last_preview":     conv.LastMessagePreview,
				"message_count":    conv.MessageCount,
				"latest_seq":       conv.LatestMessageSeq,
				"member_count":     conv.MemberCount,
				"token_prompt":     conv.TokenPrompt,
				"token_completion": conv.TokenCompletion,
				"created_at":       conv.CreatedAt,
				"updated_at":       conv.UpdatedAt,
			}
		}
		respondJSON(c, http.StatusOK, result)
	})

	api.POST("/projects/:id/conversations", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		var req service.CreateConversationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			// Allow empty body — use defaults
			req = service.CreateConversationRequest{}
		}
		conv, err := svc.CompleteCreateConversation(
			c.Request.Context(),
			userID,
			projectID,
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
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		respondJSON(c, http.StatusCreated, gin.H{
			"id":         conv.ID,
			"project_id": conv.ProjectID,
			"user_id":    conv.UserID,
			"title":      conv.Title,
			"status":     conv.Status,
			"created_at": conv.CreatedAt,
			"updated_at": conv.UpdatedAt,
		})
	})

	api.DELETE("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		if err := svc.CompleteDeleteConversation(
			c.Request.Context(),
			userID,
			conversationID,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := ws.Update{
					Seq:     update.Seq,
					Type:    update.Type,
					Payload: update.Payload,
				}
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		); err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		c.JSON(http.StatusNoContent, nil)
	})
}

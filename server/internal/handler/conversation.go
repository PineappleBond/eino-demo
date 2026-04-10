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
		result := make([]types.Conversation, len(conversations))
		for i, conv := range conversations {
			result[i] = convert.ToConversation(conv)
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
		var req types.PostProjectsIdConversationsJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			// Allow empty body — use defaults
			req = types.PostProjectsIdConversationsJSONBody{}
		}
		svcReq := service.CreateConversationRequest{
			Title: valueOrZero(req.Title),
		}
		conv, err := svc.CompleteCreateConversation(
			c.Request.Context(),
			userID,
			projectID,
			svcReq,
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
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to create conversation")
			return
		}
		respondJSON(c, http.StatusCreated, convert.ToConversation(*conv))
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
			respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			return
		}
		c.JSON(http.StatusNoContent, nil)
	})

	// List conversation members
	api.GET("/conversations/:id/members", func(c *gin.Context) {
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		members, err := svc.ListMembers(conversationID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list members")
			return
		}
		result := make([]gin.H, len(members))
		for i, m := range members {
			result[i] = gin.H{
				"id":              m.ID,
				"conversation_id": m.ConversationID,
				"member_type":     m.MemberType,
				"member_id":       m.MemberID,
				"member_name":     m.MemberName,
				"is_owner":        m.IsOwner,
			}
		}
		respondJSON(c, http.StatusOK, result)
	})
}

func valueOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

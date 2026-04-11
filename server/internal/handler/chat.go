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

// RegisterChatRoutes registers chat endpoints.
func RegisterChatRoutes(
	api *gin.RouterGroup,
	chatSvc *service.ChatService,
	wsManager *ws.Manager,
) {
	api.POST("/conversations/:id/messages", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var req types.PostConversationsIdMessagesJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		svcReq := service.SendMessageRequest{
			Content: req.Content,
		}
		resp, err := chatSvc.CompleteSendMessage(
			c.Request.Context(),
			userID,
			conversationID,
			svcReq,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		)
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to send message")
			return
		}

		respondJSON(c, http.StatusOK, convert.MessageSendResponse{
			ConversationID: conversationID,
			MessageID:      resp.MessageID,
			Seq:            resp.Seq,
		})
	})

	api.POST("/conversations/:id/stop", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		if err := chatSvc.StopMessage(
			c.Request.Context(),
			userID,
			conversationID,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		); err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "stopped"})
	})

	api.GET("/conversations/:id/messages", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var req service.GetConversationMessagesRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid query params")
			return
		}

		messages, err := chatSvc.GetConversationMessages(userID, conversationID, req)
		if err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			return
		}

		result := make([]types.Message, len(messages))
		for i, msg := range messages {
			result[i] = convert.ToMessage(msg)
		}

		respondJSON(c, http.StatusOK, result)
	})
}

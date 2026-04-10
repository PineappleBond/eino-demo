package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
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

		var req service.SendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		resp, err := chatSvc.CompleteSendMessage(
			c.Request.Context(),
			userID,
			conversationID,
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

		respondJSON(c, http.StatusOK, gin.H{
			"conversation_id": resp.ConversationID,
			"message_id":      resp.MessageID,
			"seq":             resp.Seq,
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
			respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}

		result := make([]gin.H, len(messages))
		for i, msg := range messages {
			item := gin.H{
				"id":                msg.ID,
				"conversation_id":   msg.ConversationID,
				"seq":               msg.Seq,
				"sender_role":       msg.SenderRole,
				"sender_id":         msg.SenderID,
				"content":           msg.Content,
				"reason_content":    msg.ReasonContent,
				"metadata":          msg.Metadata,
				"token_prompt":      msg.TokenPrompt,
				"token_completion":  msg.TokenCompletion,
				"created_at":        msg.CreatedAt,
			}
			if msg.ReplyToSeq != nil {
				item["reply_to_seq"] = *msg.ReplyToSeq
			}
			if msg.FinishReason != nil {
				item["finish_reason"] = *msg.FinishReason
			}
			if msg.ErrorMessage != nil {
				item["error_message"] = *msg.ErrorMessage
			}
			if msg.DurationMs != nil {
				item["duration_ms"] = *msg.DurationMs
			}
			result[i] = item
		}

		respondJSON(c, http.StatusOK, result)
	})
}

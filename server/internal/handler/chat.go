package handler

import (
	"errors"
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

	api.POST("/conversations/:id/answer", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var req types.PostConversationsIdAnswerJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		if err := chatSvc.AnswerQuestion(
			c.Request.Context(),
			userID,
			conversationID,
			service.AnswerQuestionRequest{
				CheckpointID: req.CheckpointId,
				InterruptID:  req.InterruptId,
				Answer:       req.Answer,
			},
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		); err != nil {
			if errors.Is(err, service.ErrConversationNotFound) || errors.Is(err, service.ErrHITLNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "resumed"})
	})

	api.GET("/conversations/:id/hitl", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		hitls, err := chatSvc.ListPendingHITL(userID, conversationID)
		if err != nil {
			if errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list pending HITL")
			}
			return
		}

		result := make([]types.HumanInTheLoop, len(hitls))
		for i, h := range hitls {
			result[i] = convert.ToHumanInTheLoop(h)
		}

		respondJSON(c, http.StatusOK, result)
	})

	api.GET("/conversations/:id/permissions", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var params types.GetConversationsIdPermissionsParams
		if err := c.ShouldBindQuery(&params); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid query params")
			return
		}

		status := ""
		if params.Status != nil {
			status = string(*params.Status)
		}

		perms, err := chatSvc.ListPendingPermissions(userID, conversationID, status)
		if err != nil {
			if errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list permissions")
			}
			return
		}

		result := make([]types.HumanInPermission, len(perms))
		for i, p := range perms {
			result[i] = convert.ToHumanInPermission(p)
		}

		respondJSON(c, http.StatusOK, result)
	})

	api.POST("/conversations/:id/permissions/:permId/answer", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var req types.PostConversationsIdPermissionsPermIdAnswerJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		permId, err := uuid.Parse(c.Param("permId"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid permission ID")
			return
		}

		if err := chatSvc.AnswerPermission(
			c.Request.Context(),
			userID,
			conversationID,
			permId,
			service.AnswerPermissionRequest{
				CheckpointID: req.CheckpointId,
				InterruptID:  req.InterruptId,
				Decision:     string(req.Decision),
			},
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		); err != nil {
			if errors.Is(err, service.ErrConversationNotFound) || errors.Is(err, service.ErrPermissionNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "resumed"})
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

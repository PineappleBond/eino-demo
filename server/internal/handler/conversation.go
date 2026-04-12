package handler

import (
	"errors"
	"io"
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

// RegisterConversationRoutes registers conversation endpoints.
func RegisterConversationRoutes(
	api *gin.RouterGroup,
	svc *service.ConversationService,
	chatSvc *service.ChatService,
	wsManager *ws.Manager,
	log *zap.Logger,
) {
	api.GET("/projects/:id/conversations", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("list conversations: invalid project ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		conversations, err := svc.ListConversations(userID, projectID)
		if err != nil {
			log.Error("list conversations failed",
				zap.String("user_id", userID.String()),
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
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
			log.Warn("create conversation: invalid project ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}
		var req types.PostProjectsIdConversationsJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			if err == io.EOF {
				// Empty body — use defaults
				req = types.PostProjectsIdConversationsJSONBody{}
			} else {
				log.Warn("create conversation: invalid request body",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
				return
			}
		}
		svcReq := service.CreateConversationRequest{
			Title: valueOrZero(req.Title),
			Mode:  string(valueOrZero(req.Mode)),
		}
		conv, err := svc.CompleteCreateConversation(
			c.Request.Context(),
			userID,
			projectID,
			svcReq,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		)
		if err != nil {
			log.Error("create conversation failed",
				zap.String("user_id", userID.String()),
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to create conversation")
			return
		}
		log.Info("conversation created",
			zap.String("user_id", userID.String()),
			zap.String("project_id", projectID.String()),
			zap.String("conv_id", conv.ID.String()),
		)
		respondJSON(c, http.StatusCreated, convert.ToConversation(*conv))
	})

	api.DELETE("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("delete conversation: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		if err := svc.CompleteDeleteConversation(
			c.Request.Context(),
			userID,
			conversationID,
			wsManager.NextSeq,
			func(userID uuid.UUID, update model.UserUpdate) {
				wsUpdate := convert.ToUpdate(update)
				wsManager.PushToUserConnections(userID, wsUpdate)
			},
		); err != nil {
			log.Error("delete conversation failed",
				zap.String("user_id", userID.String()),
				zap.String("conv_id", conversationID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			return
		}
		log.Info("conversation deleted",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
		)
		c.JSON(http.StatusNoContent, nil)
	})

	// Update conversation (title and/or mode)
	api.PATCH("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("update conversation: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		var req types.PatchConversationsIdJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn("update conversation: invalid request body",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
			return
		}

		updateTitle := req.Title != nil && *req.Title != ""
		updateMode := req.Mode != nil && *req.Mode != ""

		if !updateTitle && !updateMode {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "at least one of title or mode must be provided")
			return
		}

		var conv *model.Conversation

		if updateTitle {
			svcReq := service.RenameConversationRequest{
				Title: *req.Title,
			}
			conv, err = svc.CompleteRenameConversation(
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
				log.Error("rename conversation failed",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
				return
			}
		}

		if updateMode {
			svcReq := service.UpdateConversationModeRequest{
				Mode: string(*req.Mode),
			}
			modeConv, modeErr := svc.CompleteUpdateMode(
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
			if modeErr != nil {
				log.Warn("update conversation mode failed",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
					zap.Error(modeErr),
				)
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to update mode: "+modeErr.Error())
				return
			}
			conv = modeConv
		}

		if conv != nil {
			log.Info("conversation updated",
				zap.String("user_id", userID.String()),
				zap.String("conv_id", conversationID.String()),
			)
			respondJSON(c, http.StatusOK, convert.ToConversation(*conv))
		} else {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "no changes to apply")
		}
	})

	// Update conversation status (e.g., archive)
	api.PUT("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("update conversation status: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		var req types.PutConversationsIdJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn("update conversation status: invalid request body",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
			return
		}
		svcReq := service.UpdateConversationStatusRequest{
			Status: req.Status,
		}
		conv, err := svc.UpdateStatus(
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
			log.Error("update conversation status failed",
				zap.String("user_id", userID.String()),
				zap.String("conv_id", conversationID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			return
		}
		log.Info("conversation status updated",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
			zap.String("status", conv.Status),
		)
		respondJSON(c, http.StatusOK, convert.ToConversation(*conv))
	})

	// List conversation members
	api.GET("/conversations/:id/members", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("list members: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		members, err := svc.ListMembers(userID, conversationID)
		if err != nil {
			log.Error("list members failed",
				zap.String("user_id", userID.String()),
				zap.String("conv_id", conversationID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list members")
			return
		}
		result := make([]types.Member, len(members))
		for i, m := range members {
			result[i] = convert.ToMember(m)
		}
		respondJSON(c, http.StatusOK, result)
	})

	// Compact conversation — starts async compaction
	api.POST("/conversations/:id/compact", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("compact conversation: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		if err := chatSvc.StartCompaction(c.Request.Context(), userID, conversationID, wsManager.NextSeq, func(userID uuid.UUID, update model.UserUpdate) {
			wsUpdate := convert.ToUpdate(update)
			wsManager.PushToUserConnections(userID, wsUpdate)
		}); err != nil {
			if errors.Is(err, service.ErrConversationNotFound) {
				log.Warn("compact conversation: not found",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
				)
				respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			} else {
				log.Error("compact conversation failed",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}
		log.Info("conversation compaction started",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
		)
		respondJSON(c, http.StatusOK, gin.H{
			"conversation_id": conversationID.String(),
			"status":          "compacting",
		})
	})
}

func valueOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

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
		conversations, err := svc.ListConversationsTree(userID, projectID)
		if err != nil {
			log.Error("list conversations tree failed",
				zap.String("user_id", userID.String()),
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list conversations")
			return
		}
		respondJSON(c, http.StatusOK, conversations)
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
			Title:                valueOrZero(req.Title),
			Mode:                 string(valueOrZero(req.Mode)),
			ParentConversationID: req.ParentConversationId,
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

	api.GET("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("get conversation: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		conv, err := svc.GetConversation(userID, conversationID)
		if err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			return
		}
		respondJSON(c, http.StatusOK, convert.ToConversation(*conv))
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

	// Branch conversation — create a new conversation from messages up to input_seq
	api.POST("/conversations/:id/branch", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("branch conversation: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}
		var req types.PostConversationsIdBranchJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn("branch conversation: invalid request body",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
			return
		}
		svcReq := service.BranchConversationRequest{
			InputSeq: int64(req.InputSeq),
		}
		newConv, err := svc.BranchConversation(
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
			log.Error("branch conversation failed",
				zap.String("user_id", userID.String()),
				zap.String("conv_id", conversationID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		log.Info("conversation branched",
			zap.String("user_id", userID.String()),
			zap.String("conv_id", conversationID.String()),
			zap.String("new_conv_id", newConv.ID.String()),
		)
		respondJSON(c, http.StatusOK, gin.H{
			"id":    newConv.ID.String(),
			"title": newConv.Title,
			"mode":  newConv.Mode,
		})
	})

	// List pending interrupts from sub-conversations
	api.GET("/conversations/:id/sub-interrupts", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("list sub-interrupts: invalid conversation ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		result, err := svc.GetSubInterrupts(c.Request.Context(), userID, conversationID)
		if err != nil {
			if errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			} else {
				log.Error("list sub-interrupts failed",
					zap.String("user_id", userID.String()),
					zap.String("conv_id", conversationID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list sub-interrupts")
			}
			return
		}

		type subPermResponse struct {
			ID             string `json:"id"`
			ConversationID string `json:"conversation_id"`
			ToolName       string `json:"tool_name"`
			Action         string `json:"action"`
			Content        string `json:"content"`
			ToolDesc       string `json:"tool_desc,omitempty"`
			ArgsSummary    string `json:"args_summary,omitempty"`
			SafetyLevel    int    `json:"safety_level"`
			SafetyReason   string `json:"safety_reason"`
			CheckpointID   string `json:"checkpoint_id,omitempty"`
			InterruptID    string `json:"interrupt_id,omitempty"`
		}

		type subHitlResponse struct {
			ID             string `json:"id"`
			ConversationID string `json:"conversation_id"`
			Question       string `json:"question"`
			Choices        any    `json:"choices"`
			AnswerType     string `json:"answer_type"`
			CheckpointID   string `json:"checkpoint_id,omitempty"`
			InterruptID    string `json:"interrupt_id,omitempty"`
		}

		perms := make([]subPermResponse, len(result.Permissions))
		for i, p := range result.Permissions {
			perms[i] = subPermResponse{
				ID:             p.ID.String(),
				ConversationID: p.ConversationID.String(),
				ToolName:       p.ToolName,
				Action:         p.Action,
				Content:        p.Content,
				ToolDesc:       p.ToolDesc,
				ArgsSummary:    p.ArgsSummary,
				SafetyLevel:    p.SafetyLevel,
				SafetyReason:   p.SafetyReason,
				CheckpointID:   p.CheckpointID,
				InterruptID:    p.InterruptID,
			}
		}

		hitls := make([]subHitlResponse, len(result.HITLs))
		for i, h := range result.HITLs {
			hitls[i] = subHitlResponse{
				ID:             h.ID.String(),
				ConversationID: h.ConversationID.String(),
				Question:       h.Question,
				Choices:        h.Choices,
				AnswerType:     h.AnswerType,
				CheckpointID:   h.CheckpointID,
				InterruptID:    h.InterruptID,
			}
		}

		respondJSON(c, http.StatusOK, gin.H{
			"permissions": perms,
			"hitls":       hitls,
		})
	})

	// List pending permissions across all conversations in a project.
	api.GET("/projects/:id/permissions", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("list project permissions: invalid project ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
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

		perms, err := chatSvc.ListProjectPermissions(userID, projectID, status)
		if err != nil {
			if errors.Is(err, service.ErrProjectNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
			} else {
				log.Error("list project permissions failed",
					zap.String("user_id", userID.String()),
					zap.String("project_id", projectID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list project permissions")
			}
			return
		}

		result := make([]types.HumanInPermission, len(perms))
		for i, p := range perms {
			result[i] = convert.ToHumanInPermission(p)
		}

		respondJSON(c, http.StatusOK, result)
	})

	// List pending HITLs across all conversations in a project.
	api.GET("/projects/:id/hitls", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			log.Warn("list project HITLs: invalid project ID", zap.String("id", c.Param("id")))
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
			return
		}

		var params types.GetProjectsIdHitlsParams
		if err := c.ShouldBindQuery(&params); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid query params")
			return
		}

		status := ""
		if params.Status != nil {
			status = string(*params.Status)
		}

		hitls, err := chatSvc.ListProjectHITLs(userID, projectID, status)
		if err != nil {
			if errors.Is(err, service.ErrProjectNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
			} else {
				log.Error("list project HITLs failed",
					zap.String("user_id", userID.String()),
					zap.String("project_id", projectID.String()),
					zap.Error(err),
				)
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list project HITLs")
			}
			return
		}

		result := make([]types.HumanInTheLoop, len(hitls))
		for i, h := range hitls {
			result[i] = convert.ToHumanInTheLoop(h)
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

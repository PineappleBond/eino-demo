package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/convert"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"
)

// RegisterCronTaskRoutes registers cron task HTTP endpoints.
func RegisterCronTaskRoutes(
	api *gin.RouterGroup,
	cronSvc *service.CronService,
	wsManager *ws.Manager,
	db *gorm.DB,
) {
	// pushCronTaskSync persists a cron_task.sync update and pushes it to the user.
	pushCronTaskSync := func(ctx context.Context, userID uuid.UUID, conversationID string) {
		seq, err := wsManager.NextSeq(ctx, userID)
		if err != nil {
			return
		}
		payload := model.JSONMap{
			"conversation_id": conversationID,
			"seq":             seq,
		}
		update := model.UserUpdate{
			UserID:  userID,
			Seq:     seq,
			Type:    "cron_task.sync",
			Payload: payload,
		}
		if err := db.WithContext(ctx).Create(&update).Error; err != nil {
			return
		}
		wsManager.PushToUserConnections(userID, convert.ToUpdate(update))
	}

	// GET /conversations/:id/cron-tasks
	api.GET("/conversations/:id/cron-tasks", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		tasks, err := cronSvc.ListTasks(c.Request.Context(), userID, conversationID)
		if err != nil {
			if err.Error() == "conversation not found" {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list cron tasks")
			}
			return
		}

		result := make([]types.CronTask, len(tasks))
		for i, t := range tasks {
			result[i] = convert.ToCronTask(t)
		}
		c.JSON(http.StatusOK, result)
	})

	// POST /conversations/:id/cron-tasks
	api.POST("/conversations/:id/cron-tasks", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var req types.PostConversationsIdCronTasksJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		senderRole := types.CronTaskSenderRole("user")
		if req.SenderRole != nil {
			senderRole = types.CronTaskSenderRole(*req.SenderRole)
		}

		task, err := cronSvc.CreateTask(c.Request.Context(), userID, conversationID, req.Content, req.Schedule, senderRole)
		if err != nil {
			if err.Error() == "conversation not found" {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}

		pushCronTaskSync(c.Request.Context(), userID, conversationID.String())
		c.JSON(http.StatusCreated, convert.ToCronTask(*task))
	})

	// POST /conversations/:id/cron-tasks/:taskId/cancel
	api.POST("/conversations/:id/cron-tasks/:taskId/cancel", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		taskID, err := uuid.Parse(c.Param("taskId"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid task ID")
			return
		}

		if err := cronSvc.CancelTask(c.Request.Context(), userID, conversationID, taskID); err != nil {
			if err.Error() == "conversation not found" || err.Error() == "cron task not found" {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}

		pushCronTaskSync(c.Request.Context(), userID, conversationID.String())
		c.JSON(http.StatusNoContent, nil)
	})
}

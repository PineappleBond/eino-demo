package handler

import (
	"context"
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
	"gorm.io/gorm"
)

// RegisterTodoRoutes registers todo-related HTTP endpoints.
func RegisterTodoRoutes(
	api *gin.RouterGroup,
	todoSvc *service.TodoService,
	wsManager *ws.Manager,
	db *gorm.DB,
) {
	// pushTodoSync persists a todo.sync update and pushes it to the user.
	pushTodoSync := func(ctx context.Context, userID uuid.UUID, conversationID string) {
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
			Type:    "todo.sync",
			Payload: payload,
		}
		if err := db.WithContext(ctx).Create(&update).Error; err != nil {
			return
		}
		wsManager.PushToUserConnections(userID, convert.ToUpdate(update))
	}

	// GET /conversations/:id/todos
	api.GET("/conversations/:id/todos", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		todos, err := todoSvc.ListTodos(userID, conversationID)
		if err != nil {
			if errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list todos")
			}
			return
		}

		result := make([]types.Todo, len(todos))
		for i, t := range todos {
			result[i] = convert.ToTodo(t)
		}
		c.JSON(http.StatusOK, result)
	})

	// POST /conversations/:id/todos
	api.POST("/conversations/:id/todos", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var req types.PostConversationsIdTodosJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		todo, err := todoSvc.CreateTodo(userID, conversationID, req)
		if err != nil {
			if errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}

		pushTodoSync(c.Request.Context(), userID, conversationID.String())
		c.JSON(http.StatusCreated, convert.ToTodo(*todo))
	})

	// PATCH /todos/:id
	api.PATCH("/todos/:id", func(c *gin.Context) {
		userID := getUserID(c)
		todoID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid todo ID")
			return
		}

		var req types.PatchTodosIdJSONBody
		if err := c.ShouldBindJSON(&req); err != nil {
			if err == io.EOF {
				req = types.PatchTodosIdJSONBody{}
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
				return
			}
		}

		todo, err := todoSvc.UpdateTodoByTodoID(userID, todoID, req)
		if err != nil {
			if errors.Is(err, service.ErrTodoNotFound) || errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}

		pushTodoSync(c.Request.Context(), userID, todo.ConversationID.String())
		c.JSON(http.StatusOK, convert.ToTodo(*todo))
	})

	// DELETE /todos/:id
	api.DELETE("/todos/:id", func(c *gin.Context) {
		userID := getUserID(c)
		todoID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid todo ID")
			return
		}

		// Look up conversationID before delete for pushTodoSync.
		var todo model.Todo
		if err := db.Where("id = ?", todoID).First(&todo).Error; err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "todo not found")
			return
		}

		if err := todoSvc.DeleteTodoByTodoID(userID, todoID); err != nil {
			if errors.Is(err, service.ErrTodoNotFound) || errors.Is(err, service.ErrConversationNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			} else {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			}
			return
		}

		pushTodoSync(c.Request.Context(), userID, todo.ConversationID.String())
		c.JSON(http.StatusNoContent, nil)
	})
}

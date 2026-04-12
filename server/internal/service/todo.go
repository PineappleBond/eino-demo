package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
)

// TodoService handles todo CRUD operations.
type TodoService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewTodoService creates a TodoService.
func NewTodoService(db *gorm.DB, log *zap.Logger) *TodoService {
	return &TodoService{db: db, log: log}
}

// ListTodos returns all todos for a conversation.
func (s *TodoService) ListTodos(userID, conversationID uuid.UUID) ([]model.Todo, error) {
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	var todos []model.Todo
	if err := s.db.Where("conversation_id = ?", conversationID).Order("created_at ASC").Find(&todos).Error; err != nil {
		return nil, fmt.Errorf("failed to list todos: %w", err)
	}
	return todos, nil
}

// CreateTodo creates a new todo item.
func (s *TodoService) CreateTodo(userID, conversationID uuid.UUID, input types.PostConversationsIdTodosJSONBody) (*model.Todo, error) {
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	todo := model.Todo{
		ConversationID: conversationID,
		Content:        input.Content,
		Metadata:       model.JSONMap{},
	}
	if err := s.db.Create(&todo).Error; err != nil {
		return nil, fmt.Errorf("failed to create todo: %w", err)
	}
	return &todo, nil
}

// UpdateTodo updates a todo item.
func (s *TodoService) UpdateTodo(userID, conversationID, todoID uuid.UUID, input types.PatchTodosIdJSONBody) (*model.Todo, error) {
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	var todo model.Todo
	if err := s.db.Where("id = ? AND conversation_id = ?", todoID, conversationID).First(&todo).Error; err != nil {
		return nil, fmt.Errorf("todo not found")
	}

	updates := map[string]interface{}{}
	if input.Content != nil {
		updates["content"] = *input.Content
	}
	if input.Completed != nil {
		updates["completed"] = *input.Completed
	}
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := s.db.Model(&todo).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update todo: %w", err)
		}
	}

	// Reload
	if err := s.db.Where("id = ?", todoID).First(&todo).Error; err != nil {
		return nil, fmt.Errorf("failed to reload todo: %w", err)
	}
	return &todo, nil
}

// DeleteTodo deletes a todo item.
func (s *TodoService) DeleteTodo(userID, conversationID, todoID uuid.UUID) error {
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	result := s.db.Where("id = ? AND conversation_id = ?", todoID, conversationID).Delete(&model.Todo{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete todo: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("todo not found")
	}
	return nil
}

// UpdateTodoByTodoID updates a todo item by looking up its conversation from the todo.
// Verifies ownership via the conversation's user_id.
func (s *TodoService) UpdateTodoByTodoID(userID, todoID uuid.UUID, input types.PatchTodosIdJSONBody) (*model.Todo, error) {
	var todo model.Todo
	if err := s.db.Where("id = ?", todoID).First(&todo).Error; err != nil {
		return nil, fmt.Errorf("todo not found")
	}

	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", todo.ConversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	return s.UpdateTodo(userID, todo.ConversationID, todoID, input)
}

// DeleteTodoByTodoID deletes a todo item by looking up its conversation from the todo.
// Verifies ownership via the conversation's user_id.
func (s *TodoService) DeleteTodoByTodoID(userID, todoID uuid.UUID) error {
	var todo model.Todo
	if err := s.db.Where("id = ?", todoID).First(&todo).Error; err != nil {
		return fmt.Errorf("todo not found")
	}

	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", todo.ConversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	return s.DeleteTodo(userID, todo.ConversationID, todoID)
}

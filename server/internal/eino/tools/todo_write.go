package tools

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// TodoSyncFunc is a callback that pushes a todo.sync update to the user.
// This is set by module.go to avoid import cycles.
var TodoSyncFunc func(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID)

// TodoReadInput is the input schema for the todo_read tool.
type TodoReadInput struct{}

// TodoReadOutput is the output schema for the todo_read tool.
type TodoReadOutput struct {
	Todos []TodoItem `json:"todos" jsonschema_description:"List of todo items"`
}

// TodoItem represents a single todo in tool output.
type TodoItem struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Completed bool   `json:"completed"`
}

// TodoReadTool reads todos for a conversation.
type TodoReadTool struct {
	db             *gorm.DB
	ConversationID uuid.UUID
}

// NewTodoReadTool creates a todo read tool.
func NewTodoReadTool(db *gorm.DB, conversationID uuid.UUID) (tool.InvokableTool, error) {
	t := &TodoReadTool{db: db, ConversationID: conversationID}
	return utils.InferTool("todo_read", "Read all todo items for the current conversation. Returns a list of todos with their completion status. Use this to check what tasks are pending or completed.",
		func(ctx context.Context, input TodoReadInput) (TodoReadOutput, error) {
			var todos []model.Todo
			if err := t.db.Where("conversation_id = ?", t.ConversationID).Order("created_at ASC").Find(&todos).Error; err != nil {
				return TodoReadOutput{}, err
			}

			items := make([]TodoItem, 0, len(todos))
			for _, todo := range todos {
				items = append(items, TodoItem{
					ID:        todo.ID.String(),
					Content:   todo.Content,
					Completed: todo.Completed,
				})
			}
			return TodoReadOutput{Todos: items}, nil
		})
}

// TodoWriteInput is the input schema for the todo_write tool.
type TodoWriteInput struct {
	Action    string `json:"action" jsonschema_description:"Action to perform: 'create', 'update', or 'delete'"`
	Content   string `json:"content,omitempty" jsonschema_description:"Todo text (required for create, optional for update)"`
	TodoID    string `json:"todo_id,omitempty" jsonschema_description:"Todo ID to update or delete (required for update/delete)"`
	Completed *bool  `json:"completed,omitempty" jsonschema_description:"New completion status (for update action)"`
}

// TodoWriteOutput is the output schema for the todo_write tool.
type TodoWriteOutput struct {
	Success bool     `json:"success" jsonschema_description:"Whether the operation succeeded"`
	Message string   `json:"message" jsonschema_description:"Human-readable result description"`
	Todos   []string `json:"todos,omitempty" jsonschema_description:"Affected todo IDs"`
}

// TodoWriteTool creates, updates, or deletes todos.
type TodoWriteTool struct {
	db             *gorm.DB
	ConversationID uuid.UUID
}

// NewTodoWriteTool creates a todo write tool.
func NewTodoWriteTool(db *gorm.DB, conversationID uuid.UUID) (tool.InvokableTool, error) {
	t := &TodoWriteTool{db: db, ConversationID: conversationID}
	return utils.InferTool("todo_write", "Create, update, or delete todo items for the current conversation. Use 'create' to add a new todo, 'update' to modify an existing one, or 'delete' to remove one. When updating, you can change the content text, the completion status, or both.",
		func(ctx context.Context, input TodoWriteInput) (TodoWriteOutput, error) {
			switch input.Action {
			case "create":
				if input.Content == "" {
					return TodoWriteOutput{Success: false, Message: "content is required for create"}, nil
				}
				todo := model.Todo{
					ConversationID: t.ConversationID,
					Content:        input.Content,
					Metadata:       model.JSONMap{},
				}
				if err := t.db.Create(&todo).Error; err != nil {
					return TodoWriteOutput{Success: false, Message: "failed to create todo: " + err.Error()}, nil
				}

				// Notify the frontend via todo.sync so the todo panel refreshes.
				var conv model.Conversation
				if err := t.db.WithContext(ctx).Where("id = ?", t.ConversationID).First(&conv).Error; err == nil && TodoSyncFunc != nil {
					TodoSyncFunc(ctx, conv.UserID, t.ConversationID)
				}

				return TodoWriteOutput{
					Success: true,
					Message: "Created todo: " + input.Content,
					Todos:   []string{todo.ID.String()},
				}, nil

			case "update":
				if input.TodoID == "" {
					return TodoWriteOutput{Success: false, Message: "todo_id is required for update"}, nil
				}
				todoID, err := uuid.Parse(input.TodoID)
				if err != nil {
					return TodoWriteOutput{Success: false, Message: "invalid todo_id: " + err.Error()}, nil
				}
				var todo model.Todo
				if err := t.db.Where("id = ? AND conversation_id = ?", todoID, t.ConversationID).First(&todo).Error; err != nil {
					return TodoWriteOutput{Success: false, Message: "todo not found"}, nil
				}
				updates := map[string]interface{}{}
				if input.Content != "" {
					updates["content"] = input.Content
				}
				if input.Completed != nil {
					updates["completed"] = *input.Completed
				}
				if len(updates) > 0 {
					if err := t.db.Model(&todo).Updates(updates).Error; err != nil {
						return TodoWriteOutput{Success: false, Message: "failed to update todo: " + err.Error()}, nil
					}
				}

				// Notify the frontend via todo.sync so the todo panel refreshes.
				var conv model.Conversation
				if err := t.db.WithContext(ctx).Where("id = ?", t.ConversationID).First(&conv).Error; err == nil && TodoSyncFunc != nil {
					TodoSyncFunc(ctx, conv.UserID, t.ConversationID)
				}

				return TodoWriteOutput{
					Success: true,
					Message: "Updated todo",
					Todos:   []string{input.TodoID},
				}, nil

			case "delete":
				if input.TodoID == "" {
					return TodoWriteOutput{Success: false, Message: "todo_id is required for delete"}, nil
				}
				todoID, err := uuid.Parse(input.TodoID)
				if err != nil {
					return TodoWriteOutput{Success: false, Message: "invalid todo_id: " + err.Error()}, nil
				}
				result := t.db.Where("id = ? AND conversation_id = ?", todoID, t.ConversationID).Delete(&model.Todo{})
				if result.Error != nil {
					return TodoWriteOutput{Success: false, Message: "failed to delete todo: " + result.Error.Error()}, nil
				}
				if result.RowsAffected == 0 {
					return TodoWriteOutput{Success: false, Message: "todo not found"}, nil
				}

				// Notify the frontend via todo.sync so the todo panel refreshes.
				var conv model.Conversation
				if err := t.db.WithContext(ctx).Where("id = ?", t.ConversationID).First(&conv).Error; err == nil && TodoSyncFunc != nil {
					TodoSyncFunc(ctx, conv.UserID, t.ConversationID)
				}

				return TodoWriteOutput{
					Success: true,
					Message: "Deleted todo",
				}, nil

			default:
				return TodoWriteOutput{Success: false, Message: "unknown action: " + input.Action + ". Use 'create', 'update', or 'delete'."}, nil
			}
		})
}

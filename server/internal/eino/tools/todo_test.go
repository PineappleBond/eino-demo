package tools

import (
	"testing"

	"github.com/google/uuid"
)

// ── TodoReadTool with nil DB ──

func TestTodoReadTool_NilDB(t *testing.T) {
	tool, err := NewTodoReadTool(nil, testUUID())
	if err != nil {
		t.Fatalf("NewTodoReadTool() error = %v", err)
	}
	if tool == nil {
		t.Fatal("NewTodoReadTool() returned nil")
	}
}

func TestTodoWriteTool_NilDB(t *testing.T) {
	tool, err := NewTodoWriteTool(nil, testUUID())
	if err != nil {
		t.Fatalf("NewTodoWriteTool() error = %v", err)
	}
	if tool == nil {
		t.Fatal("NewTodoWriteTool() returned nil")
	}
}

// ── TodoRead/Write output structure tests ──

func TestTodoReadOutput_Structure(t *testing.T) {
	out := TodoReadOutput{
		Todos: []TodoItem{
			{ID: "1", Content: "task1", Completed: false},
			{ID: "2", Content: "task2", Completed: true},
		},
	}
	if len(out.Todos) != 2 {
		t.Fatalf("Todos count = %d, want 2", len(out.Todos))
	}
	if out.Todos[0].Content != "task1" {
		t.Errorf("Todo[0].Content = %q, want %q", out.Todos[0].Content, "task1")
	}
	if out.Todos[1].Completed != true {
		t.Errorf("Todo[1].Completed = %v, want true", out.Todos[1].Completed)
	}
}

func TestTodoWriteOutput_Structure(t *testing.T) {
	tests := []struct {
		name    string
		out     TodoWriteOutput
		wantOK  bool
		checkFn func(t *testing.T, out TodoWriteOutput)
	}{
		{
			name:   "create success",
			out:    TodoWriteOutput{Success: true, Message: "Created todo: test", Todos: []string{"id-1"}},
			wantOK: true,
		},
		{
			name:   "create failure - missing content",
			out:    TodoWriteOutput{Success: false, Message: "content is required for create"},
			wantOK: false,
		},
		{
			name:   "update success",
			out:    TodoWriteOutput{Success: true, Message: "Updated todo", Todos: []string{"id-1"}},
			wantOK: true,
		},
		{
			name:   "delete success",
			out:    TodoWriteOutput{Success: true, Message: "Deleted todo"},
			wantOK: true,
		},
		{
			name:   "unknown action",
			out:    TodoWriteOutput{Success: false, Message: "unknown action: foo. Use 'create', 'update', or 'delete'."},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.out.Success != tt.wantOK {
				t.Errorf("Success = %v, want %v", tt.out.Success, tt.wantOK)
			}
			if tt.out.Message == "" {
				t.Error("Message should not be empty")
			}
		})
	}
}

// ── TodoWriteInput validation tests ──

func TestTodoWriteInput_Fields(t *testing.T) {
	tests := []struct {
		name  string
		input TodoWriteInput
	}{
		{
			name:  "create with content",
			input: TodoWriteInput{Action: "create", Content: "buy milk"},
		},
		{
			name:  "update with todo_id",
			input: TodoWriteInput{Action: "update", TodoID: "some-uuid", Content: "updated"},
		},
		{
			name:  "update completed status",
			input: TodoWriteInput{Action: "update", TodoID: "some-uuid", Completed: boolPtr(true)},
		},
		{
			name:  "delete with todo_id",
			input: TodoWriteInput{Action: "delete", TodoID: "some-uuid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input.Action == "" {
				t.Error("Action should not be empty")
			}
		})
	}
}

func TestTodoItem_Structure(t *testing.T) {
	item := TodoItem{
		ID:        "test-id",
		Content:   "test content",
		Completed: true,
	}
	if item.ID != "test-id" {
		t.Errorf("ID = %q, want %q", item.ID, "test-id")
	}
	if item.Content != "test content" {
		t.Errorf("Content = %q, want %q", item.Content, "test content")
	}
	if !item.Completed {
		t.Error("Completed should be true")
	}
}

// ── Tool creation with various UUIDs ──

func TestTodoReadTool_Creation(t *testing.T) {
	convID := uuid.New()
	tool, err := NewTodoReadTool(nil, convID)
	if err != nil {
		t.Fatalf("NewTodoReadTool() error = %v", err)
	}
	if tool == nil {
		t.Fatal("tool is nil")
	}
}

func TestTodoWriteTool_Creation(t *testing.T) {
	convID := uuid.New()
	tool, err := NewTodoWriteTool(nil, convID)
	if err != nil {
		t.Fatalf("NewTodoWriteTool() error = %v", err)
	}
	if tool == nil {
		t.Fatal("tool is nil")
	}
}

func boolPtr(b bool) *bool { return &b }

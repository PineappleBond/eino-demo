package tools

import (
	"testing"

	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
)

// ── ToolRegistry creation tests ──

func TestNewToolRegistry(t *testing.T) {
	cfg := &config.Config{
		ServerPort:   "8080",
		TavilyAPIKey: "", // empty — tavily tool should be skipped
	}

	registry := NewToolRegistry(cfg, nil)
	if registry == nil {
		t.Fatal("NewToolRegistry() returned nil")
	}
}

func TestNewToolRegistry_WithTavilyKey(t *testing.T) {
	cfg := &config.Config{
		ServerPort:   "8080",
		TavilyAPIKey: "test-api-key",
	}

	registry := NewToolRegistry(cfg, nil)
	if registry == nil {
		t.Fatal("NewToolRegistry() returned nil")
	}
}

// ── ListToolNames ──

func TestToolRegistry_ListToolNames_WithoutConversation(t *testing.T) {
	cfg := &config.Config{
		TavilyAPIKey: "key",
	}
	registry := NewToolRegistry(cfg, nil)

	names := registry.ListToolNames(ToolBuildContext{})
	expected := []string{"weather", "tavily_search", "ask_user_question"}

	if len(names) != len(expected) {
		t.Fatalf("ListToolNames() = %v, want %v", names, expected)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestToolRegistry_ListToolNames_WithConversation(t *testing.T) {
	cfg := &config.Config{
		TavilyAPIKey: "key",
	}
	registry := NewToolRegistry(cfg, nil)

	// With nil DB, conversation-scoped tools are NOT advertised (matches GetBaseTools behavior).
	names := registry.ListToolNames(ToolBuildContext{ConversationID: uuid.New()})
	expected := []string{"weather", "tavily_search", "ask_user_question"}

	if len(names) != len(expected) {
		t.Fatalf("ListToolNames() = %v, want %v", names, expected)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

// ── GetBaseTools ──

func TestToolRegistry_GetBaseTools_WithoutConversation(t *testing.T) {
	cfg := &config.Config{
		TavilyAPIKey: "key",
	}
	registry := NewToolRegistry(cfg, nil)

	tools := registry.GetBaseTools(ToolBuildContext{})
	// Should have weather + tavily_search + ask_user_question = 3
	if len(tools) < 2 {
		t.Errorf("GetBaseTools() returned %d tools, expected at least 2", len(tools))
	}
}

func TestToolRegistry_GetBaseTools_WithConversation_NilDB(t *testing.T) {
	cfg := &config.Config{
		TavilyAPIKey: "key",
	}
	registry := NewToolRegistry(cfg, nil)

	// With nil DB, conversation-scoped tools (todo_read, todo_write, cron_task)
	// should NOT be added (r.db is nil check)
	tools := registry.GetBaseTools(ToolBuildContext{ConversationID: uuid.New()})
	// Should have weather + tavily + ask_user_question = 3
	// No todo/cron/tools since DB is nil
	if len(tools) < 2 {
		t.Errorf("GetBaseTools() returned %d tools, expected at least 2 (base tools only)", len(tools))
	}
}

// ── GetWeatherTool ──

func TestToolRegistry_GetWeatherTool(t *testing.T) {
	cfg := &config.Config{}
	registry := NewToolRegistry(cfg, nil)

	tool := registry.GetWeatherTool()
	if tool == nil {
		t.Error("GetWeatherTool() returned nil")
	}
}

// ── GetPermissionTools ──

func TestToolRegistry_GetPermissionTools(t *testing.T) {
	cfg := &config.Config{}
	registry := NewToolRegistry(cfg, nil)

	tools := registry.GetPermissionTools("/workspace")
	if len(tools) == 0 {
		t.Error("GetPermissionTools() returned no tools")
	}
}

func TestToolRegistry_GetPermissionTools_WithHTTP(t *testing.T) {
	cfg := &config.Config{}
	registry := NewToolRegistry(cfg, nil)

	tools := registry.GetPermissionTools("")
	// HTTP tools may or may not be available depending on httprequest.NewToolKit
	// At minimum, filesystem tools should be present
	if len(tools) < 6 {
		t.Errorf("GetPermissionTools() returned %d tools, expected at least 6 (filesystem tools)", len(tools))
	}
}

// ── buildBaseTools panic on failure ──

func TestToolRegistry_BuildBaseTools_NoPanic(t *testing.T) {
	// NewToolRegistry should not panic with valid config
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewToolRegistry panicked: %v", r)
		}
	}()

	cfg := &config.Config{}
	_ = NewToolRegistry(cfg, nil)
}

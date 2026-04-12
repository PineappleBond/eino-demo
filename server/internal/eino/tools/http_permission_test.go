package tools

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ── extractMethod tests ──

func TestExtractMethod(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{"request_get", "request_get", "GET"},
		{"request_post", "request_post", "POST"},
		{"request_put", "request_put", "PUT"},
		{"request_delete", "request_delete", "DELETE"},
		{"http_get_tool", "http_get_tool", "GET"},
		{"GET_request", "GET_request", "GET"},
		{"post_data", "post_data", "POST"},
		{"unknown_tool", "unknown_tool", ""},
		{"fetch", "fetch", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMethod(tt.toolName)
			if got != tt.want {
				t.Errorf("extractMethod(%q) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}

func TestExtractMethod_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{"uppercase GET", "REQUEST_GET", "GET"},
		{"uppercase POST", "REQUEST_POST", "POST"},
		{"mixed case", "Request_Get", "GET"},
		{"lowercase", "request_get", "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMethod(tt.toolName)
			if got != tt.want {
				t.Errorf("extractMethod(%q) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}

// ── WrapHTTPTools tests ──

func TestWrapHTTPTools_Empty(t *testing.T) {
	wrapped := WrapHTTPTools(nil)
	if wrapped != nil && len(wrapped) != 0 {
		t.Errorf("WrapHTTPTools(nil) = %v, want empty slice", wrapped)
	}

	wrapped = WrapHTTPTools([]tool.BaseTool{})
	if len(wrapped) != 0 {
		t.Errorf("WrapHTTPTools([]) = %d tools, want 0", len(wrapped))
	}
}

// ── httpPermWrapper NeedPermission tests ──

func TestHTTPPermWrapper_NeedPermission_GET(t *testing.T) {
	w := &httpPermWrapper{
		method: "GET",
		name:   "request_get",
		desc:   "Make a GET request",
	}

	req := w.NeedPermission(map[string]any{
		"url": "https://api.example.com/data",
	})
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	if req.Action != "network" {
		t.Errorf("Action = %q, want %q", req.Action, "network")
	}
	if req.ToolLevel != 2 {
		t.Errorf("ToolLevel = %d, want 2 for GET", req.ToolLevel)
	}
	if req.ToolName != "request_get" {
		t.Errorf("ToolName = %q, want %q", req.ToolName, "request_get")
	}
}

func TestHTTPPermWrapper_NeedPermission_POST(t *testing.T) {
	w := &httpPermWrapper{
		method: "POST",
		name:   "request_post",
		desc:   "Make a POST request",
	}

	req := w.NeedPermission(map[string]any{
		"url": "https://api.example.com/submit",
	})
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	if req.Action != "network" {
		t.Errorf("Action = %q, want %q", req.Action, "network")
	}
	if req.ToolLevel != 3 {
		t.Errorf("ToolLevel = %d, want 3 for POST", req.ToolLevel)
	}
	if req.Content != "POST https://api.example.com/submit" {
		t.Errorf("Content = %q, want %q", req.Content, "POST https://api.example.com/submit")
	}
}

func TestHTTPPermWrapper_NeedPermission_PUT(t *testing.T) {
	w := &httpPermWrapper{
		method: "PUT",
		name:   "request_put",
	}

	req := w.NeedPermission(map[string]any{
		"url": "https://api.example.com/update/1",
	})
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	if req.ToolLevel != 3 {
		t.Errorf("ToolLevel = %d, want 3 for PUT", req.ToolLevel)
	}
}

func TestHTTPPermWrapper_NeedPermission_DELETE(t *testing.T) {
	w := &httpPermWrapper{
		method: "DELETE",
		name:   "request_delete",
	}

	req := w.NeedPermission(map[string]any{
		"url": "https://api.example.com/delete/1",
	})
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	if req.ToolLevel != 3 {
		t.Errorf("ToolLevel = %d, want 3 for DELETE", req.ToolLevel)
	}
}

func TestHTTPPermWrapper_NeedPermission_UnknownURL(t *testing.T) {
	w := &httpPermWrapper{
		method: "GET",
		name:   "request_get",
	}

	// Empty URL — content becomes "GET " (method + space + empty string)
	// The code's fallback check is content == " " || content == "", which
	// doesn't match "GET ", so the trailing-space content is used.
	req := w.NeedPermission(map[string]any{})
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	if req.Content != "GET " {
		t.Errorf("Content = %q, want %q", req.Content, "GET ")
	}
}

func TestHTTPPermWrapper_NeedPermission_NilURL(t *testing.T) {
	w := &httpPermWrapper{
		method: "POST",
		name:   "request_post",
	}

	req := w.NeedPermission(map[string]any{"url": nil})
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	if req.Content != "POST " {
		t.Errorf("Content = %q, want %q", req.Content, "POST ")
	}
}

func TestHTTPPermWrapper_NeedPermission_NonStringURL(t *testing.T) {
	w := &httpPermWrapper{
		method: "GET",
		name:   "request_get",
	}

	req := w.NeedPermission(map[string]any{"url": 12345})
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	if req.Content != "GET " {
		t.Errorf("Content = %q, want %q", req.Content, "GET ")
	}
}

func TestHTTPPermWrapper_NeedPermission_NilInput(t *testing.T) {
	w := &httpPermWrapper{
		method: "GET",
		name:   "request_get",
	}

	req := w.NeedPermission(nil)
	if req == nil {
		t.Fatal("NeedPermission() returned nil")
	}
	// Type assertion to map fails, url is empty, content = "GET "
	if req.Content != "GET " {
		t.Errorf("Content = %q, want %q", req.Content, "GET ")
	}
}

// ── httpPermWrapper Info test ──

func TestHTTPPermWrapper_Info(t *testing.T) {
	// Create a mock tool that implements tool.BaseTool
	w := &httpPermWrapper{
		tool: &mockBaseTool{
			info: &schema.ToolInfo{
				Name: "request_get",
				Desc: "Make a GET request",
			},
		},
		method: "GET",
		name:   "request_get",
		desc:   "Make a GET request",
	}

	info, err := w.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "request_get" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "request_get")
	}
}

// ── Mock types for testing ──

type mockBaseTool struct {
	info *schema.ToolInfo
}

func (m *mockBaseTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return m.info, nil
}

func (m *mockBaseTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	return `{"status":"ok"}`, nil
}

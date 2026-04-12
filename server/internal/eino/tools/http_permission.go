package tools

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino-ext/components/tool/httprequest"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
)

// httpPermWrapper wraps an HTTP request tool with NeedPermissioner support.
type httpPermWrapper struct {
	tool   tool.BaseTool
	method string // "GET", "POST", "PUT", "DELETE"
	name   string
	desc   string
}

func (w *httpPermWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.tool.Info(ctx)
}

func (w *httpPermWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if inv, ok := w.tool.(tool.InvokableTool); ok {
		return inv.InvokableRun(ctx, argumentsInJSON, opts...)
	}
	return "", nil
}

func (w *httpPermWrapper) StreamableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
	if sr, ok := w.tool.(tool.StreamableTool); ok {
		return sr.StreamableRun(ctx, argumentsInJSON, opts...)
	}
	return nil, nil
}

func (w *httpPermWrapper) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	url := ""
	if v, ok := args["url"].(string); ok {
		url = v
	}

	var action string
	var riskLevel int
	switch w.method {
	case "GET":
		action = "network"
		riskLevel = 2
	default:
		action = "network"
		riskLevel = 3
	}

	content := w.method + " " + url
	if content == " " || content == "" {
		content = w.method + " (URL unknown)"
	}

	return &permission.PermissionRequest{
		Action:      action,
		Content:     content,
		ToolName:    w.name,
		ToolDesc:    w.desc,
		ArgsSummary: content,
		ToolLevel:   riskLevel,
	}
}

// WrapHTTPTools wraps httprequest tools with NeedPermissioner.
func WrapHTTPTools(tools []tool.BaseTool) []tool.BaseTool {
	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			wrapped[i] = t
			continue
		}
		method := extractMethod(info.Name)
		if method == "" {
			wrapped[i] = t
			continue
		}
		wrapped[i] = &httpPermWrapper{
			tool:   t,
			method: method,
			name:   info.Name,
			desc:   info.Desc,
		}
	}
	return wrapped
}

// extractMethod extracts HTTP method from tool name like "request_get" -> "GET".
func extractMethod(name string) string {
	lower := strings.ToLower(name)
	for _, m := range []string{"get", "post", "put", "delete"} {
		if strings.Contains(lower, m) {
			return strings.ToUpper(m)
		}
	}
	return ""
}

// NewHTTPTools creates httprequest tools with permission support.
func NewHTTPTools() []tool.BaseTool {
	rawTools, err := httprequest.NewToolKit(context.Background(), nil)
	if err != nil {
		return nil
	}
	return WrapHTTPTools(rawTools)
}

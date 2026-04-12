package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
)

// NewFilesystemTools creates filesystem tools (read_file, write_file, edit_file, glob, grep, execute)
// that implement NeedPermissioner for the permission gateway.
func NewFilesystemTools(workspaceDir string) []tool.BaseTool {
	return []tool.BaseTool{
		&readFilePerm{workspaceDir: workspaceDir},
		&writeFilePerm{workspaceDir: workspaceDir},
		&editFilePerm{workspaceDir: workspaceDir},
		&globPerm{workspaceDir: workspaceDir},
		&grepPerm{workspaceDir: workspaceDir},
		&executePerm{workspaceDir: workspaceDir},
	}
}

// resolveFilePath resolves a relative path against the workspace directory.
// If the path is already absolute, it is returned as-is.
// If workspaceDir is empty, the path is returned unchanged.
func resolveFilePath(path, workspaceDir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if workspaceDir == "" {
		return path
	}
	return filepath.Clean(filepath.Join(workspaceDir, path))
}

// checkFileWorkspaceBounds returns an error if the resolved path is outside the workspace.
func checkFileWorkspaceBounds(path, workspaceDir string) error {
	if workspaceDir == "" {
		return nil
	}
	cleanWS := filepath.Clean(workspaceDir)
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, cleanWS+string(filepath.Separator)) && cleanPath != cleanWS {
		return fmt.Errorf("path %q is outside workspace %q", path, workspaceDir)
	}
	return nil
}

// ── read_file ──

type readFileInput struct {
	FilePath string `json:"file_path" jsonschema_description:"Absolute path to the file to read"`
	Offset   int    `json:"offset" jsonschema_description:"Starting line number (1-based, default 1)"`
	Limit    int    `json:"limit" jsonschema_description:"Maximum number of lines to read (default 2000)"`
}

type readFilePerm struct{ workspaceDir string }

func (p *readFilePerm) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ws := p.workspaceDir
	if ws == "" {
		ws = "/"
	}
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: fmt.Sprintf("Read the content of a file at path 'file_path'. If not absolute, it is resolved relative to workspace %q.", ws),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {Type: schema.String, Desc: "File path (absolute or relative to workspace " + ws + ")", Required: true},
			"offset":    {Type: schema.Integer, Desc: "Starting line number (1-based, default 1)"},
			"limit":     {Type: schema.Integer, Desc: "Maximum number of lines to read (default 2000)"},
		}),
	}, nil
}

func (p *readFilePerm) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input readFileInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	resolvedPath := resolveFilePath(input.FilePath, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	content, err := readFileContent(resolvedPath, input.Offset, input.Limit)
	if err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}
	return content, nil
}

func (p *readFilePerm) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	filePath := getStringAny(args, "file_path")
	resolvedPath := resolveFilePath(filePath, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return &permission.PermissionRequest{
			Action:      "read",
			Content:     "读取文件(路径超出workspace): " + resolvedPath,
			ToolName:    "read_file",
			ToolDesc:    "读取文件内容",
			ArgsSummary: "读取 " + resolvedPath,
			ToolLevel:   4,
		}
	}
	return &permission.PermissionRequest{
		Action:      "read",
		Content:     "读取文件: " + resolvedPath,
		ToolName:    "read_file",
		ToolDesc:    "读取文件内容",
		ArgsSummary: "读取 " + resolvedPath,
		ToolLevel:   2,
	}
}

func readFileContent(path string, offset, limit int) (string, error) {
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = 2000
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	if offset > len(lines) {
		return "", nil
	}

	end := offset - 1 + limit
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[offset-1:end], "\n"), nil
}

// ── write_file ──

type writeFileInput struct {
	FilePath string `json:"file_path" jsonschema_description:"Absolute path to the file to create"`
	Content  string `json:"content" jsonschema_description:"Content to write to the file"`
}

type writeFilePerm struct{ workspaceDir string }

func (p *writeFilePerm) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ws := p.workspaceDir
	if ws == "" {
		ws = "/"
	}
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: fmt.Sprintf("Create a new file or overwrite an existing file. Path is resolved relative to workspace %q.", ws),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {Type: schema.String, Desc: "File path (absolute or relative to workspace " + ws + ")", Required: true},
			"content":   {Type: schema.String, Desc: "Content to write to the file", Required: true},
		}),
	}, nil
}

func (p *writeFilePerm) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input writeFileInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	resolvedPath := resolveFilePath(input.FilePath, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	parentDir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Sprintf("<tool_error>\nfailed to create parent directory: %s\n</tool_error>", err.Error()), nil
	}

	if err := os.WriteFile(resolvedPath, []byte(input.Content), 0644); err != nil {
		return fmt.Sprintf("<tool_error>\nfailed to write file: %s\n</tool_error>", err.Error()), nil
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), resolvedPath), nil
}

func (p *writeFilePerm) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	filePath := getStringAny(args, "file_path")
	resolvedPath := resolveFilePath(filePath, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return &permission.PermissionRequest{
			Action:      "write",
			Content:     "创建/覆盖文件(路径超出workspace): " + resolvedPath,
			ToolName:    "write_file",
			ToolDesc:    "创建新文件或覆盖现有文件",
			ArgsSummary: "写入 " + resolvedPath,
			ToolLevel:   4,
		}
	}
	return &permission.PermissionRequest{
		Action:      "write",
		Content:     "创建/覆盖文件: " + resolvedPath,
		ToolName:    "write_file",
		ToolDesc:    "创建新文件或覆盖现有文件",
		ArgsSummary: "写入 " + resolvedPath,
		ToolLevel:   3,
	}
}

// ── edit_file ──

type editFileInput struct {
	FilePath   string `json:"file_path" jsonschema_description:"Absolute path to the file to edit"`
	OldString  string `json:"old_string" jsonschema_description:"The text to replace"`
	NewString  string `json:"new_string" jsonschema_description:"The replacement text"`
	ReplaceAll bool   `json:"replace_all" jsonschema_description:"Replace all occurrences (default false)"`
}

type editFilePerm struct{ workspaceDir string }

func (p *editFilePerm) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ws := p.workspaceDir
	if ws == "" {
		ws = "/"
	}
	return &schema.ToolInfo{
		Name: "edit_file",
		Desc: fmt.Sprintf("Find and replace text in a file. Path is resolved relative to workspace %q.", ws),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path":   {Type: schema.String, Desc: "File path (absolute or relative to workspace " + ws + ")", Required: true},
			"old_string":  {Type: schema.String, Desc: "The text to replace", Required: true},
			"new_string":  {Type: schema.String, Desc: "The replacement text", Required: true},
			"replace_all": {Type: schema.Boolean, Desc: "Replace all occurrences (default false)"},
		}),
	}, nil
}

func (p *editFilePerm) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input editFileInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	resolvedPath := resolveFilePath(input.FilePath, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	if input.OldString == "" {
		return fmt.Sprintf("<tool_error>\nold_string is required\n</tool_error>"), nil
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Sprintf("<tool_error>\nfailed to read file: %s\n</tool_error>", err.Error()), nil
	}

	text := string(content)
	count := strings.Count(text, input.OldString)
	if count == 0 {
		return fmt.Sprintf("<tool_error>\nstring not found in file: '%s'\n</tool_error>", input.OldString), nil
	}
	if count > 1 && !input.ReplaceAll {
		return fmt.Sprintf("<tool_error>\nstring appears %d times. Use replace_all=true to replace all\n</tool_error>", count), nil
	}

	var newText string
	if input.ReplaceAll {
		newText = strings.ReplaceAll(text, input.OldString, input.NewString)
	} else {
		newText = strings.Replace(text, input.OldString, input.NewString, 1)
	}

	if err := os.WriteFile(resolvedPath, []byte(newText), 0644); err != nil {
		return fmt.Sprintf("<tool_error>\nfailed to write file: %s\n</tool_error>", err.Error()), nil
	}

	replacements := 1
	if input.ReplaceAll {
		replacements = count
	}
	return fmt.Sprintf("Replaced %d occurrence(s) in %s", replacements, resolvedPath), nil
}

func (p *editFilePerm) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	filePath := getStringAny(args, "file_path")
	resolvedPath := resolveFilePath(filePath, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return &permission.PermissionRequest{
			Action:      "write",
			Content:     "修改文件(路径超出workspace): " + resolvedPath,
			ToolName:    "edit_file",
			ToolDesc:    "在文件中查找并替换文本",
			ArgsSummary: "编辑 " + resolvedPath,
			ToolLevel:   4,
		}
	}
	return &permission.PermissionRequest{
		Action:      "write",
		Content:     "修改文件: " + resolvedPath,
		ToolName:    "edit_file",
		ToolDesc:    "在文件中查找并替换文本",
		ArgsSummary: "编辑 " + resolvedPath,
		ToolLevel:   3,
	}
}

// ── glob ──

type globInput struct {
	Path    string `json:"path" jsonschema_description:"Directory to start searching from"`
	Pattern string `json:"pattern" jsonschema_description:"Glob pattern to match files"`
}

type globPerm struct{ workspaceDir string }

func (p *globPerm) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ws := p.workspaceDir
	if ws == "" {
		ws = "/"
	}
	return &schema.ToolInfo{
		Name: "glob",
		Desc: fmt.Sprintf("Find files matching a glob pattern. Search starts from workspace %q.", ws),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Desc: "Directory to search from (defaults to workspace " + ws + ")"},
			"pattern": {Type: schema.String, Desc: "Glob pattern to match files (e.g. '*.go', '**/*.ts')", Required: true},
		}),
	}, nil
}

func (p *globPerm) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input globInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	searchPath := resolveFilePath(input.Path, p.workspaceDir)
	if searchPath == "" || input.Path == "" {
		searchPath = "/"
	}
	if err := checkFileWorkspaceBounds(searchPath, p.workspaceDir); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	var matches []string
	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return filepath.SkipDir
			}
			return err
		}
		relPath, _ := filepath.Rel(searchPath, path)
		if relPath == "." {
			return nil
		}
		matched, _ := doublestar.Match(input.Pattern, filepath.ToSlash(relPath))
		if matched {
			matches = append(matches, relPath)
		}
		return nil
	})
	if err != nil {
		return fmt.Sprintf("<tool_error>\nfailed to walk directory: %s\n</tool_error>", err.Error()), nil
	}

	if len(matches) == 0 {
		return "No files found", nil
	}

	return strings.Join(matches, "\n"), nil
}

func (p *globPerm) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	path := getStringAny(args, "path")
	pattern := getStringAny(args, "pattern")

	resolvedPath := resolveFilePath(path, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return &permission.PermissionRequest{
			Action:      "read",
			Content:     "glob 查找(路径超出workspace): " + resolvedPath + " " + pattern,
			ToolName:    "glob",
			ToolDesc:    "使用 glob 模式递归查找文件",
			ArgsSummary: "glob: " + pattern + " in " + resolvedPath,
			ToolLevel:   4,
		}
	}
	return &permission.PermissionRequest{
		Action:      "read",
		Content:     "glob 查找: " + resolvedPath + " " + pattern,
		ToolName:    "glob",
		ToolDesc:    "使用 glob 模式递归查找文件",
		ArgsSummary: "glob: " + pattern + " in " + resolvedPath,
		ToolLevel:   1,
	}
}

// ── grep ──

type grepInput struct {
	Path            string `json:"path" jsonschema_description:"Directory or file to search in"`
	Pattern         string `json:"pattern" jsonschema_description:"Regex pattern to search for"`
	CaseInsensitive bool   `json:"case_insensitive" jsonschema_description:"Enable case-insensitive search"`
	Glob            string `json:"glob" jsonschema_description:"Optional glob to filter files"`
}

type grepPerm struct{ workspaceDir string }

func (p *grepPerm) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ws := p.workspaceDir
	if ws == "" {
		ws = "/"
	}
	return &schema.ToolInfo{
		Name: "grep",
		Desc: fmt.Sprintf("Search for a pattern in files using ripgrep (rg). Search starts from workspace %q.", ws),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":             {Type: schema.String, Desc: "Directory or file to search (defaults to workspace " + ws + ")"},
			"pattern":          {Type: schema.String, Desc: "Regex pattern to search for", Required: true},
			"case_insensitive": {Type: schema.Boolean, Desc: "Enable case-insensitive search"},
			"glob":             {Type: schema.String, Desc: "Optional glob to filter files (e.g. '*.go')"},
		}),
	}, nil
}

func (p *grepPerm) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input grepInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	if input.Pattern == "" {
		return fmt.Sprintf("<tool_error>\npattern is required\n</tool_error>"), nil
	}

	searchPath := resolveFilePath(input.Path, p.workspaceDir)
	if searchPath == "" {
		searchPath = "."
	}
	if err := checkFileWorkspaceBounds(searchPath, p.workspaceDir); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	cmdArgs := []string{"rg", "--json", "--no-heading"}
	if input.CaseInsensitive {
		cmdArgs = append(cmdArgs, "-i")
	}
	if input.Glob != "" {
		cmdArgs = append(cmdArgs, "--glob", input.Glob)
	}
	cmdArgs = append(cmdArgs, "-e", input.Pattern, "--", searchPath)

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	if p.workspaceDir != "" {
		cmd.Dir = p.workspaceDir
	}
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		return fmt.Sprintf("<tool_error>\nripgrep failed: %s\n</tool_error>", err.Error()), nil
	}

	if len(output) == 0 {
		return "No matches found", nil
	}

	return string(output), nil
}

func (p *grepPerm) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	pattern := getStringAny(args, "pattern")
	path := getStringAny(args, "path")

	resolvedPath := resolveFilePath(path, p.workspaceDir)
	if err := checkFileWorkspaceBounds(resolvedPath, p.workspaceDir); err != nil {
		return &permission.PermissionRequest{
			Action:      "read",
			Content:     "搜索文件内容(路径超出workspace): " + pattern + " in " + resolvedPath,
			ToolName:    "grep",
			ToolDesc:    "使用 ripgrep 在文件中搜索内容",
			ArgsSummary: "grep '" + pattern + "' in " + resolvedPath,
			ToolLevel:   4,
		}
	}
	return &permission.PermissionRequest{
		Action:      "read",
		Content:     "搜索文件内容: " + pattern + " in " + resolvedPath,
		ToolName:    "grep",
		ToolDesc:    "使用 ripgrep 在文件中搜索内容",
		ArgsSummary: "grep '" + pattern + "' in " + resolvedPath,
		ToolLevel:   2,
	}
}

// ── helpers ──

func getStringAny(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
)

// ── resolveFilePath tests ──

func TestResolveFilePath(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		workspaceDir string
		want         string
	}{
		// Absolute paths — returned as-is (cleaned)
		{
			name:         "absolute path preserved",
			path:         "/etc/passwd",
			workspaceDir: "/workspace",
			want:         "/etc/passwd",
		},
		{
			name:         "absolute path within workspace",
			path:         "/workspace/src/main.go",
			workspaceDir: "/workspace",
			want:         "/workspace/src/main.go",
		},
		{
			name:         "absolute path with trailing slash",
			path:         "/workspace/src/",
			workspaceDir: "/workspace",
			want:         "/workspace/src",
		},
		// Relative paths — joined with workspace
		{
			name:         "relative path joined",
			path:         "src/main.go",
			workspaceDir: "/workspace",
			want:         "/workspace/src/main.go",
		},
		{
			name:         "dot relative path",
			path:         "./main.go",
			workspaceDir: "/workspace",
			want:         "/workspace/main.go",
		},
		{
			name:         "parent relative path escapes",
			path:         "../escape/secret.txt",
			workspaceDir: "/workspace",
			want:         "/escape/secret.txt",
		},
		{
			name:         "path traversal via ..",
			path:         "subdir/../../etc/passwd",
			workspaceDir: "/workspace",
			want:         "/etc/passwd",
		},
		// Empty workspace
		{
			name:         "empty workspace with absolute path",
			path:         "/etc/passwd",
			workspaceDir: "",
			want:         "/etc/passwd",
		},
		{
			name:         "empty workspace with relative path",
			path:         "src/main.go",
			workspaceDir: "",
			want:         "src/main.go", // BUG: raw relative path returned
		},
		// Workspace with trailing slash
		{
			name:         "workspace with trailing slash",
			path:         "src/main.go",
			workspaceDir: "/workspace/",
			want:         "/workspace/src/main.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFilePath(tt.path, tt.workspaceDir)
			if got != tt.want {
				t.Errorf("resolveFilePath(%q, %q) = %q, want %q", tt.path, tt.workspaceDir, got, tt.want)
			}
		})
	}
}

// ── checkFileWorkspaceBounds tests ──

func TestCheckFileWorkspaceBounds(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		workspaceDir string
		wantErr      bool
	}{
		{
			name:         "file inside workspace",
			path:         "/workspace/src/main.go",
			workspaceDir: "/workspace",
			wantErr:      false,
		},
		{
			name:         "workspace root itself",
			path:         "/workspace",
			workspaceDir: "/workspace",
			wantErr:      false,
		},
		{
			name:         "file outside workspace",
			path:         "/etc/passwd",
			workspaceDir: "/workspace",
			wantErr:      true,
		},
		{
			name:         "file in sibling directory",
			path:         "/sibling/secret.txt",
			workspaceDir: "/workspace",
			wantErr:      true,
		},
		{
			name:         "path that starts with workspace prefix but is different dir",
			path:         "/workspace-clone/data.txt",
			workspaceDir: "/workspace",
			wantErr:      true,
		},
		{
			name:         "empty workspace allows any absolute path",
			path:         "/etc/shadow",
			workspaceDir: "",
			wantErr:      false, // BUG: no validation when workspaceDir is empty
		},
		{
			name:         "empty workspace with relative path",
			path:         "../../etc/passwd",
			workspaceDir: "",
			wantErr:      false, // BUG: no validation when workspaceDir is empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkFileWorkspaceBounds(tt.path, tt.workspaceDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkFileWorkspaceBounds(%q, %q) err = %v, wantErr = %v", tt.path, tt.workspaceDir, err, tt.wantErr)
			}
		})
	}
}

// ── readFilePerm NeedPermission tests ──

func TestReadFilePerm_NeedPermission(t *testing.T) {
	tests := []struct {
		name         string
		workspaceDir string
		input        any
		wantNil      bool
		want         *permission.PermissionRequest
	}{
		// Relative path within workspace → safe (level 2)
		{
			name:         "relative path within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "src/main.go"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "读取文件: /workspace/src/main.go",
				ToolName:    "read_file",
				ToolDesc:    "读取文件内容",
				ArgsSummary: "读取 /workspace/src/main.go",
				ToolLevel:   2,
			},
		},
		// Absolute path within workspace → safe (level 2)
		{
			name:         "absolute path within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "/workspace/src/main.go"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "读取文件: /workspace/src/main.go",
				ToolName:    "read_file",
				ToolDesc:    "读取文件内容",
				ArgsSummary: "读取 /workspace/src/main.go",
				ToolLevel:   2,
			},
		},
		// Absolute path outside workspace → high risk (level 4)
		{
			name:         "absolute path outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "/etc/passwd"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "读取文件(路径超出workspace): /etc/passwd",
				ToolName:    "read_file",
				ToolDesc:    "读取文件内容",
				ArgsSummary: "读取 /etc/passwd",
				ToolLevel:   4,
			},
		},
		// Path traversal attempt → detected as outside workspace (level 4)
		{
			name:         "path traversal with ../",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "../../etc/passwd"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "读取文件(路径超出workspace): /etc/passwd",
				ToolName:    "read_file",
				ToolDesc:    "读取文件内容",
				ArgsSummary: "读取 /etc/passwd",
				ToolLevel:   4,
			},
		},
		// BUG: Empty workspaceDir → no bounds check, any file accessible at level 2
		{
			name:         "BUG empty workspace allows any absolute path at low risk",
			workspaceDir: "",
			input:        map[string]any{"file_path": "/etc/passwd"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "读取文件: /etc/passwd", // NOT "路径超出workspace"
				ToolName:    "read_file",
				ToolDesc:    "读取文件内容",
				ArgsSummary: "读取 /etc/passwd",
				ToolLevel:   2, // BUG: should be high risk without workspace bounds
			},
		},
		// nil input
		{
			name:    "nil input",
			input:   nil,
			wantNil: true,
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "读取文件: ",
				ToolName:    "read_file",
				ToolDesc:    "读取文件内容",
				ArgsSummary: "读取 ",
				ToolLevel:   2,
			},
		},
		// Non-map input
		{
			name:    "non-map input",
			input:   "not a map",
			wantNil: true,
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "读取文件: ",
				ToolName:    "read_file",
				ToolDesc:    "读取文件内容",
				ArgsSummary: "读取 ",
				ToolLevel:   2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &readFilePerm{workspaceDir: tt.workspaceDir}
			got := p.NeedPermission(tt.input)

			if tt.wantNil {
				// For nil/non-map inputs the type assertion fails but the code
				// still returns a PermissionRequest with empty path.
				if got == nil {
					return
				}
				// Accept the zero-path result as valid behavior
				if got.Content == "读取文件: " && got.ToolLevel == 2 {
					return
				}
			}

			if got == nil {
				t.Fatalf("NeedPermission() = nil, want non-nil")
			}
			if got.Action != tt.want.Action {
				t.Errorf("Action = %q, want %q", got.Action, tt.want.Action)
			}
			if got.Content != tt.want.Content {
				t.Errorf("Content = %q, want %q", got.Content, tt.want.Content)
			}
			if got.ToolName != tt.want.ToolName {
				t.Errorf("ToolName = %q, want %q", got.ToolName, tt.want.ToolName)
			}
			if got.ToolDesc != tt.want.ToolDesc {
				t.Errorf("ToolDesc = %q, want %q", got.ToolDesc, tt.want.ToolDesc)
			}
			if got.ArgsSummary != tt.want.ArgsSummary {
				t.Errorf("ArgsSummary = %q, want %q", got.ArgsSummary, tt.want.ArgsSummary)
			}
			if got.ToolLevel != tt.want.ToolLevel {
				t.Errorf("ToolLevel = %d, want %d", got.ToolLevel, tt.want.ToolLevel)
			}
		})
	}
}

// ── readFilePerm InvokableRun tests ──

func TestReadFilePerm_InvokableRun(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	subFile := filepath.Join(subDir, "nested.txt")
	if err := os.WriteFile(subFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		workspaceDir string
		inputJSON    string
		wantContains string
		wantErr      bool
	}{
		{
			name:         "read existing file",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "hello.txt"}`,
			wantContains: "hello world",
		},
		{
			name:         "read existing file with absolute path",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "` + testFile + `"}`,
			wantContains: "hello world",
		},
		{
			name:         "read nested file",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "sub/nested.txt"}`,
			wantContains: "line1",
		},
		{
			name:         "read with offset",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "sub/nested.txt", "offset": 2, "limit": 1}`,
			wantContains: "line2",
		},
		{
			name:         "read non-existent file",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "nonexistent.txt"}`,
			wantContains: "file not found",
		},
		{
			name:         "read file outside workspace via absolute path",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "/etc/passwd"}`,
			wantContains: "outside workspace",
		},
		{
			name:         "invalid JSON input",
			workspaceDir: tmpDir,
			inputJSON:    `not-json`,
			wantContains: "error",
		},
		{
			name:         "BUG empty workspace reads any file",
			workspaceDir: "",
			inputJSON:    `{"file_path": "/etc/passwd"}`,
			wantContains: "", // Will succeed reading /etc/passwd — this is the bug
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &readFilePerm{workspaceDir: tt.workspaceDir}
			result, err := p.InvokableRun(context.Background(), tt.inputJSON)
			if err != nil && !tt.wantErr {
				t.Fatalf("InvokableRun() unexpected error = %v", err)
			}
			if tt.wantContains != "" && !strings.Contains(result, tt.wantContains) {
				t.Errorf("InvokableRun() = %q, want to contain %q", result, tt.wantContains)
			}
		})
	}
}

// ── readFilePerm Info tests ──

func TestReadFilePerm_Info(t *testing.T) {
	p := &readFilePerm{}
	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "read_file" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "read_file")
	}
}

// ── writeFilePerm NeedPermission tests ──

func TestWriteFilePerm_NeedPermission(t *testing.T) {
	tests := []struct {
		name         string
		workspaceDir string
		input        any
		want         *permission.PermissionRequest
	}{
		{
			name:         "write within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "output.txt", "content": "data"},
			want: &permission.PermissionRequest{
				Action:      "write",
				Content:     "创建/覆盖文件: /workspace/output.txt",
				ToolName:    "write_file",
				ToolDesc:    "创建新文件或覆盖现有文件",
				ArgsSummary: "写入 /workspace/output.txt",
				ToolLevel:   3,
			},
		},
		{
			name:         "write outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "/tmp/evil.txt", "content": "data"},
			want: &permission.PermissionRequest{
				Action:      "write",
				Content:     "创建/覆盖文件(路径超出workspace): /tmp/evil.txt",
				ToolName:    "write_file",
				ToolDesc:    "创建新文件或覆盖现有文件",
				ArgsSummary: "写入 /tmp/evil.txt",
				ToolLevel:   4,
			},
		},
		{
			name:         "BUG empty workspace allows any write at level 3",
			workspaceDir: "",
			input:        map[string]any{"file_path": "/etc/evil.txt", "content": "data"},
			want: &permission.PermissionRequest{
				Action:      "write",
				Content:     "创建/覆盖文件: /etc/evil.txt",
				ToolName:    "write_file",
				ToolDesc:    "创建新文件或覆盖现有文件",
				ArgsSummary: "写入 /etc/evil.txt",
				ToolLevel:   3, // BUG: no bounds check
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &writeFilePerm{workspaceDir: tt.workspaceDir}
			got := p.NeedPermission(tt.input)
			if got == nil {
				t.Fatalf("NeedPermission() = nil, want non-nil")
			}
			if got.Action != tt.want.Action {
				t.Errorf("Action = %q, want %q", got.Action, tt.want.Action)
			}
			if got.Content != tt.want.Content {
				t.Errorf("Content = %q, want %q", got.Content, tt.want.Content)
			}
			if got.ToolLevel != tt.want.ToolLevel {
				t.Errorf("ToolLevel = %d, want %d", got.ToolLevel, tt.want.ToolLevel)
			}
		})
	}
}

// ── writeFilePerm InvokableRun tests ──

func TestWriteFilePerm_InvokableRun(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		workspaceDir string
		inputJSON    string
		wantContains string
	}{
		{
			name:         "write new file",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "new.txt", "content": "hello"}`,
			wantContains: "Successfully wrote",
		},
		{
			name:         "write with absolute path inside workspace",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "` + filepath.Join(tmpDir, "abs.txt") + `", "content": "abs"}`,
			wantContains: "Successfully wrote",
		},
		{
			name:         "write outside workspace denied",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "/tmp/outside.txt", "content": "evil"}`,
			wantContains: "outside workspace",
		},
		{
			name:         "invalid JSON",
			workspaceDir: tmpDir,
			inputJSON:    `bad-json`,
			wantContains: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &writeFilePerm{workspaceDir: tt.workspaceDir}
			result, err := p.InvokableRun(context.Background(), tt.inputJSON)
			if err != nil {
				t.Fatalf("InvokableRun() unexpected error = %v", err)
			}
			if !strings.Contains(result, tt.wantContains) {
				t.Errorf("InvokableRun() = %q, want to contain %q", result, tt.wantContains)
			}
		})
	}
}

// ── writeFilePerm Info tests ──

func TestWriteFilePerm_Info(t *testing.T) {
	p := &writeFilePerm{}
	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "write_file" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "write_file")
	}
}

// ── editFilePerm NeedPermission tests ──

func TestEditFilePerm_NeedPermission(t *testing.T) {
	tests := []struct {
		name         string
		workspaceDir string
		input        any
		want         *permission.PermissionRequest
	}{
		{
			name:         "edit within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "src/main.go", "old_string": "foo", "new_string": "bar"},
			want: &permission.PermissionRequest{
				Action:      "write",
				Content:     "修改文件: /workspace/src/main.go",
				ToolName:    "edit_file",
				ToolDesc:    "在文件中查找并替换文本",
				ArgsSummary: "编辑 /workspace/src/main.go",
				ToolLevel:   3,
			},
		},
		{
			name:         "edit outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"file_path": "/etc/hosts", "old_string": "old", "new_string": "new"},
			want: &permission.PermissionRequest{
				Action:      "write",
				Content:     "修改文件(路径超出workspace): /etc/hosts",
				ToolName:    "edit_file",
				ToolDesc:    "在文件中查找并替换文本",
				ArgsSummary: "编辑 /etc/hosts",
				ToolLevel:   4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &editFilePerm{workspaceDir: tt.workspaceDir}
			got := p.NeedPermission(tt.input)
			if got == nil {
				t.Fatalf("NeedPermission() = nil, want non-nil")
			}
			if got.Content != tt.want.Content {
				t.Errorf("Content = %q, want %q", got.Content, tt.want.Content)
			}
			if got.ToolLevel != tt.want.ToolLevel {
				t.Errorf("ToolLevel = %d, want %d", got.ToolLevel, tt.want.ToolLevel)
			}
		})
	}
}

// ── editFilePerm InvokableRun tests ──

func TestEditFilePerm_InvokableRun(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "edit.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		workspaceDir string
		inputJSON    string
		wantContains string
	}{
		{
			name:         "edit existing file",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "edit.txt", "old_string": "world", "new_string": "go"}`,
			wantContains: "Replaced 1 occurrence",
		},
		{
			name:         "edit with absolute path inside workspace",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "` + testFile + `", "old_string": "hello", "new_string": "hi"}`,
			wantContains: "Replaced 1 occurrence",
		},
		{
			name:         "edit file outside workspace denied",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "/etc/hosts", "old_string": "x", "new_string": "y"}`,
			wantContains: "outside workspace",
		},
		{
			name:         "edit non-existent file",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "nope.txt", "old_string": "x", "new_string": "y"}`,
			wantContains: "failed to read file",
		},
		{
			name:         "old_string not found",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "edit.txt", "old_string": "zzz_not_found", "new_string": "y"}`,
			wantContains: "string not found",
		},
		{
			name:         "empty old_string",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "edit.txt", "old_string": "", "new_string": "y"}`,
			wantContains: "old_string is required",
		},
		{
			name:         "multiple occurrences without replace_all",
			workspaceDir: tmpDir,
			inputJSON:    `{"file_path": "multi.txt", "old_string": "foo", "new_string": "bar"}`,
			wantContains: "times",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset files for tests that need them
			switch tt.name {
			case "edit existing file":
				os.WriteFile(filepath.Join(tmpDir, "edit.txt"), []byte("hello world"), 0644)
			case "multiple occurrences without replace_all":
				os.WriteFile(filepath.Join(tmpDir, "multi.txt"), []byte("foo foo foo"), 0644)
			}
			p := &editFilePerm{workspaceDir: tt.workspaceDir}
			result, err := p.InvokableRun(context.Background(), tt.inputJSON)
			if err != nil {
				t.Fatalf("InvokableRun() unexpected error = %v", err)
			}
			if !strings.Contains(result, tt.wantContains) {
				t.Errorf("InvokableRun() = %q, want to contain %q", result, tt.wantContains)
			}
		})
	}
}

// ── editFilePerm Info tests ──

func TestEditFilePerm_Info(t *testing.T) {
	p := &editFilePerm{}
	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "edit_file" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "edit_file")
	}
}

// ── globPerm NeedPermission tests ──

func TestGlobPerm_NeedPermission(t *testing.T) {
	tests := []struct {
		name         string
		workspaceDir string
		input        any
		want         *permission.PermissionRequest
	}{
		{
			name:         "glob within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"path": "src", "pattern": "*.go"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "glob 查找: /workspace/src *.go",
				ToolName:    "glob",
				ToolDesc:    "使用 glob 模式递归查找文件",
				ArgsSummary: "glob: *.go in /workspace/src",
				ToolLevel:   1,
			},
		},
		{
			name:         "glob outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"path": "/etc", "pattern": "*.conf"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "glob 查找(路径超出workspace): /etc *.conf",
				ToolName:    "glob",
				ToolDesc:    "使用 glob 模式递归查找文件",
				ArgsSummary: "glob: *.conf in /etc",
				ToolLevel:   4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &globPerm{workspaceDir: tt.workspaceDir}
			got := p.NeedPermission(tt.input)
			if got == nil {
				t.Fatalf("NeedPermission() = nil, want non-nil")
			}
			if got.Content != tt.want.Content {
				t.Errorf("Content = %q, want %q", got.Content, tt.want.Content)
			}
			if got.ToolLevel != tt.want.ToolLevel {
				t.Errorf("ToolLevel = %d, want %d", got.ToolLevel, tt.want.ToolLevel)
			}
		})
	}
}

// ── globPerm InvokableRun tests ──

func TestGlobPerm_InvokableRun(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src", "pkg"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "src", "main.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "src", "pkg", "util.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(""), 0644)

	tests := []struct {
		name         string
		workspaceDir string
		inputJSON    string
		wantContains string
	}{
		{
			name:         "glob *.go recursive",
			workspaceDir: tmpDir,
			inputJSON:    `{"path": "src", "pattern": "**/*.go"}`,
			wantContains: "main.go",
		},
		{
			name:         "glob outside workspace denied",
			workspaceDir: tmpDir,
			inputJSON:    `{"path": "/etc", "pattern": "*.conf"}`,
			wantContains: "outside workspace",
		},
		{
			name:         "invalid JSON",
			workspaceDir: tmpDir,
			inputJSON:    `bad`,
			wantContains: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &globPerm{workspaceDir: tt.workspaceDir}
			result, err := p.InvokableRun(context.Background(), tt.inputJSON)
			if err != nil {
				t.Fatalf("InvokableRun() unexpected error = %v", err)
			}
			if !strings.Contains(result, tt.wantContains) {
				t.Errorf("InvokableRun() = %q, want to contain %q", result, tt.wantContains)
			}
		})
	}
}

// ── globPerm Info tests ──

func TestGlobPerm_Info(t *testing.T) {
	p := &globPerm{}
	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "glob" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "glob")
	}
}

// ── grepPerm NeedPermission tests ──

func TestGrepPerm_NeedPermission(t *testing.T) {
	tests := []struct {
		name         string
		workspaceDir string
		input        any
		want         *permission.PermissionRequest
	}{
		{
			name:         "grep within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"path": "src", "pattern": "TODO", "case_insensitive": false},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "搜索文件内容: TODO in /workspace/src",
				ToolName:    "grep",
				ToolDesc:    "使用 ripgrep 在文件中搜索内容",
				ArgsSummary: "grep 'TODO' in /workspace/src",
				ToolLevel:   2,
			},
		},
		{
			name:         "grep outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"path": "/etc", "pattern": "password"},
			want: &permission.PermissionRequest{
				Action:      "read",
				Content:     "搜索文件内容(路径超出workspace): password in /etc",
				ToolName:    "grep",
				ToolDesc:    "使用 ripgrep 在文件中搜索内容",
				ArgsSummary: "grep 'password' in /etc",
				ToolLevel:   4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &grepPerm{workspaceDir: tt.workspaceDir}
			got := p.NeedPermission(tt.input)
			if got == nil {
				t.Fatalf("NeedPermission() = nil, want non-nil")
			}
			if got.Content != tt.want.Content {
				t.Errorf("Content = %q, want %q", got.Content, tt.want.Content)
			}
			if got.ToolLevel != tt.want.ToolLevel {
				t.Errorf("ToolLevel = %d, want %d", got.ToolLevel, tt.want.ToolLevel)
			}
		})
	}
}

// ── grepPerm InvokableRun tests ──

func TestGrepPerm_InvokableRun(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("// TODO: fix this\n"), 0644)

	tests := []struct {
		name         string
		workspaceDir string
		inputJSON    string
		wantContains string
	}{
		{
			name:         "grep finds pattern",
			workspaceDir: tmpDir,
			inputJSON:    `{"path": ".", "pattern": "TODO"}`,
			wantContains: "TODO",
		},
		{
			name:         "grep no matches",
			workspaceDir: tmpDir,
			inputJSON:    `{"path": ".", "pattern": "NOTFOUND_XYZ"}`,
			wantContains: "No matches found",
		},
		{
			name:         "grep outside workspace denied",
			workspaceDir: tmpDir,
			inputJSON:    `{"path": "/etc", "pattern": "root"}`,
			wantContains: "outside workspace",
		},
		{
			name:         "empty pattern",
			workspaceDir: tmpDir,
			inputJSON:    `{"pattern": ""}`,
			wantContains: "pattern is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &grepPerm{workspaceDir: tt.workspaceDir}
			result, err := p.InvokableRun(context.Background(), tt.inputJSON)
			if err != nil {
				t.Fatalf("InvokableRun() unexpected error = %v", err)
			}
			if !strings.Contains(result, tt.wantContains) {
				t.Errorf("InvokableRun() = %q, want to contain %q", result, tt.wantContains)
			}
		})
	}
}

// ── grepPerm Info tests ──

func TestGrepPerm_Info(t *testing.T) {
	p := &grepPerm{}
	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "grep" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "grep")
	}
}

// ── NewFilesystemTools integration test ──

func TestNewFilesystemTools(t *testing.T) {
	tools := NewFilesystemTools("/workspace")
	if len(tools) != 6 {
		t.Fatalf("NewFilesystemTools() returned %d tools, want 6", len(tools))
	}

	expectedNames := []string{"read_file", "write_file", "edit_file", "glob", "grep", "execute"}
	for i, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool[%d].Info() error = %v", i, err)
		}
		if info.Name != expectedNames[i] {
			t.Errorf("tool[%d].Name = %q, want %q", i, info.Name, expectedNames[i])
		}
	}

	// Verify all tools implement NeedPermissioner
	for i, tool := range tools {
		if _, ok := tool.(permission.NeedPermissioner); !ok {
			t.Errorf("tool[%d] (%s) does not implement NeedPermissioner", i, expectedNames[i])
		}
	}
}

// ── Consistency: NeedPermission vs InvokableRun workspace bounds ──

// TestNeedPermissionMatchesInvokableRunBounds verifies that NeedPermission and
// InvokableRun agree on whether a path is within workspace bounds. If
// NeedPermission returns a low-risk level (path inside workspace) but
// InvokableRun would reject the same path, the user gets a confusing experience.
func TestNeedPermissionMatchesInvokableRunBounds(t *testing.T) {
	tmpDir := t.TempDir()
	outsidePath := "/etc/passwd"

	testCases := []struct {
		name      string
		filePath  string
		inputJSON string
	}{
		{
			name:      "read_file absolute outside",
			filePath:  outsidePath,
			inputJSON: `{"file_path": "/etc/passwd"}`,
		},
		{
			name:      "write_file absolute outside",
			filePath:  outsidePath,
			inputJSON: `{"file_path": "/tmp/evil.txt", "content": "x"}`,
		},
		{
			name:      "edit_file absolute outside",
			filePath:  outsidePath,
			inputJSON: `{"file_path": "/etc/hosts", "old_string": "x", "new_string": "y"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// read_file
			rp := &readFilePerm{workspaceDir: tmpDir}
			rpReq := rp.NeedPermission(map[string]any{"file_path": tc.filePath})
			if rpReq != nil && strings.Contains(rpReq.Content, "路径超出workspace") {
				// NeedPermission flags it as outside — InvokableRun should also reject
				rpResult, _ := rp.InvokableRun(context.Background(), tc.inputJSON)
				if !strings.Contains(rpResult, "outside workspace") {
					t.Errorf("read_file: NeedPermission flags path as outside but InvokableRun result = %q", rpResult)
				}
			}

			// write_file
			wp := &writeFilePerm{workspaceDir: tmpDir}
			wpReq := wp.NeedPermission(map[string]any{"file_path": tc.filePath, "content": "x"})
			if wpReq != nil && strings.Contains(wpReq.Content, "路径超出workspace") {
				wpResult, _ := wp.InvokableRun(context.Background(), tc.inputJSON)
				if !strings.Contains(wpResult, "outside workspace") {
					t.Errorf("write_file: NeedPermission flags path as outside but InvokableRun result = %q", wpResult)
				}
			}

			// edit_file
			ep := &editFilePerm{workspaceDir: tmpDir}
			epReq := ep.NeedPermission(map[string]any{"file_path": tc.filePath, "old_string": "x", "new_string": "y"})
			if epReq != nil && strings.Contains(epReq.Content, "路径超出workspace") {
				epResult, _ := ep.InvokableRun(context.Background(), tc.inputJSON)
				if !strings.Contains(epResult, "outside workspace") {
					t.Errorf("edit_file: NeedPermission flags path as outside but InvokableRun result = %q", epResult)
				}
			}
		})
	}
}

// ── Helper: unexported function access via direct call ──

func TestGetStringAny(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want string
	}{
		{"existing key", map[string]any{"name": "test"}, "name", "test"},
		{"missing key", map[string]any{"name": "test"}, "age", ""},
		{"nil map", nil, "name", ""},
		{"non-string value", map[string]any{"count": 42}, "count", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStringAny(tt.m, tt.key)
			if got != tt.want {
				t.Errorf("getStringAny(%v, %q) = %q, want %q", tt.m, tt.key, got, tt.want)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

// ── Symlink escape test (demonstrates TOCTOU vulnerability) ──

func TestReadFilePerm_SymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file outside the workspace.
	outsideFile := filepath.Join(tmpDir, "outside", "secret.txt")
	os.MkdirAll(filepath.Dir(outsideFile), 0755)
	os.WriteFile(outsideFile, []byte("secret content"), 0644)

	// Create a symlink inside the workspace pointing outside.
	linkPath := filepath.Join(tmpDir, "link_to_secret.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// The bounds check sees /tmp/.../link_to_secret.txt (inside workspace).
	// But the actual read follows the symlink to /tmp/.../outside/secret.txt.
	p := &readFilePerm{workspaceDir: tmpDir}
	result, err := p.InvokableRun(context.Background(), `{"file_path": "link_to_secret.txt"}`)
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}

	// This demonstrates the bug: the tool reads secret content despite
	// the path appearing to be inside the workspace.
	if strings.Contains(result, "secret content") {
		t.Logf("BUG DEMONSTRATED: symlink escape succeeded — read content outside workspace: %q", result)
	} else if strings.Contains(result, "file not found") {
		t.Skip("symlink resolution failed (platform-dependent)")
	}
	// Note: this test is informational — it shows the TOCTOU gap.
}

// ── JSON roundtrip test for tool inputs ──

func TestReadFileInput_JSONRoundtrip(t *testing.T) {
	original := readFileInput{
		FilePath: "/path/to/file.txt",
		Offset:   10,
		Limit:    50,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}

	var decoded readFileInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if decoded.FilePath != original.FilePath {
		t.Errorf("FilePath = %q, want %q", decoded.FilePath, original.FilePath)
	}
	if decoded.Offset != original.Offset {
		t.Errorf("Offset = %d, want %d", decoded.Offset, original.Offset)
	}
	if decoded.Limit != original.Limit {
		t.Errorf("Limit = %d, want %d", decoded.Limit, original.Limit)
	}
}

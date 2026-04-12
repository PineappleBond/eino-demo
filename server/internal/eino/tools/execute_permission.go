package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
)

// ── execute ──

type executeInput struct {
	Command string `json:"command" jsonschema_description:"Shell command to execute"`
}

type executePerm struct{ workspaceDir string }

func (p *executePerm) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "execute",
		Desc: "Execute a shell command and return the output",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {Type: schema.String, Desc: "Shell command to execute", Required: true},
		}),
	}, nil
}

func (p *executePerm) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input executeInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return fmt.Sprintf("<tool_error>\n%s\n</tool_error>", err.Error()), nil
	}

	if input.Command == "" {
		return fmt.Sprintf("<tool_error>\ncommand is required\n</tool_error>"), nil
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", input.Command)
	if p.workspaceDir != "" {
		cmd.Dir = p.workspaceDir
	}
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	result := out.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Sprintf("Command exited with code %d:\n%s", exitErr.ExitCode(), result), nil
		}
		return fmt.Sprintf("<tool_error>\ncommand failed: %s\n</tool_error>", err.Error()), nil
	}

	if result == "" {
		return "Command executed successfully (no output)", nil
	}
	return result, nil
}

func (p *executePerm) StreamableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
	sr, sw := schema.Pipe[string](1)
	go func() {
		defer sw.Close()
		result, _ := p.InvokableRun(ctx, argumentsInJSON, opts...)
		sw.Send(result, nil)
	}()
	return sr, nil
}

func (p *executePerm) NeedPermission(input any) *permission.PermissionRequest {
	args, _ := input.(map[string]any)
	command := getStringAny(args, "command")

	if command == "" {
		return nil
	}

	result := analyzeCommand(command, p.workspaceDir)
	if result == nil {
		return nil
	}
	return result
}

// commandAnalysisResult holds the result of analyzing a shell command.
type commandAnalysisResult struct {
	action      string
	content     string
	argsSummary string
	riskLevel   int
	isCompound  bool
}

// safeNoPathCommands are commands that don't touch the filesystem at all.
// They are always safe regardless of workspaceDir.
var safeNoPathCommands = map[string]bool{
	"echo":  true,
	"date":  true,
	"whoami": true,
	"uname": true,
	"id":    true,
	"true":  true,
	"false": true,
	"sleep": true,
	"pwd":   true,
}

// safeReadCommands are read-only filesystem commands that are low-risk
// when paths are within the workspace.
var safeReadCommands = map[string]bool{
	"ls":   true,
	"cat":  true,
	"head": true,
	"tail": true,
	"wc":   true,
	"file": true,
	"stat": true,
}

// writeCommands modify the filesystem and require permission approval
// even when paths are within the workspace.
var writeCommands = map[string]bool{
	"mkdir":  true,
	"touch":  true,
	"cp":     true,
	"mv":     true,
	"chmod":  true,
	"chown":  true,
}

// systemReadCommands are system information commands that may read outside
// the workspace but are generally safe.
var systemReadCommands = map[string]bool{
	"find": true,
	"df":   true,
	"du":   true,
	"grep": true,
}

// dangerousCommands are always blocked regardless of workspace.
var dangerousCommands = map[string]bool{
	"rm":     true,
	"dd":     true,
	"mkfs":   true,
	"fdisk":  true,
	"curl":   true,
	"wget":   true,
	"ssh":    true,
	"scp":    true,
	"rsync":  true,
	"git":    true,
}

// isDangerousCommand checks if a command is dangerous, including prefix matches
// like mkfs.ext4 → mkfs.
func isDangerousCommand(cmd string) bool {
	if dangerousCommands[cmd] {
		return true
	}
	// Check prefix matches: mkfs.ext4 → mkfs
	for d := range dangerousCommands {
		if len(cmd) > len(d) && cmd[len(d)] == '.' && cmd[:len(d)] == d {
			return true
		}
	}
	return false
}

// analyzeCommand parses the shell command using mvdan/sh AST and analyzes it for:
// 1. Compound commands (&&, ||, |, ;) → always high risk
// 2. Dangerous commands → always blocked
// 3. Safe no-path commands → allowed without permission
// 4. Path-based commands → check if paths are within workspace
// 5. Everything else → medium risk, needs LLM review
//
// Returns nil when the command is safe to execute without permission.
// Returns a PermissionRequest when the command needs approval.
func analyzeCommand(command string, workspaceDir string) *permission.PermissionRequest {
	f, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		// Parse error → let LLM decide
		return &permission.PermissionRequest{
			Action:      "unknown",
			Content:     "执行命令(无法解析): " + command,
			ToolName:    "execute",
			ToolDesc:    "执行 shell 命令",
			ArgsSummary: truncateString(command, 200),
			ToolLevel:   3,
		}
	}

	// Multiple statements (semicolon-separated): check each one individually
	if len(f.Stmts) > 1 {
		highestRisk := 0
		var issues []string
		for _, s := range f.Stmts {
			// Extract statement text from source position
			stmtText := ""
			if s.Pos().IsValid() && s.End().IsValid() {
				start := s.Pos().Offset()
				end := s.End().Offset()
				if start < end && end <= uint(len(command)) {
					stmtText = strings.TrimSpace(command[start:end])
					// Strip trailing semicolon/ampersand from the statement text
					stmtText = strings.TrimRight(stmtText, ";&")
				}
			}
			if stmtText == "" {
				stmtText = command
			}

			if s.Background || s.Negated {
				return &permission.PermissionRequest{
					Action:      "compound",
					Content:     "执行复合命令(含后台/否定): " + command,
					ToolName:    "execute",
					ToolDesc:    "执行 shell 命令",
					ArgsSummary: truncateString(command, 200),
					ToolLevel:   4,
				}
			}
			if s.Cmd == nil {
				continue
			}
			result := analyzeNode(s.Cmd, workspaceDir, stmtText)
			if result == nil {
				continue
			}
			if result.riskLevel > highestRisk {
				highestRisk = result.riskLevel
			}
			issues = append(issues, result.content)
		}
		if highestRisk == 0 {
			return nil
		}
		return &permission.PermissionRequest{
			Action:      "multi",
			Content:     "执行多命令: " + strings.Join(issues, "; "),
			ToolName:    "execute",
			ToolDesc:    "执行 shell 命令",
			ArgsSummary: truncateString(command, 200),
			ToolLevel:   highestRisk,
		}
	}

	if len(f.Stmts) == 0 {
		return nil
	}

	stmt := f.Stmts[0]

	// Background command (& at the end)
	if stmt.Background {
		return &permission.PermissionRequest{
			Action:      "compound",
			Content:     "执行后台命令: " + command,
			ToolName:    "execute",
			ToolDesc:    "执行 shell 命令",
			ArgsSummary: truncateString(command, 200),
			ToolLevel:   4,
		}
	}

	// Negated command (! cmd)
	if stmt.Negated {
		return &permission.PermissionRequest{
			Action:      "compound",
			Content:     "执行否定命令: " + command,
			ToolName:    "execute",
			ToolDesc:    "执行 shell 命令",
			ArgsSummary: truncateString(command, 200),
			ToolLevel:   4,
		}
	}

	// Check the command type
	if cmd := stmt.Cmd; cmd != nil {
		result := analyzeNode(cmd, workspaceDir, command)
		if result == nil {
			return nil
		}
		return &permission.PermissionRequest{
			Action:      result.action,
			Content:     result.content,
			ToolName:    "execute",
			ToolDesc:    "执行 shell 命令",
			ArgsSummary: result.argsSummary,
			ToolLevel:   result.riskLevel,
		}
	}

	return &permission.PermissionRequest{
		Action:      "unknown",
		Content:     "执行命令: " + command,
		ToolName:    "execute",
		ToolDesc:    "执行 shell 命令",
		ArgsSummary: truncateString(command, 200),
		ToolLevel:   3,
	}
}

// analyzeNode recursively analyzes an AST node for command safety.
// Returns nil when safe, or a result when permission is needed.
func analyzeNode(node syntax.Node, workspaceDir string, originalCmd string) *commandAnalysisResult {
	switch n := node.(type) {
	case *syntax.CallExpr:
		return analyzeCallExpr(n, workspaceDir, originalCmd)

	case *syntax.BinaryCmd:
		// BinaryCmd covers &&, ||, and |
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行复合命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
			isCompound:  true,
		}

	case *syntax.IfClause:
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行条件命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
		}

	case *syntax.Subshell:
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行子shell: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
		}

	case *syntax.Block:
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行命令块: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
		}

	case *syntax.ForClause:
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行循环命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
		}

	case *syntax.WhileClause:
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行循环命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
		}

	case *syntax.FuncDecl:
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行函数声明: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
		}

	case *syntax.ArithmCmd:
		return &commandAnalysisResult{
			action:      "compound",
			content:     "执行算术命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   3,
		}

	default:
		// Unknown node type → medium risk, let LLM decide
		return &commandAnalysisResult{
			action:      "unknown",
			content:     "执行命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   3,
		}
	}
}

// analyzeCallExpr analyzes a simple command (CallExpr) for safety.
func analyzeCallExpr(call *syntax.CallExpr, workspaceDir string, originalCmd string) *commandAnalysisResult {
	if len(call.Args) == 0 {
		// Just variable assignments like `FOO=bar`
		return nil
	}

	// Extract the command name
	cmdName := wordToLiteral(call.Args[0])
	if cmdName == "" {
		// Complex command name (e.g., variable expansion) → let LLM decide
		return &commandAnalysisResult{
			action:      "unknown",
			content:     "执行命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   3,
		}
	}

	// Check if command requires no filesystem access
	if safeNoPathCommands[cmdName] {
		return nil
	}

	// Check if command is always dangerous (including prefix matches like mkfs.ext4)
	if isDangerousCommand(cmdName) {
		return &commandAnalysisResult{
			action:      cmdName,
			content:     "执行危险命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   4,
		}
	}

	// Check if command writes to filesystem
	if writeCommands[cmdName] {
		paths := extractPaths(call, workspaceDir)
		outOfBounds := checkPathsOutOfBounds(paths, workspaceDir)
		if len(outOfBounds) > 0 {
			return &commandAnalysisResult{
				action:      cmdName,
				content:     fmt.Sprintf("执行写入命令(路径超出workspace): %s, 危险路径: %s", originalCmd, strings.Join(outOfBounds, ", ")),
				argsSummary: truncateString(originalCmd, 200),
				riskLevel:   4,
			}
		}
		return &commandAnalysisResult{
			action:      cmdName,
			content:     "执行写入命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   3,
		}
	}

	// Check if command is safe read-only (within workspace)
	if safeReadCommands[cmdName] {
		paths := extractPaths(call, workspaceDir)
		outOfBounds := checkPathsOutOfBounds(paths, workspaceDir)
		if len(outOfBounds) > 0 {
			return &commandAnalysisResult{
				action:      cmdName,
				content:     fmt.Sprintf("执行读取命令(路径超出workspace): %s, 危险路径: %s", originalCmd, strings.Join(outOfBounds, ", ")),
				argsSummary: truncateString(originalCmd, 200),
				riskLevel:   3,
			}
		}
		// All paths within workspace → safe
		return nil
	}

	// System read commands (may read outside workspace)
	if systemReadCommands[cmdName] {
		paths := extractPaths(call, workspaceDir)
		outOfBounds := checkPathsOutOfBounds(paths, workspaceDir)
		if len(outOfBounds) > 0 {
			return &commandAnalysisResult{
				action:      cmdName,
				content:     fmt.Sprintf("执行系统命令(路径超出workspace): %s, 危险路径: %s", originalCmd, strings.Join(outOfBounds, ", ")),
				argsSummary: truncateString(originalCmd, 200),
				riskLevel:   3,
			}
		}
		// System command within workspace → medium risk
		return &commandAnalysisResult{
			action:      cmdName,
			content:     "执行系统命令: " + originalCmd,
			argsSummary: truncateString(originalCmd, 200),
			riskLevel:   2,
		}
	}

	// Unknown command → let LLM decide
	return &commandAnalysisResult{
		action:      cmdName,
		content:     "执行未知命令: " + originalCmd,
		argsSummary: truncateString(originalCmd, 200),
		riskLevel:   3,
	}
}

// wordToLiteral extracts a literal string from a Word node if it's a simple literal.
// Returns empty string for complex words (containing variables, expansions, etc).
func wordToLiteral(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	return w.Lit()
}

// extractPaths extracts file path arguments from a CallExpr.
// It skips flags (starting with -) and assignment-like arguments (=).
func extractPaths(call *syntax.CallExpr, workspaceDir string) []string {
	var paths []string
	for i, arg := range call.Args {
		if i == 0 {
			continue // skip command name
		}
		word := arg.Lit()
		if word == "" {
			// Complex word (variable, etc) → skip, can't determine
			continue
		}
		// Skip flags
		if strings.HasPrefix(word, "-") {
			continue
		}
		// Skip assignments
		if strings.Contains(word, "=") {
			continue
		}
		// Skip numbers
		if isNumber(word) {
			continue
		}
		// Resolve relative paths
		resolved := resolvePath(word, workspaceDir)
		if resolved != "" {
			paths = append(paths, resolved)
		}
	}
	return paths
}

// isNumber checks if a string is purely numeric.
func isNumber(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// resolvePath resolves a relative path against the workspace directory.
// Returns empty string if the path cannot be resolved.
func resolvePath(path string, workspaceDir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if workspaceDir == "" {
		return ""
	}
	return filepath.Clean(filepath.Join(workspaceDir, path))
}

// checkPathsOutOfBounds checks if any paths are outside the workspace.
// Returns a list of paths that are out of bounds.
func checkPathsOutOfBounds(paths []string, workspaceDir string) []string {
	if workspaceDir == "" {
		// No workspace configured → all absolute paths are potentially dangerous
		var outOfBounds []string
		for _, p := range paths {
			if filepath.IsAbs(p) {
				outOfBounds = append(outOfBounds, p)
			}
		}
		return outOfBounds
	}

	cleanWorkspace := filepath.Clean(workspaceDir)
	var outOfBounds []string
	for _, p := range paths {
		cleanPath := filepath.Clean(p)
		if !strings.HasPrefix(cleanPath, cleanWorkspace+string(filepath.Separator)) && cleanPath != cleanWorkspace {
			outOfBounds = append(outOfBounds, p)
		}
	}
	return outOfBounds
}

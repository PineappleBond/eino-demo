package permission

import (
	"bytes"
	"text/template"
)

const evaluatorPromptTemplate = `你是一个安全评估助手。你的任务是评估一个 AI Agent 即将执行的工具调用是否需要人类事先知情同意。

## 评估规则

安全等级分为 4 级：
- 等级 1 (Safe): 纯只读操作，访问公开信息，无任何副作用
- 等级 2 (Low): 读取操作，但可能涉及用户私人数据（如读取文件内容、列出目录）
- 等级 3 (Medium): 写入或修改操作（如创建/编辑文件、修改配置、发送只读请求以外的 HTTP）
- 等级 4 (High): 执行系统命令、不可逆操作、可能造成数据丢失的操作

## 输出格式

只输出 JSON，不要输出其他内容：
{"safety_level": <1-4>, "reason": "<一句话说明理由>"}

## 上下文

工具名称: {{.ToolName}}
工具用途: {{.ToolDesc}}
操作类型: {{.Action}}
操作内容: {{.Content}}
调用参数: {{.ArgsSummary}}
工具自评: {{.ToolLevel}} (0表示不自评)
`

type promptData struct {
	ToolName    string
	ToolDesc    string
	Action      string
	Content     string
	ArgsSummary string
	ToolLevel   int
}

func renderEvaluatorPrompt(req *PermissionRequest) (string, error) {
	tpl, err := template.New("evaluator").Parse(evaluatorPromptTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, promptData{
		ToolName:    req.ToolName,
		ToolDesc:    req.ToolDesc,
		Action:      req.Action,
		Content:     req.Content,
		ArgsSummary: req.ArgsSummary,
		ToolLevel:   req.ToolLevel,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

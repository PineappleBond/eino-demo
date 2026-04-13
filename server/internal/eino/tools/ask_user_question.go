package tools

import (
	"context"
	"encoding/gob"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func init() {
	// Eino's checkpoint serialization uses gob. When tool.Interrupt is called
	// with map[string]any containing []HitlChoice, gob needs to know the type.
	gob.Register(HitlChoice{})
	gob.Register([]HitlChoice{})
}

// HitlChoice represents a single choice option for the ask_user_question tool.
type HitlChoice struct {
	Title string `json:"title" jsonschema_description:"The short label shown as the choice title"`
	Desc  string `json:"desc,omitempty" jsonschema_description:"A short description shown below the title"`
}

// AskUserQuestionInput is the input schema for the ask_user_question tool.
type AskUserQuestionInput struct {
	Question   string      `json:"question" jsonschema_description:"The question to ask the user"`
	Choices    []HitlChoice `json:"choices,omitempty" jsonschema_description:"Optional predefined choices. If empty, the user can type free text"`
	AnswerType string      `json:"answer_type,omitempty" jsonschema_description:"How the user should answer: 'single' (radio), 'multi' (checkbox), or 'text' (free input). Defaults to 'text'"`
}

// AskUserQuestionOutput is the output schema for the ask_user_question tool.
type AskUserQuestionOutput struct {
	Answer string `json:"answer" jsonschema_description:"The user's answer (the title of the selected choice, or free text)"`
}

// NewAskUserQuestionTool creates a tool that interrupts agent execution to ask the user a question.
// The agent resumes with the user's answer after they respond via the frontend.
func NewAskUserQuestionTool() (tool.InvokableTool, error) {
	return utils.InferTool("ask_user_question", "Ask the user a question and wait for their response. Use this when you need clarification, confirmation, or the user must make a choice that you cannot determine on your own. The user will see your question in the chat interface and respond. If choices are provided, the user must select from them; otherwise they can type free text.",
		func(ctx context.Context, input AskUserQuestionInput) (AskUserQuestionOutput, error) {
			// Check if this is a resume (we have a user answer from a previous interrupt).
			wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
			if wasInterrupted {
				isTarget, hasData, data := tool.GetResumeContext[string](ctx)
				if isTarget && hasData {
					return AskUserQuestionOutput{Answer: data}, nil
				}
				// Not our turn — re-interrupt without info (keeps the pending state).
				return AskUserQuestionOutput{Answer: ""}, tool.Interrupt(ctx, nil)
			}

			// First invocation — interrupt with structured info for the frontend to render.
			answerType := input.AnswerType
			if answerType == "" {
				answerType = "text"
			}

			return AskUserQuestionOutput{Answer: ""}, tool.Interrupt(ctx, map[string]any{
				"type":        "ask_user_question",
				"question":    input.Question,
				"choices":     input.Choices,
				"answer_type": answerType,
			})
		})
}

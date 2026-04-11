package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"

	openai "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// titleAgentTimeout is the maximum duration for the title generation agent.
const titleAgentTimeout = 15 * time.Second

// titleUpdaterTool creates a tool that updates a conversation title.
// The tool is created per-invocation with conversationID captured, so it can only update its own conversation.
func titleUpdaterTool(userID, conversationID uuid.UUID, db *gorm.DB, nextSeq NextSeqFunc, pushUpdate PushUpdateFunc) (tool.BaseTool, error) {
	t, err := utils.InferTool("update_title", "Update the title of the current conversation. Call this with a concise, descriptive title.",
		func(ctx context.Context, input updateTitleInput) (updateTitleOutput, error) {
			title := input.Title
			if title == "" {
				return updateTitleOutput{Error: "title cannot be empty"}, fmt.Errorf("title cannot be empty")
			}
			if len(title) > 50 {
				title = title[:50]
			}

			seq, err := nextSeq(ctx, userID)
			if err != nil {
				return updateTitleOutput{Error: "seq assignment failed"}, fmt.Errorf("seq assignment failed: %w", err)
			}

			err = db.WithContext(ctx).Model(&model.Conversation{}).
				Where("id = ? AND user_id = ?", conversationID, userID).
				Update("title", title).Error
			if err != nil {
				return updateTitleOutput{Error: "failed to update title"}, fmt.Errorf("failed to update title: %w", err)
			}

			update := model.UserUpdate{
				UserID: userID,
				Seq:    seq,
				Type:   "conversation.updated",
				Payload: model.JSONMap{
					"id":              conversationID.String(),
					"title":           title,
					"title_generated": true,
					"seq":             seq,
				},
			}
			if dbErr := db.WithContext(ctx).Create(&update).Error; dbErr != nil {
				// Log but don't fail — title is already updated in DB
			}
			pushUpdate(userID, update)

			return updateTitleOutput{Success: true, Title: title}, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to create title updater tool: %w", err)
	}
	return t, nil
}

type updateTitleInput struct {
	Title string `json:"title" jsonschema_description:"A concise, descriptive title for the conversation, max 50 characters"`
}

type updateTitleOutput struct {
	Success bool   `json:"success"`
	Title   string `json:"title"`
	Error   string `json:"error,omitempty"`
}

// runTitleAgent launches a lightweight agent that analyzes the user's first message
// and generates a suitable conversation title.
func (s *ChatService) runTitleAgent(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	firstMessage string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("runTitleAgent panic recovered", zap.Any("recover", r))
		}
	}()

	// 1. Resolve haiku model config
	mc := s.modelProvider.GetModel("haiku")
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
	})
	if err != nil {
		s.log.Error("runTitleAgent: failed to create chat model", zap.Error(err))
		return
	}

	// 2. Create the title updater tool — scoped to this conversationID
	updaterTool, err := titleUpdaterTool(userID, conversationID, s.db, nextSeq, pushUpdate)
	if err != nil {
		s.log.Error("runTitleAgent: failed to create title tool", zap.Error(err))
		return
	}

	// 3. Create ChatModelAgent
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "title_generator",
		Description: "Analyzes the user's first message and generates a concise conversation title",
		Instruction: "Analyze the user's message below and generate a concise, descriptive title for this conversation. The title should be under 20 characters and capture the main topic or intent.\n\nCall the update_title tool with your generated title.\n\n<message>\n" + firstMessage + "\n</message>",
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{updaterTool},
			},
		},
		MaxIterations: 5,
	})
	if err != nil {
		s.log.Error("runTitleAgent: failed to create agent", zap.Error(err))
		return
	}

	// 4. Create runner
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	// 5. Run agent with timeout
	runCtx, cancel := context.WithTimeout(ctx, titleAgentTimeout)

	iter := runner.Query(runCtx, firstMessage)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event == nil {
				continue
			}
			if event.Err != nil {
				s.log.Warn("runTitleAgent: agent error", zap.Error(event.Err))
				return
			}
		}
	}()

	select {
	case <-done:
		s.log.Info("runTitleAgent: completed", zap.String("conv", conversationID.String()))
	case <-runCtx.Done():
		s.log.Warn("runTitleAgent: timed out", zap.Duration("timeout", titleAgentTimeout))
	}
	cancel()
}

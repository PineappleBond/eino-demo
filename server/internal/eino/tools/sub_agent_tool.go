package tools

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// SubAgentInput is the input schema for the sub_agent tool.
type SubAgentInput struct {
	Prompt string `json:"prompt" jsonschema_description:"Task description for the sub-agent"`
}

// SubAgentOutput is the output schema for the sub_agent tool.
type SubAgentOutput struct {
	Success    bool   `json:"success"`
	Desc       string `json:"desc"`
	TmpLogFile string `json:"tmp_log_file"`
}

// SpawnSubAgentFunc spawns a sub-agent in a sub-conversation asynchronously.
// The caller (chat service) provides the actual agent execution logic.
// parentConvID and childConvID are the parent and child conversation IDs.
// prompt is the task description. logPath is the JSONL file path.
type SpawnSubAgentFunc func(ctx context.Context, parentConvID, childConvID uuid.UUID, prompt, logPath string)

// SubAgentTool creates and runs a sub-agent in an isolated sub-conversation.
type SubAgentTool struct {
	db             *gorm.DB
	conversationID uuid.UUID
	userID         uuid.UUID
	spawnFn        SpawnSubAgentFunc
}

// NewSubAgentTool creates a sub-agent tool.
func NewSubAgentTool(
	db *gorm.DB,
	conversationID uuid.UUID,
	userID uuid.UUID,
	spawnFn SpawnSubAgentFunc,
) (tool.InvokableTool, error) {
	t := &SubAgentTool{
		db:             db,
		conversationID: conversationID,
		userID:         userID,
		spawnFn:        spawnFn,
	}

	return utils.InferTool(
		"sub_agent",
		"Spawn a sub-agent to execute a task asynchronously. The sub-agent runs in an isolated sub-conversation and writes results to a log file. Results are automatically posted to this conversation on completion.",
		func(ctx context.Context, input SubAgentInput) (SubAgentOutput, error) {
			return t.Run(ctx, input)
		},
	)
}

// Run creates a sub-conversation, starts a sub-agent, and returns the log file path.
func (t *SubAgentTool) Run(ctx context.Context, input SubAgentInput) (SubAgentOutput, error) {
	if t.db == nil {
		return SubAgentOutput{Success: false, Desc: "database not available"}, nil
	}

	if input.Prompt == "" {
		return SubAgentOutput{Success: false, Desc: "prompt is required"}, nil
	}

	// 1. Verify parent conversation exists
	var parentConv model.Conversation
	if err := t.db.WithContext(ctx).Where("id = ? AND user_id = ?", t.conversationID, t.userID).First(&parentConv).Error; err != nil {
		return SubAgentOutput{Success: false, Desc: "parent conversation not found"}, nil
	}

	// 2. Create sub-conversation
	childConv := model.Conversation{
		ProjectID:            parentConv.ProjectID,
		UserID:               t.userID,
		Title:                "Sub-agent: " + truncateStr(input.Prompt, 50),
		Status:               "active",
		Mode:                 parentConv.Mode,
		ParentConversationID: &parentConv.ID,
	}
	if err := t.db.WithContext(ctx).Create(&childConv).Error; err != nil {
		return SubAgentOutput{Success: false, Desc: "failed to create sub-conversation: " + err.Error()}, nil
	}

	// Add user as member
	member := model.ConversationMember{
		ConversationID: childConv.ID,
		MemberType:     "user",
		MemberID:       t.userID.String(),
		MemberName:     "User",
		IsOwner:        true,
	}
	if err := t.db.WithContext(ctx).Create(&member).Error; err != nil {
		return SubAgentOutput{Success: false, Desc: "failed to add member: " + err.Error()}, nil
	}

	// 3. Create JSONL log file path
	logPath := filepath.Join("/tmp", fmt.Sprintf("sub_conv_%s.jsonl", childConv.ID.String()))

	// 4. Spawn sub-agent asynchronously
	if t.spawnFn != nil {
		go t.spawnFn(ctx, parentConv.ID, childConv.ID, input.Prompt, logPath)
	}

	return SubAgentOutput{
		Success:    true,
		Desc:       "子对话已启动，执行完成后会自动通知您，您也可以去临时文件查询进度",
		TmpLogFile: logPath,
	}, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

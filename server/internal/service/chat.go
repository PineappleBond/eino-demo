package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	openai "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/tools"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/templates"
	svcutils "github.com/PineappleBond/eino-demo-dev/server/internal/utils"
)

// agentRunTimeout is the maximum duration for a single agent execution event loop.
const agentRunTimeout = 10 * time.Minute

// ChatService handles sending messages and streaming AI responses.
type ChatService struct {
	db            *gorm.DB
	log           *zap.Logger
	modelProvider *eino.ModelProvider
	toolRegistry  *tools.ToolRegistry
	runSessionMgr *runner.RunSessionManager
}

// NewChatService creates a ChatService.
func NewChatService(
	db *gorm.DB,
	log *zap.Logger,
	modelProvider *eino.ModelProvider,
	toolRegistry *tools.ToolRegistry,
	runSessionMgr *runner.RunSessionManager,
) *ChatService {
	return &ChatService{
		db:            db,
		log:           log,
		modelProvider: modelProvider,
		toolRegistry:  toolRegistry,
		runSessionMgr: runSessionMgr,
	}
}

// SendMessageRequest holds the fields for sending a message.
type SendMessageRequest struct {
	Content string `json:"content"`
}

// SendMessageResponse is the synchronous return from sending a message.
type SendMessageResponse struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
	Seq            int64     `json:"seq"`
}

// NextSeqFunc is a function that assigns the next sequence number for a user.
type NextSeqFunc func(ctx context.Context, userID uuid.UUID) (int64, error)

// PushUpdateFunc pushes an update to a user's WebSocket connections.
type PushUpdateFunc func(userID uuid.UUID, update model.UserUpdate)

// CompleteSendMessage handles the full send flow: service logic + seq + update + push + AI agent run.
// Wraps user message creation + user_update in a single transaction for atomicity.
func (s *ChatService) CompleteSendMessage(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	req SendMessageRequest,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*SendMessageResponse, error) {
	// 1. Verify conversation ownership before allocating seq
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	// Reject if conversation is compacting or compacted
	if conv.Status == "compacting" || conv.Status == "compacted" {
		return nil, fmt.Errorf("conversation is %s, cannot send messages", conv.Status)
	}

	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// 2. Check if this is the first message
	var messageCount int64
	if err := s.db.Model(&model.Message{}).Where("conversation_id = ?", conversationID).Count(&messageCount).Error; err != nil {
		s.log.Error("CompleteSendMessage: failed to count messages", zap.Error(err))
	}
	isFirstMessage := messageCount == 0

	// 3. Allocate seq after ownership confirmed
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("seq assignment failed: %w", err)
	}

	var messageID uuid.UUID

	// 3. Create user message + user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		msg := model.Message{
			ConversationID: conversationID,
			SenderRole:     "user",
			SenderID:       userID.String(),
			Content:        req.Content,
			Metadata:       model.JSONMap{},
			Seq:            seq,
		}
		if err := tx.Create(&msg).Error; err != nil {
			return err
		}
		messageID = msg.ID

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "message.new",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"message_id":      messageID.String(),
				"role":            "user",
				"content":         req.Content,
				"seq":             seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// 4. Push to WebSocket connections
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "message.new",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"role":            "user",
			"content":         req.Content,
			"seq":             seq,
		},
	})

	// --- Run the AI agent asynchronously ---
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("runAgent panic recovered", zap.Any("recover", r))
			}
		}()
		s.runAgent(context.WithoutCancel(ctx), userID, conversationID, req.Content, nextSeq, pushUpdate)
	}()

	// --- Generate title for first message asynchronously ---
	if isFirstMessage {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					s.log.Error("runTitleAgent panic recovered", zap.Any("recover", r))
				}
			}()
			s.runTitleAgent(context.WithoutCancel(ctx), userID, conversationID, req.Content, nextSeq, pushUpdate)
		}()
	}

	return &SendMessageResponse{
		ConversationID: conversationID,
		MessageID:      messageID,
		Seq:            seq,
	}, nil
}

// runAgent starts the AI agent in a background goroutine and streams results via WS.
func (s *ChatService) runAgent(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	userContent string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) {
	// 1. Resolve template config: conversation → project → template
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		s.log.Error("runAgent: conversation not found", zap.Error(err))
		return
	}

	var project model.Project
	if err := s.db.Where("id = ?", conv.ProjectID).First(&project).Error; err != nil {
		s.log.Error("runAgent: project not found", zap.Error(err))
		return
	}

	tpl, ok := templates.Get(project.TemplateID)
	if !ok {
		s.log.Warn("runAgent: template not found, using fallback", zap.String("template_id", project.TemplateID))
		tpl, ok = templates.Get("01")
		if !ok {
			s.log.Error("runAgent: no fallback template available")
			return
		}
	}

	// Extract system prompt from first agent in template
	systemPrompt := ""
	if len(tpl.Agents) > 0 {
		systemPrompt = tpl.Agents[0].SystemPrompt
	}

	// Get model tier from project config (default: sonnet)
	modelTier := "sonnet"
	if tier, ok := project.Config["model_tier"].(string); ok && tier != "" {
		modelTier = tier
	}

	// 2. Create cancellable context
	runCtx, cancel := context.WithCancel(ctx)

	// 3. Build callback handler (creates messages on first chunk)
	callbackCfg := runner.RunCallbackConfig{
		UserID:         userID,
		ConversationID: conversationID,
		DB:             s.db,
		Log:            s.log,
		NextSeq:        nextSeq,
		PushUpdate:     pushUpdate,
		ParentCtx:      ctx, // parent context, not cancelled — used for lifecycle methods
	}
	callbacks := runner.NewRootRunnerCallbacks(callbackCfg)

	// 4. Register session (stop func marks in-progress messages as stopped in DB)
	s.runSessionMgr.Start(conversationID, cancel, callbacks.Stop)

	// 5. Build run config
	runCfg := runner.RootRunnerConfig{
		ModelProvider: s.modelProvider,
		ModelTier:     modelTier,
		SystemPrompt:  systemPrompt,
		Tools:         s.toolRegistry.GetBaseTools(),
		MaxIteration:  100,
	}

	rootRunner, err := runner.NewRootRunner(runCtx, runCfg, callbacks)
	if err != nil {
		s.log.Error("runAgent: failed to create runner", zap.Error(err))
		cancel()
		s.runSessionMgr.Cleanup(conversationID)
		return
	}

	// 5. Create handler
	handler := runner.NewRootRunnerHandler(callbacks, nil)

	// 7. Check for existing checkpoint to resume
	checkpointID, hasCheckpoint := s.runSessionMgr.GetCheckpointID(conversationID)

	var iter *adk.AsyncIterator[*adk.AgentEvent]
	// Load conversation history as context: user, assistant, and system messages
	var historyMessages []model.Message
	if err := s.db.
		Select("sender_role", "content").
		Where("conversation_id = ? AND sender_role IN ?", conversationID, []string{"user", "assistant", "system"}).
		Order("seq ASC").
		Find(&historyMessages).Error; err != nil {
		s.log.Error("runAgent: failed to load conversation history", zap.Error(err))
	}

	// Build message list from history + current user message
	var messages []*schema.Message
	for _, msg := range historyMessages {
		switch msg.SenderRole {
		case "user":
			messages = append(messages, schema.UserMessage(msg.Content))
		case "assistant":
			messages = append(messages, schema.AssistantMessage(msg.Content, nil))
		case "system":
			messages = append(messages, schema.SystemMessage(msg.Content))
		}
	}
	//messages = append(messages, schema.UserMessage(userContent))

	// Check prompt token count before running agent to prevent context overflow.
	// Limit: 80K tokens — beyond this, compaction is recommended.
	const maxPromptTokens = 80000
	if len(messages) > 0 {
		counter, cerr := svcutils.NewTokenCounter(modelTier)
		if cerr != nil {
			s.log.Warn("runAgent: failed to create token counter, skipping check", zap.Error(cerr))
		} else {
			roles := make([]string, 0, len(messages))
			contents := make([]string, 0, len(messages))
			for _, m := range messages {
				roles = append(roles, string(m.Role))
				contents = append(contents, m.Content)
			}
			promptTokens := counter.CountMessages(roles, contents)
			s.log.Info("runAgent: prompt token estimate",
				zap.Int("tokens", promptTokens),
				zap.Int("limit", maxPromptTokens),
				zap.Int("messages", len(messages)),
			)

			// Update conversation token_prompt field and push notification
			seq, seqErr := nextSeq(ctx, userID)
			if seqErr != nil {
				s.log.Error("runAgent: seq assignment failed for token update", zap.Error(seqErr))
			} else {
				if err := s.db.WithContext(ctx).Model(&model.Conversation{}).
					Where("id = ?", conversationID).
					Updates(map[string]interface{}{
						"token_prompt": promptTokens,
					}).Error; err != nil {
					s.log.Error("runAgent: failed to update conversation token_prompt", zap.Error(err))
				}

				update := model.UserUpdate{
					UserID: userID,
					Seq:    seq,
					Type:   "conversation.updated",
					Payload: model.JSONMap{
						"conversation_id": conversationID.String(),
						"token_prompt":    promptTokens,
						"seq":             seq,
					},
				}
				if dbErr := s.db.WithContext(ctx).Create(&update).Error; dbErr != nil {
					s.log.Error("failed to persist token update", zap.Error(dbErr))
				}
				pushUpdate(userID, update)
			}

			if promptTokens > maxPromptTokens {
				s.log.Warn("runAgent: prompt exceeds token limit, suggesting compaction",
					zap.Int("tokens", promptTokens),
					zap.Int("limit", maxPromptTokens),
				)
				// Push error update instead of running agent
				errSeq, err := nextSeq(ctx, userID)
				if err != nil {
					s.log.Error("runAgent: seq assignment failed", zap.Error(err))
				} else {
					errUpdate := model.UserUpdate{
						UserID: userID,
						Seq:    errSeq,
						Type:   "message.error",
						Payload: model.JSONMap{
							"conversation_id": conversationID.String(),
							"error":           fmt.Sprintf("对话历史过长（约 %d tokens），超出限制 %d tokens。请先压缩后再发送。", promptTokens, maxPromptTokens),
						},
					}
					if dbErr := s.db.WithContext(ctx).Create(&errUpdate).Error; dbErr != nil {
						s.log.Error("failed to persist token-limit error update", zap.Error(dbErr))
					}
					pushUpdate(userID, errUpdate)
				}
				cancel()
				s.runSessionMgr.Cleanup(conversationID)
				return
			}
		}
	}

	if hasCheckpoint {
		iter, err = rootRunner.Resume(runCtx, checkpointID, handler)
		if err != nil {
			s.log.Error("runAgent: resume failed, falling back to fresh run", zap.Error(err), zap.String("checkpoint", checkpointID))
			iter = rootRunner.Run(runCtx, messages, "", handler)
		}
	} else {
		iter = rootRunner.Run(runCtx, messages, "", handler)
	}

	// 8. Consume events in goroutine with timeout protection and panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("runAgent event loop panic recovered", zap.Any("recover", r))
				s.runSessionMgr.Cleanup(conversationID)
				s.runSessionMgr.Done(conversationID)
				cancel()
			}
		}()
		defer func() {
			s.runSessionMgr.Cleanup(conversationID)
			s.runSessionMgr.Done(conversationID)
		}()
		defer cancel()

		timer := time.NewTimer(agentRunTimeout)
		defer timer.Stop()

		done := make(chan struct{})
		go func() {
			for {
				event, ok := iter.Next()
				if !ok {
					callbacks.OnEnd()
					break
				}
				if event == nil {
					continue
				}
				if event.Err != nil {
					callbacks.OnError(event.Err)
					break
				}
				if event.Action != nil && event.Action.Interrupted != nil {
					// Store checkpoint ID for resume
					if len(event.Action.Interrupted.InterruptContexts) > 0 {
						cpID := event.Action.Interrupted.InterruptContexts[0].ID
						s.runSessionMgr.SetCheckpointID(conversationID, cpID)
					}
					callbacks.OnInterrupted(event.Action.Interrupted)
					break
				}
			}
			close(done)
		}()

		select {
		case <-done:
			// Normal completion
		case <-timer.C:
			s.log.Warn("runAgent: event loop timed out", zap.Duration("timeout", agentRunTimeout))
		}
	}()
}

// StopMessage marks the latest AI message as stopped and cancels the running agent.
func (s *ChatService) StopMessage(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// Verify conversation exists and belongs to user
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	// 1. Cancel the running agent
	s.runSessionMgr.Stop(conversationID)

	// 2. Assign seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// 3. Create user_update for the stop event
	update := model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "message.stop",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
		},
	}
	if err := s.db.Create(&update).Error; err != nil {
		return err
	}

	pushUpdate(userID, update)

	return nil
}

// GetConversationMessagesRequest holds the query params for listing messages.
type GetConversationMessagesRequest struct {
	AfterSeq int64 `form:"after_seq"`
}

// GetConversationMessages returns messages for a conversation, optionally after a seq.
func (s *ChatService) GetConversationMessages(userID, conversationID uuid.UUID, req GetConversationMessagesRequest) ([]model.Message, error) {
	// Verify conversation exists and belongs to user
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}

	query := s.db.Where("conversation_id = ?", conversationID).Order("seq ASC")
	if req.AfterSeq > 0 {
		query = query.Where("seq > ?", req.AfterSeq)
	}

	var messages []model.Message
	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

// CompactConversationResponse holds the response for starting compaction.
type CompactConversationResponse struct {
	ConversationID string `json:"conversation_id"`
}

// StartCompaction sets the conversation status to "compacting",
// emits a conversation.compacting update, and launches the compact agent asynchronously.
func (s *ChatService) StartCompaction(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Verify ownership
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found")
	}

	// 2. Check current status
	if conv.Status == "compacting" || conv.Status == "compacted" {
		return fmt.Errorf("conversation is already %s", conv.Status)
	}

	// 3. Allocate seq
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	// 4. Set status to compacting + create user_update in a single transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Conversation{}).
			Where("id = ? AND user_id = ?", conversationID, userID).
			Update("status", "compacting").Error; err != nil {
			return fmt.Errorf("failed to set compacting status: %w", err)
		}

		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.compacting",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"seq":             seq,
			},
		}
		if err := tx.Create(&update).Error; err != nil {
			return fmt.Errorf("failed to create user_update: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 5. Push update
	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.compacting",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
			"seq":             seq,
		},
	})

	// 6. Load messages for the agent
	var historyMessages []model.Message
	if err := s.db.
		Select("sender_role", "content").
		Where("conversation_id = ? AND sender_role IN ?", conversationID, []string{"user", "assistant", "system"}).
		Order("seq ASC").
		Find(&historyMessages).Error; err != nil {
		s.log.Error("runAgent: failed to load conversation history", zap.Error(err))
	}
	// 7. Launch the compact agent asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("runCompactAgent panic recovered", zap.Any("recover", r))
			}
		}()
		s.runCompactAgent(context.WithoutCancel(ctx), userID, conversationID, conv.ProjectID, historyMessages, nextSeq, pushUpdate)
	}()

	return nil
}

// compactAgentTimeout is the maximum duration for the compact agent.
const compactAgentTimeout = 2 * time.Minute

// runCompactAgent launches an agent that analyzes a conversation, generates a summary,
// and calls the complete_compaction tool to archive the old conversation and create a new one.
func (s *ChatService) runCompactAgent(
	ctx context.Context,
	userID, conversationID, projectID uuid.UUID,
	messages []model.Message,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("runCompactAgent panic recovered", zap.Any("recover", r))
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
		s.log.Error("runCompactAgent: failed to create chat model", zap.Error(err))
		return
	}

	// 2. Build message content for the agent
	var msgContent strings.Builder
	for _, m := range messages {
		if m.SenderRole == "user" {
			msgContent.WriteString(fmt.Sprintf("<user>%s</user>\n", m.Content))
		} else if m.SenderRole == "assistant" {
			msgContent.WriteString(fmt.Sprintf("<assistant>%s</assistant>\n", m.Content))
		} else if m.SenderRole == "system" {
			msgContent.WriteString(fmt.Sprintf("<system>%s</system>\n", m.Content))
		}
	}

	// 3. Create the complete_compaction tool — scoped to this conversation
	compactionTool, err := completeCompactionTool(userID, conversationID, projectID, s.db, nextSeq, pushUpdate)
	if err != nil {
		s.log.Error("runCompactAgent: failed to create compaction tool", zap.Error(err))
		return
	}

	// 4. Create ChatModelAgent
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "compact_agent",
		Description: "Analyzes a conversation and compresses it into a new conversation with preserved context",
		Instruction: "你是一个对话压缩助手。分析下面的对话历史，提取关键信息，然后调用 complete_compaction 工具来完成压缩。\n\n" +
			"<instructions>\n" +
			"1. 阅读对话历史，理解用户的意图和讨论的主题\n" +
			"2. 生成一个简洁的对话摘要（保留关键信息、决定、待办事项）\n" +
			"3. 为新对话生成一个合适的标题\n" +
			"4. 调用 complete_compaction 工具，传入 summary 和 title\n" +
			"</instructions>\n\n" +
			"<conversation>\n" + msgContent.String() + "\n</conversation>",
		Model: chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{compactionTool},
			},
		},
		MaxIterations: 10,
	})
	if err != nil {
		s.log.Error("runCompactAgent: failed to create agent", zap.Error(err))
		return
	}

	// 5. Create runner
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	// 6. Run agent with timeout
	runCtx, cancel := context.WithTimeout(ctx, compactAgentTimeout)
	defer cancel()

	iter := runner.Query(runCtx, "请压缩以上对话历史")
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
				s.log.Warn("runCompactAgent: agent error", zap.Error(event.Err))
				return
			}
		}
	}()

	select {
	case <-done:
		s.log.Info("runCompactAgent: completed", zap.String("conv", conversationID.String()))
	case <-runCtx.Done():
		s.log.Warn("runCompactAgent: timed out", zap.Duration("timeout", compactAgentTimeout))
	}
}

// complete_compaction tool input/output
type completeCompactionInput struct {
	Summary string `json:"summary" jsonschema_description:"A concise summary of the conversation, preserving key information, decisions, and action items"`
	Title   string `json:"title" jsonschema_description:"A concise, descriptive title for the new conversation"`
}

type completeCompactionOutput struct {
	Success      bool   `json:"success"`
	NewConvID    string `json:"new_conversation_id,omitempty"`
	OldConvID    string `json:"old_conversation_id,omitempty"`
	Title        string `json:"title,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// completeCompactionTool creates a tool that archives the old conversation and creates a new one.
// The tool captures conversationID, userID, projectID via closure so it can only operate on the correct conversation.
func completeCompactionTool(userID, conversationID, projectID uuid.UUID, db *gorm.DB, nextSeq NextSeqFunc, pushUpdate PushUpdateFunc) (tool.BaseTool, error) {
	t, err := utils.InferTool("complete_compaction", "Archive the old conversation and create a new conversation with preserved context. Call this with a summary and title.",
		func(ctx context.Context, input completeCompactionInput) (completeCompactionOutput, error) {
			if input.Summary == "" {
				return completeCompactionOutput{ErrorMessage: "summary is required"}, fmt.Errorf("summary is required")
			}
			title := input.Title
			if title == "" {
				title = "Continued from previous conversation"
			}
			if len(title) > 100 {
				title = title[:100]
			}

			compactedSeq, err := nextSeq(ctx, userID)
			if err != nil {
				return completeCompactionOutput{ErrorMessage: "seq assignment failed"}, fmt.Errorf("seq assignment failed: %w", err)
			}

			var newConvID uuid.UUID

			err = db.Transaction(func(tx *gorm.DB) error {
				// 1. Archive old conversation
				if err := tx.Model(&model.Conversation{}).
					Where("id = ? AND user_id = ?", conversationID, userID).
					Updates(map[string]interface{}{
						"status":  "compacted",
						"summary": input.Summary,
					}).Error; err != nil {
					return fmt.Errorf("failed to archive old conversation: %w", err)
				}

				// 2. Create new conversation
				newConv := model.Conversation{
					ProjectID:            projectID,
					UserID:               userID,
					Title:                title,
					Status:               "active",
					ParentConversationID: &conversationID,
				}
				if err := tx.Create(&newConv).Error; err != nil {
					return fmt.Errorf("failed to create new conversation: %w", err)
				}
				newConvID = newConv.ID

				// 3. Add user as member
				member := model.ConversationMember{
					ConversationID: newConv.ID,
					MemberType:     "user",
					MemberID:       userID.String(),
					MemberName:     "User",
					IsOwner:        true,
				}
				if err := tx.Create(&member).Error; err != nil {
					return fmt.Errorf("failed to add conversation member: %w", err)
				}

				// 4. Insert system message with compressed context
				systemMsg := model.Message{
					ConversationID: newConv.ID,
					SenderRole:     "system",
					SenderID:       "system",
					Content:        input.Summary,
					Metadata:       model.JSONMap{"compressed": true},
					Seq:            0, // First message, seq will be assigned below
				}
				if err := tx.Create(&systemMsg).Error; err != nil {
					return fmt.Errorf("failed to insert system message: %w", err)
				}

				// 5. Create user_update for the new conversation
				update := model.UserUpdate{
					UserID: userID,
					Seq:    compactedSeq,
					Type:   "conversation.compacted",
					Payload: model.JSONMap{
						"conversation_id": conversationID.String(),
						"old_conv_id":     conversationID.String(),
						"new_conv_id":     newConv.ID.String(),
						"project_id":      projectID.String(),
						"title":           title,
						"seq":             compactedSeq,
					},
				}
				if err := tx.Create(&update).Error; err != nil {
					return fmt.Errorf("failed to create user_update: %w", err)
				}

				return nil
			})
			if err != nil {
				return completeCompactionOutput{ErrorMessage: "failed to complete compaction"}, err
			}

			// 6. Push update to WebSocket
			pushUpdate(userID, model.UserUpdate{
				UserID: userID,
				Seq:    compactedSeq,
				Type:   "conversation.compacted",
				Payload: model.JSONMap{
					"conversation_id": conversationID.String(),
					"old_conv_id":     conversationID.String(),
					"new_conv_id":     newConvID.String(),
					"project_id":      projectID.String(),
					"title":           title,
					"seq":             compactedSeq,
				},
			})

			return completeCompactionOutput{
				Success:   true,
				NewConvID: newConvID.String(),
				OldConvID: conversationID.String(),
				Title:     title,
			}, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to create complete_compaction tool: %w", err)
	}
	return t, nil
}

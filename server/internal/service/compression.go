package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/prompts"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// CompressConversationResult holds the result of a conversation compression.
type CompressConversationResult struct {
	Summary     string `json:"summary"`
	SummarySeq  int64  `json:"summary_seq"`
	MinSeq      int64  `json:"min_seq"`
	OldMsgCount int    `json:"old_msg_count"`
}

// CompressionService handles in-place conversation compression.
type CompressionService struct {
	db            *gorm.DB
	log           *zap.Logger
	modelProvider *eino.ModelProvider
}

// NewCompressionService creates a compression service.
func NewCompressionService(
	db *gorm.DB,
	log *zap.Logger,
	modelProvider *eino.ModelProvider,
) *CompressionService {
	return &CompressionService{
		db:            db,
		log:           log,
		modelProvider: modelProvider,
	}
}

// compressToolInput holds the input for the compress_conversation tool.
type compressToolInput struct {
	Summary string `json:"summary" jsonschema_description:"A concise summary of the conversation, preserving key information, decisions, and action items"`
}

type compressToolOutput struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// compressionPromptData is the template data for compression prompts.
type compressionPromptData struct {
	Conversation string
}

// CompressConversation compresses a conversation in-place:
// 1. Loads all messages with seq >= conv.MinSeq
// 2. Calls LLM to generate a summary
// 3. Inserts a system message with the summary
// 4. Updates conversation.min_seq and conversation.summary
func (s *CompressionService) CompressConversation(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (*CompressConversationResult, error) {
	// 1. Verify ownership and get current MinSeq
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	// 2. Load messages to compress (seq >= MinSeq) — includes tool messages
	var messages []model.Message
	if err := s.db.
		Select("sender_role", "content", "seq").
		Where("conversation_id = ? AND seq >= ? AND sender_role IN ?",
			conversationID, conv.MinSeq, []string{"user", "assistant", "tool", "system"}).
		Order("seq ASC").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to load conversation history: %w", err)
	}

	if len(messages) == 0 {
		return &CompressConversationResult{}, nil
	}

	// 3. Build prompt content from raw messages for the summary LLM
	var msgContent strings.Builder
	for _, m := range messages {
		switch m.SenderRole {
		case "user":
			msgContent.WriteString(fmt.Sprintf("<user>%s</user>\n", m.Content))
		case "assistant":
			msgContent.WriteString(fmt.Sprintf("<assistant>%s</assistant>\n", m.Content))
		case "tool":
			content := m.Content
			if content == "" && m.ToolCalling != nil {
				if out, ok := m.ToolCalling["output"].(string); ok {
					content = out
				}
			}
			msgContent.WriteString(fmt.Sprintf("<tool>%s</tool>\n", content))
		case "system":
			msgContent.WriteString(fmt.Sprintf("<system>%s</system>\n", m.Content))
		}
	}

	// 4. Create LLM model (haiku for cost efficiency)
	mc := s.modelProvider.GetModel("haiku")
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model: %w", err)
	}

	// 5. Create compress tool scoped to this conversation
	compTool, err := s.createCompressTool(userID, conversationID, conv.Summary, nextSeq, pushUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to create compress tool: %w", err)
	}

	// 6. Create agent — render prompt template with conversation content.
	instruction, err := prompts.Render("compression", compressionPromptData{Conversation: msgContent.String()})
	if err != nil {
		return nil, fmt.Errorf("failed to render compression prompt: %w", err)
	}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "compression",
		Description: "Analyzes a conversation and generates a concise summary",
		Instruction: instruction,
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{compTool},
			},
		},
		MaxIterations: 10,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// 7. Run agent
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	iter := runner.Query(ctx, "请压缩以上对话历史")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return nil, fmt.Errorf("compression agent error: %w", event.Err)
		}
	}

	// The tool itself does the DB write, so we just need to reload the conversation
	// to get the updated min_seq. Return what we know.
	return &CompressConversationResult{
		OldMsgCount: len(messages),
	}, nil
}

// createCompressTool creates a tool that writes the compression result to DB.
func (s *CompressionService) createCompressTool(
	userID, conversationID uuid.UUID,
	existingSummary string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) (tool.BaseTool, error) {
	return utils.InferTool("compress_conversation",
		"Store the conversation summary and update the compression cursor. Call this after analyzing the conversation.",
		func(ctx context.Context, input compressToolInput) (compressToolOutput, error) {
			if input.Summary == "" {
				return compressToolOutput{ErrorMessage: "summary is required"}, fmt.Errorf("summary is required")
			}

			seq, err := nextSeq(ctx, userID)
			if err != nil {
				return compressToolOutput{ErrorMessage: "seq assignment failed"}, fmt.Errorf("seq assignment failed: %w", err)
			}

			var summaryMsgID uuid.UUID
			var summarySeq int64
			var newMinSeq int64

			err = s.db.Transaction(func(tx *gorm.DB) error {
				// 1. Get the max seq of messages that are being compressed
				var maxCompressedSeq int64
				if err := tx.Model(&model.Message{}).
					Where("conversation_id = ?", conversationID).
					Select("COALESCE(MAX(seq), 0)").
					Scan(&maxCompressedSeq).Error; err != nil {
					return fmt.Errorf("failed to get max seq: %w", err)
				}

				// 2. Insert summary message
				summarySeq, err = nextSeq(ctx, userID)
				if err != nil {
					return fmt.Errorf("seq assignment failed: %w", err)
				}
				summaryMsgID = uuid.New()
				newMinSeq = summarySeq

				summaryMsg := model.Message{
					ConversationID: conversationID,
					SenderRole:     "system",
					SenderID:       "system",
					Content:        input.Summary,
					Metadata:       model.JSONMap{"type": "summary"},
					Seq:            summarySeq,
				}
				summaryMsg.ID = summaryMsgID
				if err := tx.Create(&summaryMsg).Error; err != nil {
					return fmt.Errorf("failed to insert summary message: %w", err)
				}

				// 3. Update conversation min_seq and summary
				if err := tx.Model(&model.Conversation{}).
					Where("id = ? AND user_id = ?", conversationID, userID).
					Updates(map[string]interface{}{
						"min_seq": newMinSeq,
						"summary": input.Summary,
					}).Error; err != nil {
					return fmt.Errorf("failed to update conversation: %w", err)
				}

				// 4. Create user_update
				update := model.UserUpdate{
					UserID: userID,
					Seq:    seq,
					Type:   "conversation.compressed",
					Payload: model.JSONMap{
						"conversation_id": conversationID.String(),
						"min_seq":         newMinSeq,
						"summary":         input.Summary,
						"message_id":      summaryMsgID.String(),
						"seq":             seq,
					},
				}
				if err := tx.Create(&update).Error; err != nil {
					return fmt.Errorf("failed to create user_update: %w", err)
				}

				return nil
			})
			if err != nil {
				return compressToolOutput{ErrorMessage: "failed to compress conversation"}, err
			}

			// 5. Push update to WebSocket
			pushUpdate(userID, model.UserUpdate{
				UserID: userID,
				Seq:    seq,
				Type:   "conversation.compressed",
				Payload: model.JSONMap{
					"conversation_id": conversationID.String(),
					"min_seq":         newMinSeq,
					"summary":         input.Summary,
					"message_id":      summaryMsgID.String(),
					"seq":             seq,
				},
			})

			s.log.Info("conversation compressed",
				zap.String("conv", conversationID.String()),
				zap.Int64("min_seq", newMinSeq),
				zap.Int("summary_len", len(input.Summary)),
			)

			return compressToolOutput{Success: true}, nil
		})
}

// CompressAndResume handles token overflow by compressing the conversation.
// It does NOT restart the agent — the caller (chat.go overflow handler) is
// responsible for stopping the old session, cleaning up, and launching a
// fresh runAgent after compression succeeds.
// Called from the event loop when TokenOverflowError is detected.
func (s *CompressionService) CompressAndResume(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	s.log.Info("starting compress and resume",
		zap.String("conv", conversationID.String()),
	)

	// 1. Compress the conversation
	if _, err := s.CompressConversation(ctx, userID, conversationID, nextSeq, pushUpdate); err != nil {
		return fmt.Errorf("compress failed: %w", err)
	}

	return nil
}

// SyncSummarizationToDB writes a summary message to DB when the Summarization
// middleware compresses context during an active agent run.
//
// compressedMsgCount is the number of non-system messages that were compressed
// away (before.Messages had N non-system messages that became just a summary).
// We find the last compressed message by seq and set min_seq = its seq + 1.
//
// The summary text is regenerated from the compressed messages via LLM to
// ensure it accurately represents what was compressed.
func (s *CompressionService) SyncSummarizationToDB(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	compressedMsgCount int,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) error {
	// 1. Get current min_seq
	var conv model.Conversation
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", conversationID, userID).
		First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found: %w", err)
	}

	// 2. Load messages to summarize using the shared function (handles tool_calling + tool messages)
	schemaMessages, err := loadConversationMessages(ctx, s.db, conversationID, conv.MinSeq)
	if err != nil {
		return fmt.Errorf("failed to load conversation history: %w", err)
	}

	if len(schemaMessages) == 0 {
		return nil
	}

	// 3. Generate summary via LLM
	summaryText, err := s.CompressMessagesToSummary(ctx, schemaMessages)
	if err != nil {
		s.log.Error("SyncSummarizationToDB: failed to generate summary, using fallback", zap.Error(err))
		summaryText = fmt.Sprintf("对话历史已压缩。此前有 %d 条消息，已被摘要。", len(schemaMessages))
	}

	// 4. Find the max seq among compressed messages to set new min_seq
	var maxCompressedSeq int64
	if err := s.db.WithContext(ctx).Model(&model.Message{}).
		Where("conversation_id = ? AND seq >= ?", conversationID, conv.MinSeq).
		Select("COALESCE(MAX(seq), 0)").
		Scan(&maxCompressedSeq).Error; err != nil {
		return fmt.Errorf("failed to get max seq: %w", err)
	}

	// 5. Insert summary message
	seq, err := nextSeq(ctx, userID)
	if err != nil {
		return fmt.Errorf("seq assignment failed: %w", err)
	}

	newMinSeq := maxCompressedSeq + 1

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		summaryMsgID := uuid.New()
		summarySeq, err := nextSeq(ctx, userID)
		if err != nil {
			return fmt.Errorf("seq assignment failed for summary: %w", err)
		}
		summaryMsg := model.Message{
			ConversationID: conversationID,
			SenderRole:     "system",
			SenderID:       "system",
			Content:        summaryText,
			Metadata:       model.JSONMap{"type": "summary"},
			Seq:            summarySeq,
		}
		summaryMsg.ID = summaryMsgID
		if err := tx.Create(&summaryMsg).Error; err != nil {
			return fmt.Errorf("failed to insert summary message: %w", err)
		}

		// 6. Update conversation min_seq to the summary's seq so it is
		// included in future context loads (seq >= min_seq).
		if err := tx.Model(&model.Conversation{}).
			Where("id = ? AND user_id = ?", conversationID, userID).
			Updates(map[string]interface{}{
				"min_seq": summarySeq,
				"summary": summaryText,
			}).Error; err != nil {
			return fmt.Errorf("failed to update conversation: %w", err)
		}

		// 7. Create user_update
		update := model.UserUpdate{
			UserID: userID,
			Seq:    seq,
			Type:   "conversation.compressed",
			Payload: model.JSONMap{
				"conversation_id": conversationID.String(),
				"min_seq":         newMinSeq,
				"summary":         summaryText,
				"message_id":      summaryMsgID.String(),
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

	pushUpdate(userID, model.UserUpdate{
		UserID: userID,
		Seq:    seq,
		Type:   "conversation.compressed",
		Payload: model.JSONMap{
			"conversation_id": conversationID.String(),
			"min_seq":         newMinSeq,
			"summary":         summaryText,
			"seq":             seq,
		},
	})

	s.log.Info("conversation compressed (via summarization)",
		zap.String("conv", conversationID.String()),
		zap.Int64("min_seq", newMinSeq),
		zap.Int("compressed_count", compressedMsgCount),
	)

	return nil
}

// CompressMessagesToSummary is a helper that takes raw messages and returns a summary string.
// Used by the Summarization middleware Callback to get the summary text before writing to DB.
func (s *CompressionService) CompressMessagesToSummary(
	ctx context.Context,
	messages []*schema.Message,
) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	var msgContent strings.Builder
	for _, m := range messages {
		switch m.Role {
		case schema.User:
			msgContent.WriteString(fmt.Sprintf("<user>%s</user>\n", m.Content))
		case schema.Assistant:
			msgContent.WriteString(fmt.Sprintf("<assistant>%s</assistant>\n", m.Content))
		case schema.Tool:
			msgContent.WriteString(fmt.Sprintf("<tool>%s</tool>\n", m.Content))
		case schema.System:
			msgContent.WriteString(fmt.Sprintf("<system>%s</system>\n", m.Content))
		}
	}

	mc := s.modelProvider.GetModel("haiku")
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create chat model: %w", err)
	}

	instruction, err := prompts.Render("compression_summary", compressionPromptData{Conversation: msgContent.String()})
	if err != nil {
		return "", fmt.Errorf("failed to render compression_summary prompt: %w", err)
	}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "compression",
		Description:   "Generates a concise summary of a conversation",
		Instruction:   instruction,
		Model:         chatModel,
		MaxIterations: 5,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create agent: %w", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	// Collect the final output from the iterator
	iter := runner.Query(ctx, "请压缩以上对话历史")
	var summaryText string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return "", fmt.Errorf("summarization agent error: %w", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg := event.Output.MessageOutput.Message
			if msg != nil {
				summaryText = msg.Content
			}
		}
	}

	return summaryText, nil
}

package service

import (
	"context"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/cloudwego/eino/adk"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// resumeAgent sets resume params and restarts the agent in a background goroutine.
// The resume data is keyed on interruptID → answerValue.
func (s *ChatService) resumeAgent(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	interruptID, answerValue string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
) {
	// Clear the DB checkpoint — it will be re-set by the next interrupt if needed.
	if err := s.db.WithContext(ctx).
		Model(&model.Conversation{}).
		Where("id = ?", conversationID).
		Update("checkpoint_id", "").Error; err != nil {
		s.log.Error("failed to clear conversation checkpoint", zap.Error(err))
	}

	// Set resume params and checkpoint ID for runAgent.
	// conversationID.String() is the key passed to RootRunner.Run via WithCheckPointID.
	s.runSessionMgr.SetResumeParams(conversationID, &adk.ResumeParams{
		Targets: map[string]any{
			interruptID: answerValue,
		},
	})
	s.runSessionMgr.SetCheckpointID(conversationID, conversationID.String())

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("resumeAgent runAgent panic recovered", zap.Any("recover", r))
			}
		}()
		s.runAgent(context.WithoutCancel(ctx), userID, conversationID, "", nextSeq, pushUpdate)
	}()
}

package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	svcutils "github.com/PineappleBond/eino-demo-dev/server/internal/utils"
)

// TokenOverflowError is returned when the conversation history exceeds the token limit.
// The event loop should catch this error, stop the agent, re-enqueue DrainedMessages,
// compress the conversation, and start a fresh run.
type TokenOverflowError struct {
	TokenCount      int
	MaxTokens       int
	DrainedMessages []string // Messages drained from queue before overflow check; must be re-enqueued before fresh run
}

func (e *TokenOverflowError) Error() string {
	return fmt.Sprintf("token overflow: %d tokens exceed limit of %d", e.TokenCount, e.MaxTokens)
}

// TokenCheckConfig holds the parameters for token overflow detection.
type TokenCheckConfig struct {
	// Counter estimates tokens for messages.
	Counter *svcutils.TokenCounter
	// MaxTokens is the hard limit for the conversation history.
	MaxTokens int
	// OnOverflow is called when token count exceeds MaxTokens.
	// It should stop the agent and initiate compaction.
	// Called from within BeforeModelRewriteState, not goroutine-safe by default.
	OnOverflow func(tokenCount int)
}

// contextInjectionMiddleware injects pending messages from the MessageQueue
// into the agent's message history before each model call.
//
// This allows users to send new messages while the agent is actively running.
// The injected messages are drained from the queue and appended to state.Messages
// so they become part of the conversation history naturally.
//
// Design reference: Claude Code's pendingMessages queue, drained at
// tool-round boundaries (query.ts:1580).
type contextInjectionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	queue          *MessageQueue
	conversationID uuid.UUID
	tokenCheck     *TokenCheckConfig // nil = no token checking
}

// NewContextInjectionMiddleware creates a ChatModelAgentMiddleware that injects
// pending messages from the MessageQueue before each model call.
// tokenCheck can be nil to skip token counting.
func NewContextInjectionMiddleware(queue *MessageQueue, conversationID uuid.UUID, tokenCheck *TokenCheckConfig) adk.ChatModelAgentMiddleware {
	return &contextInjectionMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		queue:          queue,
		conversationID: conversationID,
		tokenCheck:     tokenCheck,
	}
}

func (m *contextInjectionMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	// 1. Drain pending messages and track them for potential re-enqueue on overflow.
	drained := m.queue.Drain(m.conversationID)
	if len(drained) > 0 {
		for _, content := range drained {
			state.Messages = append(state.Messages, schema.UserMessage(content))
		}
	}

	// 2. Check token count after injection.
	if m.tokenCheck != nil && m.tokenCheck.Counter != nil && len(state.Messages) > 0 {
		roles := make([]string, 0, len(state.Messages))
		contents := make([]string, 0, len(state.Messages))
		for _, msg := range state.Messages {
			roles = append(roles, string(msg.Role))
			contents = append(contents, msg.Content)
		}
		totalTokens := m.tokenCheck.Counter.CountMessages(roles, contents)

		if totalTokens > m.tokenCheck.MaxTokens {
			if m.tokenCheck.OnOverflow != nil {
				m.tokenCheck.OnOverflow(totalTokens)
			}
			return ctx, state, &TokenOverflowError{
				TokenCount:      totalTokens,
				MaxTokens:       m.tokenCheck.MaxTokens,
				DrainedMessages: drained, // Preserve for re-enqueue before fresh run
			}
		}
	}

	return ctx, state, nil
}

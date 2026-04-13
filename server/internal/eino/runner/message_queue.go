package runner

import (
	"sync"

	"github.com/google/uuid"
)

// MessageQueue manages per-conversation pending messages.
// Thread-safe. Messages are enqueued by SendMessage and drained
// by the agent's contextInjectionMiddleware at each model call boundary.
//
// Design reference: Claude Code's pendingMessages queue, drained at
// tool-round boundaries (query.ts:1580).
type MessageQueue struct {
	mu     sync.Mutex
	queues map[string][]string // keyed by conversationID.String()
}

// NewMessageQueue creates an empty message queue.
func NewMessageQueue() *MessageQueue {
	return &MessageQueue{
		queues: make(map[string][]string),
	}
}

// Enqueue adds a message to the conversation's pending queue.
func (q *MessageQueue) Enqueue(conversationID uuid.UUID, content string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := conversationID.String()
	q.queues[key] = append(q.queues[key], content)
}

// Drain removes and returns all pending messages for the conversation.
// Returns nil if no messages are pending.
func (q *MessageQueue) Drain(conversationID uuid.UUID) []string {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := conversationID.String()
	msgs, ok := q.queues[key]
	if !ok || len(msgs) == 0 {
		return nil
	}
	delete(q.queues, key)
	return msgs
}

// HasPending returns true if the conversation has pending messages.
func (q *MessageQueue) HasPending(conversationID uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := conversationID.String()
	msgs, ok := q.queues[key]
	return ok && len(msgs) > 0
}

// Cleanup removes any pending messages for the conversation.
// Call this when the agent run ends to prevent stale map entries
// from accumulating (e.g. messages enqueued after the agent started
// but before it called Drain).
func (q *MessageQueue) Cleanup(conversationID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.queues, conversationID.String())
}

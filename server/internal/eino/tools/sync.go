package tools

import (
	"context"

	"github.com/google/uuid"
)

// SyncPushFunc pushes a panel-sync WebSocket update to a user.
// Called by tools (todo_write, cron_task) after mutating state so the
// frontend can refresh its UI without polling.
type SyncPushFunc func(ctx context.Context, userID, conversationID uuid.UUID, updateType string)

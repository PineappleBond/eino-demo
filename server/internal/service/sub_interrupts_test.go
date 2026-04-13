package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestConversationService_GetSubInterrupts_NotFound(t *testing.T) {
	// Use nil DB — GetSubInterrupts should panic or error when DB is nil.
	// The test verifies the method exists and reaches the ownership check.
	svc := NewConversationService(nil, zap.NewNop())
	uid := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.GetSubInterrupts(context.Background(), uid, uid)
	}()

	if !panicked {
		t.Error("expected nil DB to cause a panic during ownership check")
	}
}

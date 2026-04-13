package service

import "errors"

// Sentinel errors for service-layer error handling.
// These replace string-based error comparisons.
var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrProjectNotFound      = errors.New("project not found")
	ErrTodoNotFound         = errors.New("todo not found")
	ErrHITLNotFound         = errors.New("pending HITL request not found")
	ErrPermissionNotFound   = errors.New("pending permission request not found")
	ErrContextLimitExceeded = errors.New("context limit exceeded, compress conversation first")
)

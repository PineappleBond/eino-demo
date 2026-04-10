package runner

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AgentRunSession tracks the state of one agent execution.
type AgentRunSession struct {
	CancelFunc   context.CancelFunc
	CheckpointID string
	IsRunning    bool
	StopFunc     func()        // marks in-progress messages as stopped in DB
	done         chan struct{} // closed when the event loop goroutine fully drains
}

// RunSessionManager tracks active agent runs per conversation.
type RunSessionManager struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*AgentRunSession
}

// NewRunSessionManager creates a new session manager.
func NewRunSessionManager() *RunSessionManager {
	return &RunSessionManager{
		sessions: make(map[uuid.UUID]*AgentRunSession),
	}
}

// Start registers a new agent run with its cancel function.
func (m *RunSessionManager) Start(conversationID uuid.UUID, cancel context.CancelFunc, stopFunc func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[conversationID] = &AgentRunSession{
		CancelFunc: cancel,
		IsRunning:  true,
		StopFunc:   stopFunc,
		done:       make(chan struct{}),
	}
}

// Done signals that the event loop goroutine has fully drained.
func (m *RunSessionManager) Done(conversationID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[conversationID]; ok {
		select {
		case <-s.done:
			// already closed
		default:
			close(s.done)
		}
	}
}

// SetCheckpointID stores a checkpoint ID for later resume.
func (m *RunSessionManager) SetCheckpointID(conversationID uuid.UUID, checkpointID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[conversationID]; ok {
		s.CheckpointID = checkpointID
	}
}

// Stop cancels a running agent and waits for the event loop to drain.
// Safe to call when not running (no-op).
func (m *RunSessionManager) Stop(conversationID uuid.UUID) {
	m.mu.Lock()
	s, ok := m.sessions[conversationID]
	if !ok {
		m.mu.Unlock()
		return
	}
	if s.CancelFunc != nil {
		s.CancelFunc()
	}
	if s.StopFunc != nil {
		s.StopFunc()
	}
	s.IsRunning = false
	doneCh := s.done
	m.mu.Unlock()

	// Wait for the event loop goroutine to drain (with timeout).
	// Use a short timeout so the HTTP handler responds quickly to the stop request.
	select {
	case <-doneCh:
	case <-time.After(1 * time.Second):
	}
}

// GetCheckpointID returns the stored checkpoint ID, if any.
func (m *RunSessionManager) GetCheckpointID(conversationID uuid.UUID) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.sessions[conversationID]; ok {
		return s.CheckpointID, s.CheckpointID != ""
	}
	return "", false
}

// IsRunning checks whether an agent is currently executing.
func (m *RunSessionManager) IsRunning(conversationID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.sessions[conversationID]; ok {
		return s.IsRunning
	}
	return false
}

// Cleanup removes a session when the run is fully complete.
func (m *RunSessionManager) Cleanup(conversationID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, conversationID)
}

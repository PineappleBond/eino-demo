package runner

import (
	"context"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/google/uuid"
)

// AgentRunSession tracks the state of one agent execution.
type AgentRunSession struct {
	CancelFunc   context.CancelFunc
	CheckpointID string
	ResumeParams *adk.ResumeParams // Params for targeted resume with user data
	IsRunning    bool
	StopFunc     func()        // marks in-progress messages as stopped in DB
	done         chan struct{} // closed when the event loop goroutine fully drains
	ParentID     uuid.UUID     // if set, this session is a sub-agent of ParentID
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

// createSession is an internal helper. Caller must hold m.mu write lock.
func (m *RunSessionManager) createSession(conversationID uuid.UUID, cancel context.CancelFunc, stopFunc func(), parentID uuid.UUID) {
	m.sessions[conversationID] = &AgentRunSession{
		CancelFunc: cancel,
		IsRunning:  true,
		StopFunc:   stopFunc,
		done:       make(chan struct{}),
		ParentID:   parentID,
	}
}

// Start registers a new agent run with its cancel function.
func (m *RunSessionManager) Start(conversationID uuid.UUID, cancel context.CancelFunc, stopFunc func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createSession(conversationID, cancel, stopFunc, uuid.Nil)
}

// TryStart registers a new agent run only if no active session exists.
// If a placeholder session exists (created by SetResumeParams with IsRunning=false),
// it will be replaced with a real session, preserving checkpoint and resume params.
// The optional parentID parameter records a parent-child relationship for cascading stop.
// Returns true if the session was registered or replaced, false if an active session
// already exists. This prevents concurrent agent runs for the same conversation.
func (m *RunSessionManager) TryStart(conversationID uuid.UUID, cancel context.CancelFunc, stopFunc func(), parentID ...uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	pid := uuid.Nil
	if len(parentID) > 0 {
		pid = parentID[0]
	}

	if existing, ok := m.sessions[conversationID]; ok {
		// If an active agent is already running, don't replace it.
		if existing.IsRunning {
			return false
		}
		// Replace placeholder (e.g., from SetResumeParams) with a real session,
		// preserving checkpoint and resume params.
		m.createSession(conversationID, cancel, stopFunc, pid)
		m.sessions[conversationID].CheckpointID = existing.CheckpointID
		m.sessions[conversationID].ResumeParams = existing.ResumeParams
		return true
	}
	m.createSession(conversationID, cancel, stopFunc, pid)
	return true
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

// ClearCheckpointID removes the stored checkpoint ID. Called before a fresh run
// after compression to prevent resuming from stale state.
func (m *RunSessionManager) ClearCheckpointID(conversationID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[conversationID]; ok {
		s.CheckpointID = ""
	}
}

// Stop cancels a running agent and waits for the event loop to drain.
// If the session has child sessions (sub-agents), they are cancelled first.
// Safe to call when not running (no-op).
func (m *RunSessionManager) Stop(conversationID uuid.UUID) {
	m.stopWithLock(conversationID)
}

// stopWithLock cancels a session and all its descendants.
// Caller must NOT hold the lock — it acquires/releases the lock internally
// to avoid blocking the session map while calling cancel/stop/DB functions.
func (m *RunSessionManager) stopWithLock(conversationID uuid.UUID) {
	// First, collect all child session IDs to cascade stop.
	m.mu.RLock()
	childIDs := m.findChildrenLocked(conversationID)
	m.mu.RUnlock()

	// Stop all children first (they may have their own nested children).
	for _, childID := range childIDs {
		m.stopWithLock(childID)
	}

	m.mu.Lock()
	s, ok := m.sessions[conversationID]
	if !ok {
		m.mu.Unlock()
		return
	}
	cancelFn := s.CancelFunc
	stopFn := s.StopFunc
	s.IsRunning = false
	doneCh := s.done
	m.mu.Unlock()

	// Call cancel and stop outside the lock — StopFunc does DB writes that
	// should not block other concurrent operations on the session map.
	if cancelFn != nil {
		cancelFn()
	}
	if stopFn != nil {
		stopFn()
	}

	// Wait for the event loop goroutine to drain (with timeout).
	// Use a short timeout so the HTTP handler responds quickly to the stop request.
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}
}

// findChildrenLocked returns all session IDs that have conversationID as their parent.
// Caller must hold at least a read lock.
func (m *RunSessionManager) findChildrenLocked(conversationID uuid.UUID) []uuid.UUID {
	var children []uuid.UUID
	for id, s := range m.sessions {
		if s.ParentID == conversationID {
			children = append(children, id)
		}
	}
	return children
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

// SetResumeParams stores resume params for targeted resume with ResumeWithParams.
// If no session exists (e.g., already cleaned up by a previous run), it creates a
// minimal session so that params are not silently dropped.
func (m *RunSessionManager) SetResumeParams(conversationID uuid.UUID, params *adk.ResumeParams) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[conversationID]
	if !ok {
		// Session was already cleaned up — create a minimal placeholder so
		// params survive until runAgent picks them up.
		s = &AgentRunSession{
			IsRunning: false,
			done:      make(chan struct{}),
		}
		close(s.done) // mark as already done so no one waits on it
		m.sessions[conversationID] = s
	}
	s.ResumeParams = params
}

// GetResumeParams returns the stored resume params, if any.
func (m *RunSessionManager) GetResumeParams(conversationID uuid.UUID) (*adk.ResumeParams, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.sessions[conversationID]; ok {
		return s.ResumeParams, s.ResumeParams != nil
	}
	return nil, false
}

// ClearResumeParams removes stored resume params.
func (m *RunSessionManager) ClearResumeParams(conversationID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[conversationID]; ok {
		s.ResumeParams = nil
	}
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

// IsActive returns true if the conversation has a registered session
// (i.e. an agent run is in progress or has not yet been cleaned up).
func (m *RunSessionManager) IsActive(conversationID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessions[conversationID]
	return ok
}

// Cleanup removes a session when the run is fully complete.
func (m *RunSessionManager) Cleanup(conversationID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, conversationID)
}

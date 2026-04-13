package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Manager handles per-user WebSocket connections and seq-based update delivery.
type Manager struct {
	mu            sync.RWMutex
	conns         map[uuid.UUID][]*wsConn // user_id → connections
	maxConnsTotal int                     // global connection limit (0 = unlimited)
	connsTotal    int                     // current total connections across all users
	rdb           *redis.Client
	log           *zap.Logger
}

type wsConn struct {
	conn   *websocket.Conn
	send   chan []byte
	userID uuid.UUID
}

// ManagerConfig holds configuration for the WebSocket manager.
type ManagerConfig struct {
	// MaxConnsTotal limits the total number of concurrent WebSocket connections.
	// 0 means no limit. Default is 10000.
	MaxConnsTotal int
}

// NewManager creates a connection manager.
func NewManager(rdb *redis.Client, log *zap.Logger) *Manager {
	return NewManagerWithConfig(rdb, log, nil)
}

// NewManagerWithConfig creates a connection manager with custom config.
func NewManagerWithConfig(rdb *redis.Client, log *zap.Logger, cfg *ManagerConfig) *Manager {
	maxConns := 10000
	if cfg != nil && cfg.MaxConnsTotal > 0 {
		maxConns = cfg.MaxConnsTotal
	}
	return &Manager{
		conns:         make(map[uuid.UUID][]*wsConn),
		maxConnsTotal: maxConns,
		rdb:           rdb,
		log:           log,
	}
}

// AddConnection registers a new WebSocket for a user and starts read/write loops.
// connCtx is a per-connection context. connCancel is called by readLoop on all exit paths
// to signal ServeHTTP that the connection is done.
func (m *Manager) AddConnection(connCtx context.Context, userID uuid.UUID, conn *websocket.Conn, connCancel context.CancelFunc) error {
	m.mu.Lock()
	if m.maxConnsTotal > 0 && m.connsTotal >= m.maxConnsTotal {
		m.mu.Unlock()
		m.log.Warn("ws: connection limit reached", zap.Int("max", m.maxConnsTotal))
		return fmt.Errorf("connection limit reached")
	}
	m.connsTotal++
	wc := &wsConn{
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
	}
	m.conns[userID] = append(m.conns[userID], wc)
	m.mu.Unlock()

	// Start read/write loops with the per-connection context.
	go m.writeLoop(connCtx, wc)
	go m.readLoop(connCtx, wc, connCancel)
	return nil
}

// PushToUserConnections broadcasts an Update to all connections for a user.
func (m *Manager) PushToUserConnections(userID uuid.UUID, update Update) {
	data, err := json.Marshal(ServerFrame{
		Type:    FrameUpdates,
		Payload: []Update{update},
	})
	if err != nil {
		m.log.Error("ws: marshal failed", zap.Error(err))
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, wc := range m.conns[userID] {
		select {
		case wc.send <- data:
		default:
			m.log.Warn("ws: send channel full, dropping message")
		}
	}
}

// PushBatchToUserConnections broadcasts a batch of Updates to all connections for a user.
// Used when multiple updates need to be sent atomically (e.g., offline recovery).
func (m *Manager) PushBatchToUserConnections(userID uuid.UUID, updates []Update) {
	if len(updates) == 0 {
		return
	}
	data, err := json.Marshal(ServerFrame{
		Type:    FrameUpdates,
		Payload: updates,
	})
	if err != nil {
		m.log.Error("ws: marshal batch failed", zap.Error(err))
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, wc := range m.conns[userID] {
		select {
		case wc.send <- data:
		default:
			m.log.Warn("ws: send channel full, dropping batch")
		}
	}
}

// NextSeq assigns a monotonically increasing seq via Redis INCR.
func (m *Manager) NextSeq(ctx context.Context, userID uuid.UUID) (int64, error) {
	return m.rdb.Incr(ctx, "seq:"+userID.String()).Result()
}

// KickAllConnections closes and removes all connections for a user.
// Called during reconnection to prevent stale connections from accumulating.
func (m *Manager) KickAllConnections(userID uuid.UUID) {
	m.mu.Lock()
	conns := m.conns[userID]
	delete(m.conns, userID)
	m.mu.Unlock()

	for _, wc := range conns {
		wc.conn.Close(websocket.StatusGoingAway, "reconnected")
	}
}

// writeLoop sends frames to the client.
func (m *Manager) writeLoop(ctx context.Context, wc *wsConn) {
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-wc.send:
			if err := wc.conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

const heartbeatTimeout = 60 * time.Second

// readLoop handles incoming client frames (only ping).
// Uses a resettable timer: each received ping resets the 60s timeout.
// On all exit paths, readLoop calls connCancel to unblock ServeHTTP.
func (m *Manager) readLoop(ctx context.Context, wc *wsConn, connCancel context.CancelFunc) {
	defer connCancel()
	timer := time.NewTimer(heartbeatTimeout)
	defer timer.Stop()

	// Read in a separate goroutine so we can cancel it via ctx.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, msg, err := wc.conn.Read(ctx)
			if err != nil {
				return // ctx cancelled or connection closed
			}

			var frame ClientFrame
			if err := json.Unmarshal(msg, &frame); err != nil {
				continue
			}

			if frame.Type == FramePing {
				// Reset the timeout — connection is alive.
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(heartbeatTimeout)
			}
		}
	}()

	select {
	case <-ctx.Done():
		m.log.Info("ws: connection context cancelled", zap.String("user_id", wc.userID.String()))
		wc.conn.Close(websocket.StatusGoingAway, "closed")
		m.RemoveConnection(wc.userID, wc)
		<-readDone
	case <-timer.C:
		m.log.Warn("ws: heartbeat timeout, closing connection", zap.String("user_id", wc.userID.String()))
		wc.conn.Close(websocket.StatusGoingAway, "heartbeat timeout")
		m.RemoveConnection(wc.userID, wc)
		<-readDone
	}
}

// RemoveConnection removes a connection when it closes.
func (m *Manager) RemoveConnection(userID uuid.UUID, wc *wsConn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conns := m.conns[userID]
	for i, c := range conns {
		if c == wc {
			m.conns[userID] = append(conns[:i], conns[i+1:]...)
			if len(m.conns[userID]) == 0 {
				delete(m.conns, userID)
			}
			m.connsTotal--
			return
		}
	}
}

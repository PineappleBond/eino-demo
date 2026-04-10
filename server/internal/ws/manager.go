package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Manager handles per-user WebSocket connections and seq-based update delivery.
type Manager struct {
	mu    sync.RWMutex
	conns map[uuid.UUID][]*wsConn // user_id → connections
	rdb   *redis.Client
	log   *zap.Logger
}

type wsConn struct {
	conn   *websocket.Conn
	send   chan []byte
	userID uuid.UUID
}

// NewManager creates a connection manager.
func NewManager(rdb *redis.Client, log *zap.Logger) *Manager {
	return &Manager{
		conns: make(map[uuid.UUID][]*wsConn),
		rdb:   rdb,
		log:   log,
	}
}

// AddConnection registers a new WebSocket for a user and starts read/write loops.
func (m *Manager) AddConnection(ctx context.Context, userID uuid.UUID, conn *websocket.Conn) {
	wc := &wsConn{
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
	}

	m.mu.Lock()
	m.conns[userID] = append(m.conns[userID], wc)
	m.mu.Unlock()

	// Heartbeat timeout: 1 minute
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)

	go m.writeLoop(ctx, wc, cancel)
	go m.readLoop(ctx, wc, cancel)
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

// writeLoop sends frames to the client.
func (m *Manager) writeLoop(ctx context.Context, wc *wsConn, cancel context.CancelFunc) {
	defer cancel()
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

// readLoop handles incoming client frames (only ping).
func (m *Manager) readLoop(ctx context.Context, wc *wsConn, cancel context.CancelFunc) {
	defer cancel()
	for {
		_, msg, err := wc.conn.Read(ctx)
		if err != nil {
			return
		}

		var frame ClientFrame
		if err := json.Unmarshal(msg, &frame); err != nil {
			continue
		}

		if frame.Type == FramePing {
			// Reset the timeout — connection is alive
			// We rely on the parent context's timeout for eviction
		}
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
			return
		}
	}
}

package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/auth"
)

// WSHandler handles WebSocket upgrades.
type WSHandler struct {
	manager *Manager
	db      *gorm.DB
	log     *zap.Logger
}

// NewWSHandler creates a WebSocket handler.
func NewWSHandler(manager *Manager, db *gorm.DB, log *zap.Logger) *WSHandler {
	return &WSHandler{manager: manager, db: db, log: log}
}

// RegisterWSRoutes registers the /ws endpoint on the root router (not /api/v1).
func RegisterWSRoutes(r *gin.Engine, manager *Manager, db *gorm.DB, log *zap.Logger) {
	handler := NewWSHandler(manager, db, log)
	r.GET("/ws", gin.WrapH(handler))
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	userID, err := auth.ResolveTokenToUser(h.db, token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// last_seq is reserved for future offline replay logic
	_ = r.URL.Query().Get("last_seq")

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		OriginPatterns:     []string{"*"},
	})
	if err != nil {
		h.log.Error("ws: accept failed", zap.Error(err))
		return
	}

	// On reconnect, kick all stale connections for this user before adding the new one.
	// This prevents stale connections from accumulating and receiving duplicate pushes.
	h.manager.KickAllConnections(userID)

	// Create a per-connection context. Cancelled by readLoop on all exit paths
	// (heartbeat timeout, client disconnect, or reconnect kick).
	connCtx, connCancel := context.WithCancel(context.Background())

	// Send connected frame
	ctx := context.Background()
	maxSeq, _ := h.manager.rdb.Get(ctx, "seq:"+userID.String()).Int64()
	connectedData := ServerFrame{
		Type: FrameConnected,
		Payload: ConnectedPayload{
			UserID:     userID.String(),
			ServerTime: time.Now().UTC().Format(time.RFC3339),
			MaxSeq:     maxSeq,
		},
	}
	data, _ := json.Marshal(connectedData)
	if err := conn.Write(connCtx, websocket.MessageText, data); err != nil {
		h.log.Error("ws: connected frame write failed", zap.Error(err))
		return
	}

	h.manager.AddConnection(connCtx, userID, conn, connCancel)

	// Block until the connection is closed (heartbeat timeout, client disconnect, or reconnect kick).
	// The readLoop calls connCancel() on all exit paths, which unblocks this wait.
	// defer connCancel() handles cleanup if AddConnection returns early due to write error above.
	<-connCtx.Done()
}

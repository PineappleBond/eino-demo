package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/auth"
	"github.com/PineappleBond/eino-demo-dev/server/internal/convert"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
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

	// Parse last_seq for offline replay
	lastSeqStr := r.URL.Query().Get("last_seq")
	var lastSeq int64
	if lastSeqStr != "" {
		fmt.Sscanf(lastSeqStr, "%d", &lastSeq)
	}

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
	defer connCancel()

	// Send connected frame
	ctx := context.Background()
	maxSeq, _ := h.manager.rdb.Get(ctx, "seq:"+userID.String()).Int64()

	// If Redis is stale (maxSeq=0 or maxSeq < lastSeq), fall back to DB
	if maxSeq == 0 || maxSeq < lastSeq {
		var dbMaxSeq int64
		h.db.WithContext(ctx).Raw("SELECT COALESCE(MAX(seq), 0) FROM user_updates WHERE user_id = ?", userID).Scan(&dbMaxSeq)
		if dbMaxSeq > maxSeq {
			maxSeq = dbMaxSeq
			// Sync Redis counter
			h.manager.rdb.Set(ctx, "seq:"+userID.String(), maxSeq, 0)
		}
	}

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

	// If client provided last_seq, replay missed updates
	if lastSeq > 0 && lastSeq < maxSeq {
		var updates []model.UserUpdate
		if err := h.db.WithContext(ctx).
			Where("user_id = ? AND seq > ? AND seq <= ?", userID, lastSeq, maxSeq).
			Order("seq ASC").
			Limit(500).
			Find(&updates).Error; err != nil {
			h.log.Error("ws: replay query failed", zap.Error(err))
		} else if len(updates) > 0 {
			replay := make([]Update, len(updates))
			for i, u := range updates {
				replay[i] = convert.ToUpdate(u)
			}
			h.manager.PushBatchToUserConnections(userID, replay)
		}
	}

	// Register connection (enforces connection limit)
	if err := h.manager.AddConnection(connCtx, userID, conn, connCancel); err != nil {
		h.log.Error("ws: add connection failed", zap.Error(err))
		conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}

	// Block until the connection is closed (heartbeat timeout, client disconnect, or reconnect kick).
	// The readLoop calls connCancel() on all exit paths, which unblocks this wait.
	<-connCtx.Done()
}

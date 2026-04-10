package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

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
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Resolve token to user UUID (same logic as auth middleware)
	userID := uuid.NewSHA1(uuid.Nil, []byte(token))
	var user model.User
	result := h.db.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		user.ID = userID
		h.db.Create(&user)
	}

	// last_seq is reserved for future offline replay logic
	_ = r.URL.Query().Get("last_seq")

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		h.log.Error("ws: accept failed", zap.Error(err))
		return
	}

	ctx := context.Background()

	// Send connected frame
	maxSeq, _ := h.manager.rdb.Get(ctx, "seq:"+user.ID.String()).Int64()
	connectedData := ServerFrame{
		Type: FrameConnected,
		Payload: ConnectedPayload{
			UserID:     user.ID.String(),
			ServerTime: time.Now().UTC().Format(time.RFC3339),
			MaxSeq:     maxSeq,
		},
	}
	data, _ := json.Marshal(connectedData)
	conn.Write(ctx, websocket.MessageText, data)

	h.manager.AddConnection(ctx, user.ID, conn)
}

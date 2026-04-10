package ws

import "encoding/json"

// FrameType identifies WebSocket frame types.
type FrameType string

const (
	FrameConnected FrameType = "connected"
	FrameUpdates   FrameType = "updates"
	FramePing      FrameType = "ping"
)

// ServerFrame is a server→client WebSocket envelope.
type ServerFrame struct {
	Type    FrameType `json:"type"`
	Payload any       `json:"payload"`
}

// ClientFrame is a client→server WebSocket envelope.
type ClientFrame struct {
	Type    FrameType      `json:"type"`
	Payload map[string]any `json:"payload"`
}

// ConnectedPayload is sent when a client connects.
type ConnectedPayload struct {
	UserID     string `json:"user_id"`
	ServerTime string `json:"server_time"`
	MaxSeq     int64  `json:"max_seq"`
}

// MarshalJSON encodes a ServerFrame to JSON.
func (f ServerFrame) MarshalJSON() ([]byte, error) {
	type Alias ServerFrame
	return json.Marshal(Alias(f))
}

// Update represents a single Update event (matches the OpenAPI Update schema).
type Update struct {
	Seq     int64          `json:"seq"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

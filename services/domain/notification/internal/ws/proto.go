package ws

import (
	"encoding/json"
	"time"
)

// Frame is the top-level JSON message exchanged over the WebSocket.
type Frame struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"` // client correlation id
	Data json.RawMessage `json:"data,omitempty"`
}

func marshal(f Frame) ([]byte, error) {
	return json.Marshal(f)
}

// ── Server → Client frames ────────────────────────────────────────────────────

func ConnectedFrame(connID string, autoSubscribed []string) ([]byte, error) {
	return marshal(Frame{
		Type: "connected",
		Data: mustJSON(map[string]any{
			"connection_id":   connID,
			"server_time":     time.Now().UTC(),
			"auto_subscribed": autoSubscribed,
		}),
	})
}

func SubscribeAckFrame(correlationID string, subscribed, denied []string, deniedReasons map[string]string) ([]byte, error) {
	deniedList := make([]map[string]string, 0, len(denied))
	for _, ch := range denied {
		deniedList = append(deniedList, map[string]string{"channel": ch, "reason": deniedReasons[ch]})
	}
	return marshal(Frame{
		Type: "subscribe_ack",
		ID:   correlationID,
		Data: mustJSON(map[string]any{
			"subscribed": subscribed,
			"denied":     deniedList,
		}),
	})
}

func EventFrame(channel, eventType string, occurredAt time.Time, payload any) ([]byte, error) {
	return marshal(Frame{
		Type: "event",
		Data: mustJSON(map[string]any{
			"channel":     channel,
			"event_type":  eventType,
			"occurred_at": occurredAt,
			"payload":     payload,
		}),
	})
}

func NotificationFrame(notifID, notifType string, createdAt time.Time, payload any) ([]byte, error) {
	return marshal(Frame{
		Type: "notification",
		Data: mustJSON(map[string]any{
			"notification_id":   notifID,
			"notification_type": notifType,
			"created_at":        createdAt,
			"payload":           payload,
		}),
	})
}

func PongFrame() ([]byte, error) {
	return marshal(Frame{
		Type: "pong",
		Data: mustJSON(map[string]any{"server_time": time.Now().UTC()}),
	})
}

func ErrorFrame(correlationID, code, message string) ([]byte, error) {
	return marshal(Frame{
		Type: "error",
		ID:   correlationID,
		Data: mustJSON(map[string]any{"code": code, "message": message}),
	})
}

// ── Client → Server message types ────────────────────────────────────────────

type InboundFrame struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type SubscribeData struct {
	Channels []string `json:"channels"`
}

type UnsubscribeData struct {
	Channels []string `json:"channels"`
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

package events

import "encoding/json"

// Envelope wraps a RabbitMQ message.
type Envelope struct {
	EventType string
	MessageID string
	Payload   []byte
}

func unmarshal(payload []byte, v any) error {
	return json.Unmarshal(payload, v)
}

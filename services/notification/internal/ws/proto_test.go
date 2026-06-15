package ws_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/notification/internal/ws"
)

func TestConnectedFrame(t *testing.T) {
	data, err := ws.ConnectedFrame("conn-123", []string{"user:abc"})
	require.NoError(t, err)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))
	assert.Equal(t, "connected", frame["type"])
	d := frame["data"].(map[string]any)
	assert.Equal(t, "conn-123", d["connection_id"])
}

func TestSubscribeAckFrame(t *testing.T) {
	data, err := ws.SubscribeAckFrame("corr-1", []string{"board:X"}, []string{"task:Y"}, map[string]string{"task:Y": "permission_denied"})
	require.NoError(t, err)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))
	assert.Equal(t, "subscribe_ack", frame["type"])
	assert.Equal(t, "corr-1", frame["id"])
}

func TestEventFrame(t *testing.T) {
	payload := map[string]any{"task_id": "abc"}
	data, err := ws.EventFrame("board:X", "task.created", time.Now(), payload)
	require.NoError(t, err)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))
	assert.Equal(t, "event", frame["type"])
}

func TestNotificationFrame(t *testing.T) {
	data, err := ws.NotificationFrame("notif-1", "mention", time.Now(), map[string]any{})
	require.NoError(t, err)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))
	assert.Equal(t, "notification", frame["type"])
}

func TestPongFrame(t *testing.T) {
	data, err := ws.PongFrame()
	require.NoError(t, err)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))
	assert.Equal(t, "pong", frame["type"])
}

func TestErrorFrame(t *testing.T) {
	data, err := ws.ErrorFrame("corr-1", "invalid_channel", "bad channel format")
	require.NoError(t, err)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))
	assert.Equal(t, "error", frame["type"])
	d := frame["data"].(map[string]any)
	assert.Equal(t, "invalid_channel", d["code"])
}

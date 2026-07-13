package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/teamboard/services/notification/internal/domain"
)

const (
	sendBufSize    = 64
	writeTimeout   = 10 * time.Second
	pingInterval   = 30 * time.Second
	idleTimeout    = 60 * time.Second
	maxMessageSize = 8192
)

// Conn wraps a gorilla WebSocket connection with subscription tracking.
// All writes are serialised through the send channel to a single writer goroutine.
type Conn struct {
	mu           sync.RWMutex
	conn         *domain.Connection
	ws           *gorillaws.Conn
	send         chan []byte
	subscriptions map[domain.Channel]struct{}
	hub          *Hub
	closeOnce    sync.Once
}

func NewConn(ws *gorillaws.Conn, dc *domain.Connection, hub *Hub) *Conn {
	return &Conn{
		conn:          dc,
		ws:            ws,
		send:          make(chan []byte, sendBufSize),
		subscriptions: make(map[domain.Channel]struct{}),
		hub:           hub,
	}
}

// Send enqueues data for writing. Drops + closes if the buffer is full (slow consumer).
func (c *Conn) Send(data []byte) {
	select {
	case c.send <- data:
	default:
		c.Close(gorillaws.CloseNormalClosure, "slow consumer")
	}
}

func (c *Conn) Close(code int, reason string) {
	c.closeOnce.Do(func() {
		_ = c.ws.WriteControl(gorillaws.CloseMessage,
			gorillaws.FormatCloseMessage(code, reason),
			time.Now().Add(writeTimeout))
		_ = c.ws.Close()
		close(c.send)
	})
}

func (c *Conn) AddSubscription(ch domain.Channel) {
	c.mu.Lock()
	c.subscriptions[ch] = struct{}{}
	c.mu.Unlock()
}

func (c *Conn) RemoveSubscription(ch domain.Channel) {
	c.mu.Lock()
	delete(c.subscriptions, ch)
	c.mu.Unlock()
}

func (c *Conn) IsSubscribed(ch domain.Channel) bool {
	c.mu.RLock()
	_, ok := c.subscriptions[ch]
	c.mu.RUnlock()
	return ok
}

func (c *Conn) SubscriptionCount() int {
	c.mu.RLock()
	n := len(c.subscriptions)
	c.mu.RUnlock()
	return n
}

// RunWriter pumps frames from the send channel to the WebSocket.
// Exits when the send channel is closed.
func (c *Conn) RunWriter() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.ws.WriteMessage(gorillaws.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.ws.WriteMessage(gorillaws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// RunReader reads frames from the WebSocket and dispatches them to handler.
// Exits when the connection is closed or context is cancelled.
type FrameHandler func(ctx context.Context, c *Conn, frame InboundFrame)

func (c *Conn) RunReader(ctx context.Context, handler FrameHandler) {
	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(idleTimeout))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(idleTimeout))
	})

	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			if !gorillaws.IsUnexpectedCloseError(err, gorillaws.CloseGoingAway, gorillaws.CloseNormalClosure) {
				slog.ErrorContext(ctx, "ws read error", "conn_id", c.conn.ID, "error", err)
			}
			return
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(idleTimeout))

		var frame InboundFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			errFrame, _ := ErrorFrame(frame.ID, "invalid_frame", "JSON parse error")
			c.Send(errFrame)
			continue
		}
		handler(ctx, c, frame)
	}
}

func (c *Conn) DomainConn() *domain.Connection { return c.conn }

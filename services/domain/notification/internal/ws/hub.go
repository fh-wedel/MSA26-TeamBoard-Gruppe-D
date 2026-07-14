package ws

import (
	"sync"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/notification/internal/domain"
)

// Hub is an in-process registry of WebSocket connections for this instance.
// Each connection is registered here on connect and removed on disconnect.
type Hub struct {
	mu          sync.RWMutex
	connections map[string]*Conn // connID → Conn
	userConns   map[uuid.UUID]map[string]*Conn // userID → connID → Conn
}

func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*Conn),
		userConns:   make(map[uuid.UUID]map[string]*Conn),
	}
}

func (h *Hub) Register(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[c.conn.ID] = c
	if h.userConns[c.conn.UserID] == nil {
		h.userConns[c.conn.UserID] = make(map[string]*Conn)
	}
	h.userConns[c.conn.UserID][c.conn.ID] = c
}

func (h *Hub) Unregister(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, c.conn.ID)
	if uc, ok := h.userConns[c.conn.UserID]; ok {
		delete(uc, c.conn.ID)
		if len(uc) == 0 {
			delete(h.userConns, c.conn.UserID)
		}
	}
}

// SendToUser delivers a frame to all local connections for userID.
func (h *Hub) SendToUser(userID uuid.UUID, data []byte) {
	h.mu.RLock()
	conns := h.userConns[userID]
	targets := make([]*Conn, 0, len(conns))
	for _, c := range conns {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.Send(data)
	}
}

// SendToChannel delivers a frame to all local connections subscribed to channel.
func (h *Hub) SendToChannel(channel domain.Channel, data []byte) {
	h.mu.RLock()
	targets := make([]*Conn, 0)
	for _, c := range h.connections {
		if c.IsSubscribed(channel) {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.Send(data)
	}
}

// UnsubscribeFromChannel removes channel from all local connections of a user.
func (h *Hub) UnsubscribeFromChannel(userID uuid.UUID, channel domain.Channel) {
	h.mu.RLock()
	conns := h.userConns[userID]
	targets := make([]*Conn, 0, len(conns))
	for _, c := range conns {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.RemoveSubscription(channel)
	}
}

// ConnectionCount returns the number of active connections.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

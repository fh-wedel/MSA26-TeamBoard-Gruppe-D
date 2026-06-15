package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	gorillaws "github.com/gorilla/websocket"
	"github.com/google/uuid"
	"github.com/teamboard/services/notification/internal/domain"
	"github.com/teamboard/services/notification/internal/push"
	"github.com/teamboard/services/notification/internal/ws"
)

var upgrader = gorillaws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // CORS handled by Traefik
}

func handleWebSocket(pushSvc *push.Service, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Token is in query string for WebSocket (browsers can't set headers on WS upgrade).
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		userID, err := extractSubFromToken(token)
		if err != nil || userID == uuid.Nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		rawConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.ErrorContext(r.Context(), "ws upgrade failed", "error", err)
			return
		}

		connID := uuid.New().String()
		dc := &domain.Connection{
			ID:     connID,
			UserID: userID,
			Send:   make(chan []byte, 64),
		}
		wc := ws.NewConn(rawConn, dc, hub)

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Register connection.
		if err := pushSvc.RegisterConnection(ctx, wc); err != nil {
			slog.ErrorContext(ctx, "ws register connection failed", "error", err)
			wc.Close(gorillaws.CloseInternalServerErr, "registration failed")
			return
		}
		defer pushSvc.DeregisterConnection(context.Background(), wc)

		// Auto-subscribe to user channel.
		userCh := string(domain.UserChannel(userID))
		wc.AddSubscription(domain.UserChannel(userID))

		// Send connected frame.
		connectedFrame, _ := ws.ConnectedFrame(connID, []string{userCh})
		wc.Send(connectedFrame)

		// Start writer goroutine.
		go wc.RunWriter()

		// Read loop (blocks until disconnect).
		wc.RunReader(ctx, func(ctx context.Context, c *ws.Conn, frame ws.InboundFrame) {
			handleWSFrame(ctx, c, frame, pushSvc)
		})
	}
}

func handleWSFrame(ctx context.Context, wc *ws.Conn, frame ws.InboundFrame, pushSvc *push.Service) {
	switch frame.Type {
	case "ping":
		pong, _ := ws.PongFrame()
		wc.Send(pong)

	case "subscribe":
		var data ws.SubscribeData
		if err := json.Unmarshal(frame.Data, &data); err != nil {
			errFrame, _ := ws.ErrorFrame(frame.ID, "invalid_frame", "bad subscribe data")
			wc.Send(errFrame)
			return
		}
		ok, denied, reasons := pushSvc.Subscribe(ctx, wc, data.Channels)
		ack, _ := ws.SubscribeAckFrame(frame.ID, ok, denied, reasons)
		wc.Send(ack)

	case "unsubscribe":
		var data ws.UnsubscribeData
		if err := json.Unmarshal(frame.Data, &data); err != nil {
			return
		}
		pushSvc.Unsubscribe(ctx, wc, data.Channels)

	default:
		errFrame, _ := ws.ErrorFrame(frame.ID, "unknown_frame_type", "unknown type: "+frame.Type)
		wc.Send(errFrame)
	}
}

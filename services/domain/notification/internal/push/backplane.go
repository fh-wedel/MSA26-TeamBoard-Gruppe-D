package push

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"github.com/teamboard/services/domain/notification/internal/domain"
	"github.com/teamboard/services/domain/notification/internal/ws"
)

const broadcastPattern = "broadcast:*"

// Backplane publishes messages to Redis Pub/Sub and fans them out to local connections.
type Backplane struct {
	rdb *redis.Client
	hub *ws.Hub
}

func NewBackplane(rdb *redis.Client, hub *ws.Hub) *Backplane {
	return &Backplane{rdb: rdb, hub: hub}
}

// Publish sends data to all instances subscribed to the given channel key.
func (b *Backplane) Publish(ctx context.Context, redisChannel string, data []byte) error {
	return b.rdb.Publish(ctx, redisChannel, data).Err()
}

// RunSubscriber listens on the broadcast:* pattern and routes incoming messages
// to local connections via the hub.
func (b *Backplane) RunSubscriber(ctx context.Context) {
	sub := b.rdb.PSubscribe(ctx, broadcastPattern)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			b.route(msg)
		}
	}
}

func (b *Backplane) route(msg *redis.Message) {
	// Pattern: "broadcast:user:{uuid}" → push to user
	//          "broadcast:channel:{channel-string}" → push to channel
	data := []byte(msg.Payload)

	// "broadcast:" prefix is 10 chars
	suffix := msg.Channel[len("broadcast:"):]

	if len(suffix) > 5 && suffix[:5] == "user:" {
		idStr := suffix[5:]
		userID, err := parseUUID(idStr)
		if err != nil {
			slog.Warn("backplane: invalid user uuid", "channel", msg.Channel)
			return
		}
		b.hub.SendToUser(userID, data)
		return
	}

	// Otherwise treat as a domain channel string
	b.hub.SendToChannel(domain.Channel(suffix), data)
}

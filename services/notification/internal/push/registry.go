package push

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/teamboard/services/notification/internal/domain"
)

const (
	connTTL          = 60 * time.Second
	heartbeatInterval = 30 * time.Second
)

// Registry tracks WebSocket connections in Redis for cross-instance routing.
type Registry struct {
	rdb        *redis.Client
	instanceID string
}

func NewRegistry(rdb *redis.Client, instanceID string) *Registry {
	return &Registry{rdb: rdb, instanceID: instanceID}
}

func (r *Registry) RegisterConnection(ctx context.Context, conn *domain.Connection) error {
	key := fmt.Sprintf("conn:%s:%s", conn.UserID, conn.ID)
	pipe := r.rdb.Pipeline()
	pipe.Set(ctx, key, r.instanceID, connTTL)
	pipe.SAdd(ctx, fmt.Sprintf("user_conns:%s", conn.UserID), conn.ID)
	pipe.Expire(ctx, fmt.Sprintf("user_conns:%s", conn.UserID), connTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Registry) DeregisterConnection(ctx context.Context, conn *domain.Connection) error {
	key := fmt.Sprintf("conn:%s:%s", conn.UserID, conn.ID)
	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, fmt.Sprintf("user_conns:%s", conn.UserID), conn.ID)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Registry) AddChannelMember(ctx context.Context, channel domain.Channel, connID string) error {
	return r.rdb.SAdd(ctx, "channel:"+string(channel), connID).Err()
}

func (r *Registry) RemoveChannelMember(ctx context.Context, channel domain.Channel, connID string) error {
	return r.rdb.SRem(ctx, "channel:"+string(channel), connID).Err()
}

// RefreshConnection extends the TTL of a connection entry (heartbeat).
func (r *Registry) RefreshConnection(ctx context.Context, conn *domain.Connection) {
	key := fmt.Sprintf("conn:%s:%s", conn.UserID, conn.ID)
	r.rdb.Expire(ctx, key, connTTL)
	r.rdb.Expire(ctx, fmt.Sprintf("user_conns:%s", conn.UserID), connTTL)
}

// StartHeartbeat periodically refreshes the TTL for a connection.
func (r *Registry) StartHeartbeat(ctx context.Context, conn *domain.Connection) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.RefreshConnection(ctx, conn)
		}
	}
}

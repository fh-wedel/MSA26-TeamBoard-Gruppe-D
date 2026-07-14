package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/teamboard/services/domain/project/internal/domain"
)

const defaultTTL = 60 * time.Second

type redisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedis returns a Redis-backed PermissionCache.
func NewRedis(client *redis.Client) domain.PermissionCache {
	return &redisCache{client: client, ttl: defaultTTL}
}

func (c *redisCache) Get(ctx context.Context, projectID, userID uuid.UUID) (*domain.PermissionSet, error) {
	data, err := c.client.Get(ctx, key(projectID, userID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var ps domain.PermissionSet
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, err
	}
	return &ps, nil
}

func (c *redisCache) Set(ctx context.Context, projectID, userID uuid.UUID, ps *domain.PermissionSet) error {
	data, err := json.Marshal(ps)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key(projectID, userID), data, c.ttl).Err()
}

func (c *redisCache) Delete(ctx context.Context, projectID, userID uuid.UUID) error {
	return c.client.Del(ctx, key(projectID, userID)).Err()
}

func key(projectID, userID uuid.UUID) string {
	return fmt.Sprintf("perm:%s:%s", projectID, userID)
}

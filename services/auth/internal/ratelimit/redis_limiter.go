package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/teamboard/services/auth/internal/domain"
)

type redisLimiter struct {
	client *redis.Client
}

// NewRedisLimiter returns a sliding-window rate limiter backed by Redis.
func NewRedisLimiter(client *redis.Client) domain.RateLimiter {
	return &redisLimiter{client: client}
}

var _ domain.RateLimiter = (*redisLimiter)(nil)

// Allow uses a Redis sorted-set sliding window.
// The set stores request timestamps (in ms) as both score and member.
func (l *redisLimiter) Allow(ctx context.Context, key string, maxAttempts int, window time.Duration) (bool, error) {
	now := time.Now().UnixMilli()
	cutoff := now - window.Milliseconds()
	rKey := "ratelimit:" + key

	pipe := l.client.TxPipeline()
	// remove entries outside the window
	pipe.ZRemRangeByScore(ctx, rKey, "0", fmt.Sprintf("%d", cutoff))
	// count remaining
	countCmd := pipe.ZCard(ctx, rKey)
	// add current request
	pipe.ZAdd(ctx, rKey, redis.Z{Score: float64(now), Member: now})
	// expire the key slightly beyond the window
	pipe.Expire(ctx, rKey, window+time.Second)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("redis pipeline: %w", err)
	}

	return countCmd.Val() < int64(maxAttempts), nil
}

package authmiddleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
)

// JWKSSource fetches and caches public keys for JWT validation.
type JWKSSource interface {
	Key(ctx context.Context, kid string) (any, error)
}

// JWKSOption configures a JWKS source.
type JWKSOption func(*jwksConfig)

type jwksConfig struct {
	cacheTTL time.Duration
}

// WithJWKSCacheTTL sets how long fetched keys are cached.
func WithJWKSCacheTTL(ttl time.Duration) JWKSOption {
	return func(c *jwksConfig) { c.cacheTTL = ttl }
}

type jwksSource struct {
	url      string
	cacheTTL time.Duration

	mu      sync.RWMutex
	set     jwk.Set
	fetched time.Time
}

// NewJWKSSource creates a JWKSSource that fetches from url and caches results.
func NewJWKSSource(url string, opts ...JWKSOption) JWKSSource {
	cfg := &jwksConfig{cacheTTL: 10 * time.Minute}
	for _, o := range opts {
		o(cfg)
	}
	return &jwksSource{url: url, cacheTTL: cfg.cacheTTL}
}

func (j *jwksSource) Key(ctx context.Context, kid string) (any, error) {
	j.mu.RLock()
	if j.set != nil && time.Since(j.fetched) < j.cacheTTL {
		if k := j.lookupKID(kid); k != nil {
			j.mu.RUnlock()
			return k, nil
		}
	}
	j.mu.RUnlock()

	if err := j.refresh(ctx); err != nil {
		return nil, fmt.Errorf("refresh JWKS: %w", err)
	}

	j.mu.RLock()
	defer j.mu.RUnlock()
	if k := j.lookupKID(kid); k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("kid %q not found in JWKS", kid)
}

func (j *jwksSource) lookupKID(kid string) any {
	if j.set == nil {
		return nil
	}
	k, ok := j.set.LookupKeyID(kid)
	if !ok {
		return nil
	}
	var raw any
	if err := k.Raw(&raw); err != nil {
		return nil
	}
	return raw
}

func (j *jwksSource) refresh(ctx context.Context) error {
	set, err := jwk.Fetch(ctx, j.url)
	if err != nil {
		return err
	}
	j.mu.Lock()
	j.set = set
	j.fetched = time.Now()
	j.mu.Unlock()
	return nil
}

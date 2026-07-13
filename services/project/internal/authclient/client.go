// Package authclient resolves personal access tokens (issued by the auth
// service) to their owning user, so this service's public API can accept
// them as Bearer tokens alongside normal session JWTs.
package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sony/gobreaker"

	"github.com/teamboard/shared/go/servicetoken"
)

const cacheTTL = 30 * time.Second

type cacheEntry struct {
	userID uuid.UUID
	email  string
	expiry time.Time
}

// Client calls the auth service's internal token-introspection endpoint with
// a circuit breaker and an in-process cache — mirrors projectclient.Client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	issuer     servicetoken.Issuer
	breaker    *gobreaker.CircuitBreaker
	cache      sync.Map
}

func New(baseURL string, issuer servicetoken.Issuer) *Client {
	settings := gobreaker.Settings{
		Name:        "auth-service",
		MaxRequests: 1,
		Interval:    30 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 200 * time.Millisecond},
		issuer:     issuer,
		breaker:    gobreaker.NewCircuitBreaker(settings),
	}
}

// Introspect satisfies authmiddleware.TokenIntrospector.
func (c *Client) Introspect(ctx context.Context, token string) (uuid.UUID, string, error) {
	if v, ok := c.cache.Load(token); ok {
		entry := v.(*cacheEntry)
		if time.Now().Before(entry.expiry) {
			return entry.userID, entry.email, nil
		}
		c.cache.Delete(token)
	}

	result, err := c.breaker.Execute(func() (any, error) {
		return c.fetch(ctx, token)
	})
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("auth client: %w", err)
	}

	entry := result.(*cacheEntry)
	c.cache.Store(token, entry)
	return entry.userID, entry.email, nil
}

func (c *Client) fetch(ctx context.Context, token string) (*cacheEntry, error) {
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, err
	}
	url := c.baseURL + "/api/v1/internal/tokens/introspect"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	tok, err := c.issuer.Issue(ctx, "project-service", "internal")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth service returned %d", resp.StatusCode)
	}

	var wrapper struct {
		Data struct {
			UserID string `json:"user_id"`
			Email  string `json:"email"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(wrapper.Data.UserID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	return &cacheEntry{userID: userID, email: wrapper.Data.Email, expiry: time.Now().Add(cacheTTL)}, nil
}

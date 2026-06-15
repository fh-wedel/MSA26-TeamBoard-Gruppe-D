package projectclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sony/gobreaker"
	"github.com/teamboard/services/document/internal/domain"
)

const cacheTTL = 30 * time.Second

type cacheEntry struct {
	perms  *domain.PermissionSet
	expiry time.Time
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	breaker    *gobreaker.CircuitBreaker
	cache      sync.Map
}

func New(baseURL, serviceTokenSecret string) *Client {
	mac := hmac.New(sha256.New, []byte(serviceTokenSecret))
	mac.Write([]byte("internal"))
	token := hex.EncodeToString(mac.Sum(nil))

	settings := gobreaker.Settings{
		Name:        "project-service",
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
		token:      token,
		breaker:    gobreaker.NewCircuitBreaker(settings),
	}
}

func (c *Client) GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*domain.PermissionSet, error) {
	key := projectID.String() + ":" + userID.String()

	if v, ok := c.cache.Load(key); ok {
		entry := v.(*cacheEntry)
		if time.Now().Before(entry.expiry) {
			return entry.perms, nil
		}
		c.cache.Delete(key)
	}

	result, err := c.breaker.Execute(func() (any, error) {
		return c.fetch(ctx, projectID, userID)
	})
	if err != nil {
		return nil, fmt.Errorf("project client: %w", err)
	}

	perms := result.(*domain.PermissionSet)
	c.cache.Store(key, &cacheEntry{perms: perms, expiry: time.Now().Add(cacheTTL)})
	return perms, nil
}

func (c *Client) InvalidateCache(projectID, userID uuid.UUID) {
	c.cache.Delete(projectID.String() + ":" + userID.String())
}

func (c *Client) fetch(ctx context.Context, projectID, userID uuid.UUID) (*domain.PermissionSet, error) {
	url := fmt.Sprintf("%s/api/v1/internal/projects/%s/permissions/%s", c.baseURL, projectID, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("project service returned %d", resp.StatusCode)
	}

	var wrapper struct {
		Data *domain.PermissionSet `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
}

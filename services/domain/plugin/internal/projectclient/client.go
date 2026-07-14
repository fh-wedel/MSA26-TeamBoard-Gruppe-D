package projectclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/shared/go/servicetoken"
)

type permCacheEntry struct {
	permissions []string
	expiresAt   time.Time
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	issuer     servicetoken.Issuer
	cacheTTL   time.Duration
	cache      sync.Map // key: "projectID:userID"
}

func New(baseURL string, issuer servicetoken.Issuer, timeout, cacheTTL time.Duration) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
		issuer:     issuer,
		cacheTTL:   cacheTTL,
	}
}

// HasPermission checks if the user has the given permission on the project.
func (c *Client) HasPermission(ctx context.Context, projectID, userID uuid.UUID, permission string) (bool, error) {
	perms, err := c.getPermissions(ctx, projectID, userID)
	if err != nil {
		return false, err
	}
	for _, p := range perms {
		if p == permission || p == "admin" || p == "owner" {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) getPermissions(ctx context.Context, projectID, userID uuid.UUID) ([]string, error) {
	key := projectID.String() + ":" + userID.String()
	if v, ok := c.cache.Load(key); ok {
		entry := v.(*permCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.permissions, nil
		}
	}

	url := fmt.Sprintf("%s/api/v1/internal/projects/%s/permissions/%s", c.baseURL, projectID, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	tok, err := c.issuer.Issue(ctx, "plugin-service", "internal")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("project service returned %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Permissions []string `json:"permissions"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	c.cache.Store(key, &permCacheEntry{
		permissions: body.Data.Permissions,
		expiresAt:   time.Now().Add(c.cacheTTL),
	})
	return body.Data.Permissions, nil
}

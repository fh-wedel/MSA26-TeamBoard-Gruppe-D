// Package boardtypeclient is the project service's runtime client for the board
// registry service. It implements domain.BoardTypeRegistry by fetching board-type
// definitions over the registry's internal HTTP API, caching them with a TTL.
package boardtypeclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/teamboard/services/domain/project/internal/domain"
	"github.com/teamboard/shared/go/servicetoken"
)

type cacheEntry struct {
	def       *domain.BoardTypeDef
	expiresAt time.Time
}

// Client fetches board-type definitions from the board registry service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	issuer     servicetoken.Issuer
	cacheTTL   time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

// New builds a client. issuer mints short-lived service tokens (aud "internal")
// for the registry's internal API, signed with the shared service-token secret.
func New(baseURL string, issuer servicetoken.Issuer, timeout, cacheTTL time.Duration) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
		issuer:     issuer,
		cacheTTL:   cacheTTL,
		cache:      make(map[string]cacheEntry),
	}
}

var _ domain.BoardTypeRegistry = (*Client)(nil)

// boardTypeDTO mirrors the registry's internal response shape.
type boardTypeDTO struct {
	Type           string `json:"type"`
	DisplayName    string `json:"display_name"`
	Icon           string `json:"icon"`
	DefaultColumns []struct {
		Name     string `json:"name"`
		Position int    `json:"position"`
		WIPLimit *int   `json:"wip_limit"`
		Status   string `json:"status"`
	} `json:"default_columns"`
	DefaultConfig map[string]any `json:"default_config"`
	ConfigSchema  map[string]any `json:"config_schema"`
}

// GetType returns the board-type definition for typeID, using the cache when fresh.
func (c *Client) GetType(ctx context.Context, typeID string) (*domain.BoardTypeDef, error) {
	c.mu.RLock()
	e, ok := c.cache[typeID]
	c.mu.RUnlock()
	if ok && time.Now().Before(e.expiresAt) {
		return e.def, nil
	}

	url := fmt.Sprintf("%s/api/v1/internal/board-types/%s", c.baseURL, typeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	tok, err := c.issuer.Issue(ctx, "project-service", "internal")
	if err != nil {
		return nil, domain.ErrBoardTypeRegistryUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, domain.ErrBoardTypeRegistryUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrInvalidBoardType
	}
	if resp.StatusCode != http.StatusOK {
		return nil, domain.ErrBoardTypeRegistryUnavailable
	}

	var body struct {
		Data boardTypeDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, domain.ErrBoardTypeRegistryUnavailable
	}

	def := toDomain(body.Data)
	c.mu.Lock()
	c.cache[typeID] = cacheEntry{def: def, expiresAt: time.Now().Add(c.cacheTTL)}
	c.mu.Unlock()
	return def, nil
}

// Invalidate drops a single cached type (called on boardtype.* events).
func (c *Client) Invalidate(typeID string) {
	c.mu.Lock()
	delete(c.cache, typeID)
	c.mu.Unlock()
}

func toDomain(d boardTypeDTO) *domain.BoardTypeDef {
	cols := make([]domain.BoardTypeColumn, len(d.DefaultColumns))
	for i, c := range d.DefaultColumns {
		cols[i] = domain.BoardTypeColumn{Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit, Status: c.Status}
	}
	return &domain.BoardTypeDef{
		Type:           d.Type,
		DisplayName:    d.DisplayName,
		Icon:           d.Icon,
		DefaultColumns: cols,
		DefaultConfig:  d.DefaultConfig,
		ConfigSchema:   d.ConfigSchema,
	}
}

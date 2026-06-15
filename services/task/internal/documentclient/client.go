package documentclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/task/internal/domain"
)

// Client calls the Document Service to validate document ownership.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

func New(baseURL, serviceTokenSecret string) *Client {
	mac := hmac.New(sha256.New, []byte(serviceTokenSecret))
	mac.Write([]byte("internal"))
	token := hex.EncodeToString(mac.Sum(nil))

	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 500 * time.Millisecond},
		token:      token,
	}
}

func (c *Client) GetDocumentInfo(ctx context.Context, documentID uuid.UUID) (*domain.DocumentInfo, error) {
	url := fmt.Sprintf("%s/api/v1/internal/documents/%s", c.baseURL, documentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("document client: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("document service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrDocumentUnreachable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("document service returned %d", resp.StatusCode)
	}

	var wrapper struct {
		Data *domain.DocumentInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
}

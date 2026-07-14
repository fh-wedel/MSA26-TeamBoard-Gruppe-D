// Command mcp-server exposes TeamBoard (projects, boards, tasks, board types)
// as MCP tools, so an MCP client (Claude Desktop, Claude Code) can read and
// manage a user's TeamBoard data using a personal access token from Settings.
//
// Two transports, selected by MCP_TRANSPORT:
//
//   - "http"  (default when deployed): one shared server behind the gateway.
//     Each request carries the caller's PAT in the Authorization header, so the
//     server is multi-tenant — friends just point their client at the URL, no
//     local binary needed. This is what the Docker/Traefik deployment runs.
//   - "stdio" (default locally): a single-user process the client spawns, with
//     the PAT supplied once via TEAMBOARD_TOKEN. Handy for local development.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/services/other/mcp-server/internal/teamboard"
	"github.com/teamboard/services/other/mcp-server/internal/tools"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	apiURL := os.Getenv("TEAMBOARD_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost"
	}

	switch transport := envOr("MCP_TRANSPORT", "stdio"); transport {
	case "http":
		return runHTTP(apiURL)
	case "stdio":
		return runStdio(apiURL)
	default:
		return fmt.Errorf("unknown MCP_TRANSPORT %q (want \"http\" or \"stdio\")", transport)
	}
}

// runStdio serves one user, whose PAT is baked into the process environment.
func runStdio(apiURL string) error {
	token := os.Getenv("TEAMBOARD_TOKEN")
	if token == "" {
		return fmt.Errorf("TEAMBOARD_TOKEN is required for stdio transport — create a personal access token in TeamBoard Settings")
	}
	return newServer(apiURL, token).Run(context.Background(), &mcp.StdioTransport{})
}

// runHTTP serves many users; each request's PAT comes from its Authorization
// header, so a single deployed instance works for everyone.
func runHTTP(apiURL string) error {
	addr := envOr("MCP_HTTP_ADDR", ":8080")

	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(apiURL, bearer(r.Header.Get("Authorization")))
	}, &mcp.StreamableHTTPOptions{
		// The server sits behind Traefik on a public host; the SDK's built-in
		// DNS-rebinding/localhost guard would reject those proxied Host headers.
		// Auth is enforced by the per-request token, not by network origin.
		DisableLocalhostProtection: true,
	})

	// Act as an OAuth 2.1 resource server (MCP authorization spec): reject
	// unauthenticated requests with a WWW-Authenticate challenge so clients like
	// Claude Desktop can discover the authorization server and run the browser
	// login flow. See services/auth/internal/api/oauth.go for the AS side.
	handler := requireAuth(mcpHandler, apiURL)

	log.Printf("teamboard MCP server listening on %s (http transport, api=%s)", addr, apiURL)
	return http.ListenAndServe(addr, handler)
}

// requireAuth guards the MCP handler. It challenges unauthenticated requests
// (401 + WWW-Authenticate pointing at the protected-resource metadata) and
// validates session JWTs against the auth service. Personal access tokens
// (the Claude Code CLI path) are passed straight through and validated
// downstream by the gateway, preserving backward compatibility.
func requireAuth(next http.Handler, apiURL string) http.Handler {
	cache := &tokenCache{valid: make(map[string]time.Time)}
	httpClient := &http.Client{Timeout: 5 * time.Second}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r.Header.Get("Authorization"))
		if token == "" {
			challenge(w, r)
			return
		}
		// Opaque PATs (tbpat_) are validated by the gateway on the actual API
		// call — keep the existing CLI behaviour and skip the extra round-trip.
		if strings.HasPrefix(token, "tbpat_") {
			next.ServeHTTP(w, r)
			return
		}
		// Session JWT (OAuth-issued): validate so expired tokens produce a 401
		// and the client refreshes. Positive results are cached briefly.
		if !cache.get(token) {
			if !validateToken(httpClient, apiURL, token) {
				challenge(w, r)
				return
			}
			cache.put(token)
		}
		next.ServeHTTP(w, r)
	})
}

// challenge emits the RFC 9728 WWW-Authenticate response pointing at the
// protected-resource metadata (served by the auth service on the same host).
func challenge(w http.ResponseWriter, r *http.Request) {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
	}
	metadataURL := scheme + "://" + r.Host + "/.well-known/oauth-protected-resource"
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+metadataURL+`"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","error_description":"authorization required"}`))
}

// validateToken checks a bearer token by calling the auth service's /me endpoint
// through the gateway. 200 means the token is valid.
func validateToken(client *http.Client, apiURL, token string) bool {
	req, err := http.NewRequest(http.MethodGet, apiURL+"/api/v1/auth/me", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// tokenCache memoizes valid tokens for a short window to avoid revalidating on
// every request in a streamed MCP session.
type tokenCache struct {
	mu    sync.Mutex
	valid map[string]time.Time
}

const tokenCacheTTL = 60 * time.Second

func (c *tokenCache) get(token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp, ok := c.valid[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(c.valid, token)
		return false
	}
	return true
}

func (c *tokenCache) put(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Bound memory: clear if the map grows unexpectedly large.
	if len(c.valid) > 1000 {
		c.valid = make(map[string]time.Time)
	}
	c.valid[token] = time.Now().Add(tokenCacheTTL)
}

func newServer(apiURL, token string) *mcp.Server {
	client := teamboard.New(apiURL, token)
	server := mcp.NewServer(&mcp.Implementation{Name: "teamboard", Version: "0.1.0"}, nil)
	tools.Register(server, client)
	return server
}

func bearer(h string) string {
	return strings.TrimPrefix(h, "Bearer ")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

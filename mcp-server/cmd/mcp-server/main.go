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

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/mcp-server/internal/teamboard"
	"github.com/teamboard/mcp-server/internal/tools"
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

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(apiURL, bearer(r.Header.Get("Authorization")))
	}, &mcp.StreamableHTTPOptions{
		// The server sits behind Traefik on a public host; the SDK's built-in
		// DNS-rebinding/localhost guard would reject those proxied Host headers.
		// Auth is enforced by the per-request PAT, not by network origin.
		DisableLocalhostProtection: true,
	})

	log.Printf("teamboard MCP server listening on %s (http transport, api=%s)", addr, apiURL)
	return http.ListenAndServe(addr, handler)
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

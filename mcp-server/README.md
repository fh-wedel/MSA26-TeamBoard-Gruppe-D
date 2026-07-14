# TeamBoard MCP Server

An [MCP](https://modelcontextprotocol.io) server that lets an MCP client (Claude Desktop, Claude
Code) read and manage TeamBoard data — projects, boards, tasks, and board types — using a
[Personal Access Token](../docs/services/auth.md#17-personal-access-tokens-pat) instead of a login
session. It talks to the same gateway routes the web app uses, so it sees exactly what the token's
owner is a member of.

## Two ways to run it

### A) Deployed HTTP server (recommended — for you and friends)

The server runs as part of the Docker Compose stack (`make up-build`), behind Traefik at `/mcp`
on both `:80` and `:443`. It is **multi-tenant**: each request carries the caller's own PAT in the
`Authorization` header, so one running instance serves everyone — nobody needs to build or run a
local binary. This is what deploys to the EC2 box.

**Claude Desktop (easiest — browser sign-in, no token):** add a **custom connector** with the URL
`https://<host>/mcp`. Claude discovers the OAuth authorization server (the auth service implements
the [MCP authorization spec](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization):
protected-resource + authorization-server metadata, Dynamic Client Registration, and an
authorization-code + PKCE flow), opens a browser window where you sign in with your TeamBoard
account, and stores the issued token itself. No personal access token required.

For **Claude Code** (CLI), or any client that authenticates with a static bearer header, use a
Personal Access Token instead. Create one in **Settings → Personal access tokens**, then:

**Claude Code** (one command — the dialog gives you this pre-filled):

```bash
claude mcp add --transport http teamboard https://<host>/mcp \
  --header "Authorization: Bearer tbpat_..."
```

**Claude Desktop** — add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "teamboard": {
      "type": "http",
      "url": "https://<host>/mcp",
      "headers": { "Authorization": "Bearer tbpat_..." }
    }
  }
}
```

`<host>` is wherever the stack runs — `localhost` locally, or the EC2 IP in production.

> **Self-signed certificate:** the gateway serves HTTPS with Traefik's built-in self-signed cert
> (public-CA issuance via Let's Encrypt needs a real domain, and the box is reached by IP). Clients
> reject that by default:
> - **Claude Code:** set `{ "env": { "NODE_TLS_REJECT_UNAUTHORIZED": "0" } }` in
>   `~/.claude/settings.json` (process-wide, not per-server).
> - **Claude Desktop:** accept/trust the certificate when prompted.
>
> To get a real certificate later: point a domain at the box, add an ACME `certResolver` to
> `infra/traefik/traefik.yml`, and the self-signed workaround goes away.

### B) Local stdio binary (single user, no gateway HTTPS needed)

Runs as a process the client spawns, with your PAT supplied once via env. Good for local dev.

```bash
cd mcp-server
go build -o mcp-server.exe ./cmd/mcp-server   # Windows  (drop .exe on macOS/Linux)
```

```json
{
  "mcpServers": {
    "teamboard": {
      "command": "C:\\path\\to\\mcp-server\\mcp-server.exe",
      "env": {
        "TEAMBOARD_API_URL": "http://localhost",
        "TEAMBOARD_TOKEN": "tbpat_..."
      }
    }
  }
}
```

## Configuration (env)

| Var | Mode | Default | Meaning |
|-----|------|---------|---------|
| `MCP_TRANSPORT` | both | `stdio` | `http` (deployed, multi-tenant) or `stdio` (local, single user) |
| `TEAMBOARD_API_URL` | both | `http://localhost` | Gateway base URL (in Compose it's `http://traefik`) |
| `MCP_HTTP_ADDR` | http | `:8080` | Listen address for HTTP transport |
| `TEAMBOARD_TOKEN` | stdio | — | The PAT (http mode reads it per-request from the header instead) |

## Tools

| Tool | Description |
|------|-------------|
| `list_projects` | List all projects you're a member of |
| `list_boards` | List boards, optionally scoped to one project |
| `get_board` | Get a board's detail (columns, type) |
| `list_tasks` | List tasks on a board, optionally filtered by status/column |
| `get_task` | Get full detail for one task |
| `list_board_types` | List the board-type catalog (built-in + custom) |
| `register_board_type` | Register a new custom board type (e.g. a Gantt/timeline board) |
| `delete_board_type` | Delete a custom board type |

`register_board_type`/`delete_board_type` mirror what
[`docs/demo/presentation-board-registry.ipynb`](../docs/demo/presentation-board-registry.ipynb)
does interactively — the same `POST`/`DELETE /api/v1/board-types` endpoints, callable from an LLM.

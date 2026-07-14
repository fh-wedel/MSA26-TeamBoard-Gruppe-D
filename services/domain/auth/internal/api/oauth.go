package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/teamboard/services/domain/auth/internal/domain"
)

// OAuthHandlers implements the subset of OAuth 2.1 that the MCP authorization
// spec requires, so clients like Claude Desktop can connect with just a URL and
// a browser "Authorize" step — no manually-pasted tokens.
//
// Design notes:
//   - Public clients only (PKCE, no client secret); clients self-register via
//     Dynamic Client Registration (RFC 7591), so the user never creates one.
//   - Registered clients and short-lived authorization codes live in Redis
//     (codes carry a 60s TTL), avoiding a schema migration.
//   - The issued access token IS a standard TeamBoard session JWT, so it flows
//     through the gateway, the domain services, and the MCP server unchanged.
//     We therefore do not bind a distinct `resource` audience onto it; the MCP
//     server validates it as an ordinary TeamBoard token. This is a deliberate
//     simplification for a single-audience deployment.
type OAuthHandlers struct {
	svc   domain.AuthService
	redis *redis.Client
}

// NewOAuthHandlers constructs the OAuth authorization-server handlers.
func NewOAuthHandlers(svc domain.AuthService, rdb *redis.Client) *OAuthHandlers {
	return &OAuthHandlers{svc: svc, redis: rdb}
}

const (
	oauthCodeTTL     = 60 * time.Second
	oauthClientTTL   = 90 * 24 * time.Hour
	oauthClientKey   = "oauth:client:"
	oauthCodeKey     = "oauth:code:"
	oauthScope       = "user"
	redirectBadError = "invalid_redirect_uri"
)

type oauthClient struct {
	RedirectURIs []string `json:"redirect_uris"`
	ClientName   string   `json:"client_name"`
}

type oauthCode struct {
	ClientID      string `json:"client_id"`
	UserID        string `json:"user_id"`
	RedirectURI   string `json:"redirect_uri"`
	CodeChallenge string `json:"code_challenge"`
	Scope         string `json:"scope"`
}

// baseURL reconstructs the public origin from the proxied request. Traefik sets
// X-Forwarded-Proto and preserves the Host header, so behind the gateway this is
// e.g. https://<public-host>.
func baseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

// ── Discovery metadata ───────────────────────────────────────────────────────

// ASMetadata serves OAuth 2.0 Authorization Server Metadata (RFC 8414).
func (h *OAuthHandlers) ASMetadata(w http.ResponseWriter, r *http.Request) {
	base := baseURL(r)
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/oauth/token",
		"registration_endpoint":                 base + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{oauthScope},
	})
}

// PRMetadata serves OAuth 2.0 Protected Resource Metadata (RFC 9728) for the MCP
// endpoint. Served here (same public host) because the gateway routes all of
// /.well-known to the auth service; the MCP server's 401 challenge points here.
func (h *OAuthHandlers) PRMetadata(w http.ResponseWriter, r *http.Request) {
	base := baseURL(r)
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"resource":                 base + "/mcp",
		"authorization_servers":    []string{base},
		"scopes_supported":         []string{oauthScope},
		"bearer_methods_supported": []string{"header"},
	})
}

// ── Dynamic Client Registration (RFC 7591) ───────────────────────────────────

// Register lets a client obtain a client_id without human interaction.
func (h *OAuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURIs            []string `json:"redirect_uris"`
		ClientName              string   `json:"client_name"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "malformed registration request")
		return
	}
	if len(req.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "at least one redirect_uri is required")
		return
	}
	for _, u := range req.RedirectURIs {
		if !isAllowedRedirectURI(u) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri must be https or a loopback address")
			return
		}
	}

	clientID := "mcp_" + randomToken(16)
	client := oauthClient{RedirectURIs: req.RedirectURIs, ClientName: req.ClientName}
	if err := h.storeClient(r.Context(), clientID, client); err != nil {
		slog.ErrorContext(r.Context(), "oauth: store client failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not register client")
		return
	}

	writeOAuthJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_id_issued_at":        time.Now().Unix(),
		"redirect_uris":              req.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"client_name":                req.ClientName,
		"scope":                      oauthScope,
	})
}

// ── Authorization endpoint ───────────────────────────────────────────────────

type authorizeParams struct {
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	Resource            string
}

func parseAuthorizeParams(v url.Values) authorizeParams {
	return authorizeParams{
		ClientID:            v.Get("client_id"),
		RedirectURI:         v.Get("redirect_uri"),
		State:               v.Get("state"),
		CodeChallenge:       v.Get("code_challenge"),
		CodeChallengeMethod: v.Get("code_challenge_method"),
		Scope:               v.Get("scope"),
		Resource:            v.Get("resource"),
	}
}

// AuthorizeForm renders the login/consent page (GET /oauth/authorize).
func (h *OAuthHandlers) AuthorizeForm(w http.ResponseWriter, r *http.Request) {
	p := parseAuthorizeParams(r.URL.Query())
	client, err := h.getClient(r.Context(), p.ClientID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}
	// Never redirect to an unregistered URI (open-redirect protection).
	if !slices.Contains(client.RedirectURIs, p.RedirectURI) {
		writeOAuthError(w, http.StatusBadRequest, redirectBadError, "redirect_uri not registered for this client")
		return
	}
	if r.URL.Query().Get("response_type") != "code" {
		redirectError(w, r, p.RedirectURI, p.State, "unsupported_response_type", "only response_type=code is supported")
		return
	}
	if p.CodeChallenge == "" || p.CodeChallengeMethod != "S256" {
		redirectError(w, r, p.RedirectURI, p.State, "invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}
	renderAuthorizePage(w, p, client.ClientName, "")
}

// AuthorizeSubmit verifies credentials and issues an authorization code
// (POST /oauth/authorize).
func (h *OAuthHandlers) AuthorizeSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "malformed form")
		return
	}
	p := parseAuthorizeParams(r.PostForm)
	client, err := h.getClient(r.Context(), p.ClientID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}
	if !slices.Contains(client.RedirectURIs, p.RedirectURI) {
		writeOAuthError(w, http.StatusBadRequest, redirectBadError, "redirect_uri not registered for this client")
		return
	}
	if p.CodeChallenge == "" || p.CodeChallengeMethod != "S256" {
		redirectError(w, r, p.RedirectURI, p.State, "invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}

	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	user, err := h.svc.AuthenticateForOAuth(r.Context(), email, password)
	if err != nil {
		msg := "Invalid email or password."
		if errors.Is(err, domain.ErrRateLimited) {
			msg = "Too many attempts. Please try again later."
		}
		w.WriteHeader(http.StatusUnauthorized)
		renderAuthorizePage(w, p, client.ClientName, msg)
		return
	}

	code := randomToken(32)
	entry := oauthCode{
		ClientID:      p.ClientID,
		UserID:        user.ID.String(),
		RedirectURI:   p.RedirectURI,
		CodeChallenge: p.CodeChallenge,
		Scope:         oauthScope,
	}
	if err := h.storeCode(r.Context(), code, entry); err != nil {
		slog.ErrorContext(r.Context(), "oauth: store code failed", "error", err)
		redirectError(w, r, p.RedirectURI, p.State, "server_error", "could not issue authorization code")
		return
	}

	u, _ := url.Parse(p.RedirectURI)
	q := u.Query()
	q.Set("code", code)
	if p.State != "" {
		q.Set("state", p.State)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// ── Token endpoint ───────────────────────────────────────────────────────────

// Token exchanges an authorization code (with PKCE) or a refresh token for an
// access token pair (POST /oauth/token).
func (h *OAuthHandlers) Token(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "malformed form")
		return
	}

	switch r.PostFormValue("grant_type") {
	case "authorization_code":
		h.tokenAuthCode(w, r)
	case "refresh_token":
		h.tokenRefresh(w, r)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

func (h *OAuthHandlers) tokenAuthCode(w http.ResponseWriter, r *http.Request) {
	code := r.PostFormValue("code")
	entry, err := h.consumeCode(r.Context(), code)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid or expired")
		return
	}
	if entry.ClientID != r.PostFormValue("client_id") || entry.RedirectURI != r.PostFormValue("redirect_uri") {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "client_id or redirect_uri mismatch")
		return
	}
	if !verifyPKCE(r.PostFormValue("code_verifier"), entry.CodeChallenge) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	userID, err := uuid.Parse(entry.UserID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid subject")
		return
	}
	pair, err := h.svc.IssueTokensForUser(r.Context(), userID)
	if err != nil {
		slog.ErrorContext(r.Context(), "oauth: issue tokens failed", "error", err)
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "could not issue token")
		return
	}
	writeTokenResponse(w, pair)
}

func (h *OAuthHandlers) tokenRefresh(w http.ResponseWriter, r *http.Request) {
	pair, err := h.svc.Refresh(r.Context(), r.PostFormValue("refresh_token"))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid or expired")
		return
	}
	writeTokenResponse(w, pair)
}

// ── Redis-backed storage ─────────────────────────────────────────────────────

func (h *OAuthHandlers) storeClient(ctx context.Context, id string, c oauthClient) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return h.redis.Set(ctx, oauthClientKey+id, b, oauthClientTTL).Err()
}

func (h *OAuthHandlers) getClient(ctx context.Context, id string) (*oauthClient, error) {
	if id == "" {
		return nil, errors.New("empty client_id")
	}
	b, err := h.redis.Get(ctx, oauthClientKey+id).Bytes()
	if err != nil {
		return nil, err
	}
	var c oauthClient
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (h *OAuthHandlers) storeCode(ctx context.Context, code string, c oauthCode) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return h.redis.Set(ctx, oauthCodeKey+code, b, oauthCodeTTL).Err()
}

// consumeCode atomically fetches and deletes an authorization code (one-time use).
func (h *OAuthHandlers) consumeCode(ctx context.Context, code string) (*oauthCode, error) {
	if code == "" {
		return nil, errors.New("empty code")
	}
	b, err := h.redis.GetDel(ctx, oauthCodeKey+code).Bytes()
	if err != nil {
		return nil, err
	}
	var c oauthCode
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ── Small helpers ────────────────────────────────────────────────────────────

func writeOAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Discovery/token endpoints may be fetched from a web context by some clients.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	writeOAuthJSON(w, status, map[string]any{"error": code, "error_description": desc})
}

func writeTokenResponse(w http.ResponseWriter, pair *domain.TokenPair) {
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"access_token":  pair.AccessToken,
		"token_type":    "Bearer",
		"expires_in":    pair.ExpiresIn,
		"refresh_token": pair.RefreshToken,
		"scope":         oauthScope,
	})
}

// redirectError sends an OAuth error back to a validated redirect_uri.
func redirectError(w http.ResponseWriter, r *http.Request, redirectURI, state, code, desc string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, code, desc)
		return
	}
	q := u.Query()
	q.Set("error", code)
	q.Set("error_description", desc)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// isAllowedRedirectURI enforces the OAuth 2.1 rule: redirect URIs must use
// https, or be loopback (http://localhost / http://127.0.0.1) for native apps.
func isAllowedRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	// Custom app schemes (e.g. claude://) are also acceptable for native clients.
	return u.Scheme != "" && u.Opaque == "" && !strings.HasPrefix(u.Scheme, "http")
}

// verifyPKCE checks base64url(SHA256(verifier)) == challenge (S256).
func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
}

func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return hex.EncodeToString(b)
}

// authorizePageData feeds the consent template.
type authorizePageData struct {
	ClientName          string
	Error               string
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	Resource            string
}

func renderAuthorizePage(w http.ResponseWriter, p authorizeParams, clientName, errMsg string) {
	name := clientName
	if name == "" {
		name = "An application"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = authorizeTmpl.Execute(w, authorizePageData{
		ClientName:          name,
		Error:               errMsg,
		ClientID:            p.ClientID,
		RedirectURI:         p.RedirectURI,
		State:               p.State,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
		Scope:               p.Scope,
		Resource:            p.Resource,
	})
}

// authorizeTmpl is the browser login+consent page. html/template auto-escapes
// every field, so the hidden OAuth parameters cannot be used for injection.
var authorizeTmpl = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Authorize · TeamBoard</title>
<style>
  :root { color-scheme: light dark; }
  * { box-sizing: border-box; }
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center;
         font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif; background:#0f172a; color:#e2e8f0; }
  .card { width:100%; max-width:380px; margin:1rem; padding:2rem; background:#1e293b; border-radius:14px;
          box-shadow:0 10px 40px rgba(0,0,0,.4); }
  h1 { font-size:1.15rem; margin:0 0 .25rem; }
  p.sub { margin:0 0 1.5rem; color:#94a3b8; font-size:.9rem; line-height:1.4; }
  strong { color:#e2e8f0; }
  label { display:block; font-size:.8rem; color:#94a3b8; margin:.9rem 0 .3rem; }
  input { width:100%; padding:.6rem .7rem; border-radius:8px; border:1px solid #334155;
          background:#0f172a; color:#e2e8f0; font-size:.95rem; }
  input:focus { outline:2px solid #6366f1; border-color:transparent; }
  button { width:100%; margin-top:1.4rem; padding:.7rem; border:0; border-radius:8px; cursor:pointer;
           background:#6366f1; color:#fff; font-size:.95rem; font-weight:600; }
  button:hover { background:#4f46e5; }
  .err { margin-top:1rem; padding:.6rem .7rem; border-radius:8px; background:#7f1d1d; color:#fecaca; font-size:.85rem; }
  .foot { margin-top:1.2rem; font-size:.75rem; color:#64748b; text-align:center; }
</style>
</head>
<body>
  <form class="card" method="post" action="/oauth/authorize">
    <h1>Authorize {{.ClientName}}</h1>
    <p class="sub"><strong>{{.ClientName}}</strong> wants to access your TeamBoard account and act on your behalf. Sign in to allow it.</p>
    {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
    <label for="email">Email</label>
    <input id="email" name="email" type="email" autocomplete="username" required autofocus>
    <label for="password">Password</label>
    <input id="password" name="password" type="password" autocomplete="current-password" required>
    <input type="hidden" name="client_id" value="{{.ClientID}}">
    <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
    <input type="hidden" name="state" value="{{.State}}">
    <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
    <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
    <input type="hidden" name="scope" value="{{.Scope}}">
    <input type="hidden" name="resource" value="{{.Resource}}">
    <button type="submit">Sign in &amp; Authorize</button>
    <div class="foot">You are authorizing access to TeamBoard.</div>
  </form>
</body>
</html>`))

package authmiddleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/teamboard/shared/go/httputil"
)

// patPrefix identifies a bearer token as an opaque personal access token
// (issued by the auth service) rather than a self-contained RS256 JWT, so
// Middleware can route to introspection without attempting a JWT parse.
const patPrefix = "tbpat_"

// TokenIntrospector resolves an opaque personal access token to its owning
// user. Implemented per-service against the auth service's internal
// introspection endpoint (typically with a short-TTL cache) — see
// services/project/internal/authclient and services/task/internal/authclient.
type TokenIntrospector interface {
	Introspect(ctx context.Context, token string) (userID uuid.UUID, email string, err error)
}

// Option configures the JWT middleware.
type Option func(*middlewareConfig)

type middlewareConfig struct {
	issuer       string
	audience     string
	clockSkew    time.Duration
	introspector TokenIntrospector
}

// WithIssuer enforces a specific JWT issuer.
func WithIssuer(iss string) Option {
	return func(c *middlewareConfig) { c.issuer = iss }
}

// WithAudience enforces a specific JWT audience.
func WithAudience(aud string) Option {
	return func(c *middlewareConfig) { c.audience = aud }
}

// WithClockSkew sets tolerance for clock differences. Default: 30s.
func WithClockSkew(skew time.Duration) Option {
	return func(c *middlewareConfig) { c.clockSkew = skew }
}

// WithPATIntrospector enables acceptance of personal access tokens (prefixed
// "tbpat_") alongside session JWTs. Without this option, Middleware only
// ever accepts RS256 JWTs — fully backward-compatible for services that
// don't opt in.
func WithPATIntrospector(introspector TokenIntrospector) Option {
	return func(c *middlewareConfig) { c.introspector = introspector }
}

// Middleware returns an HTTP middleware that validates Bearer JWTs and injects user info into context.
func Middleware(jwks JWKSSource, opts ...Option) func(http.Handler) http.Handler {
	cfg := &middlewareConfig{clockSkew: 30 * time.Second}
	for _, o := range opts {
		o(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearer(r.Header.Get("Authorization"))
			if tokenStr == "" {
				httputil.WriteProblem(w, r, http.StatusUnauthorized, "missing authorization header")
				return
			}

			if cfg.introspector != nil && strings.HasPrefix(tokenStr, patPrefix) {
				uid, email, err := cfg.introspector.Introspect(r.Context(), tokenStr)
				if err != nil {
					httputil.WriteProblem(w, r, http.StatusUnauthorized, "invalid or expired token")
					return
				}
				ctx := context.WithValue(r.Context(), contextKeyUserID, uid)
				if email != "" {
					ctx = context.WithValue(ctx, contextKeyEmail, email)
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			claims, err := parseToken(r.Context(), tokenStr, jwks, cfg)
			if err != nil {
				httputil.WriteProblem(w, r, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			sub, err := claims.GetSubject()
			if err != nil {
				httputil.WriteProblem(w, r, http.StatusUnauthorized, "missing subject claim")
				return
			}

			uid, err := uuid.Parse(sub)
			if err != nil {
				httputil.WriteProblem(w, r, http.StatusUnauthorized, "invalid user id in token")
				return
			}

			ctx := context.WithValue(r.Context(), contextKeyUserID, uid)
			if email, ok := claims["email"].(string); ok && email != "" {
				ctx = context.WithValue(ctx, contextKeyEmail, email)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// VerifyToken validates a raw JWT string against the JWKS and returns its claims.
// It enforces RS256 and expiry, plus issuer/audience when configured via opts
// (WithIssuer/WithAudience/WithClockSkew). Use it for non-HTTP token paths such
// as WebSocket upgrades where the token arrives in a query parameter rather than
// an Authorization header.
func VerifyToken(ctx context.Context, tokenStr string, jwks JWKSSource, opts ...Option) (jwt.MapClaims, error) {
	cfg := &middlewareConfig{clockSkew: 30 * time.Second}
	for _, o := range opts {
		o(cfg)
	}
	return parseToken(ctx, tokenStr, jwks, cfg)
}

func extractBearer(h string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimPrefix(h, prefix)
}

func parseToken(ctx context.Context, tokenStr string, jwks JWKSSource, cfg *middlewareConfig) (jwt.MapClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		kid, _ := t.Header["kid"].(string)
		return jwks.Key(ctx, kid)
	},
		jwt.WithLeeway(cfg.clockSkew),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	if cfg.issuer != "" {
		iss, _ := claims.GetIssuer()
		if iss != cfg.issuer {
			return nil, jwt.ErrTokenInvalidIssuer
		}
	}
	if cfg.audience != "" {
		aud, _ := claims.GetAudience()
		found := false
		for _, a := range aud {
			if a == cfg.audience {
				found = true
				break
			}
		}
		if !found {
			return nil, jwt.ErrTokenInvalidAudience
		}
	}

	return claims, nil
}

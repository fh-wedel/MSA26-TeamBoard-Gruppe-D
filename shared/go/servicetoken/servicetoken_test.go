package servicetoken

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
)

const testSecret = "test-shared-service-secret"

func TestIssueAndVerify_RoundTrip(t *testing.T) {
	iss := NewIssuer(testSecret)
	ver := NewVerifier(testSecret, "internal")

	token, err := iss.Issue(context.Background(), "task-service", "internal")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	claims, err := ver.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Service != "task-service" {
		t.Errorf("service = %q, want task-service", claims.Service)
	}
	if claims.Audience != "internal" {
		t.Errorf("audience = %q, want internal", claims.Audience)
	}
}

func TestVerify_RejectsWrongAudience(t *testing.T) {
	iss := NewIssuer(testSecret)
	ver := NewVerifier(testSecret, "internal")

	// Issue for a different audience than the verifier accepts.
	token, err := iss.Issue(context.Background(), "task-service", "other-audience")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ver.Verify(context.Background(), token); err == nil {
		t.Fatal("expected verification to fail for mismatched audience")
	}
}

func TestVerify_RejectsWrongSecret(t *testing.T) {
	iss := NewIssuer(testSecret)
	ver := NewVerifier("a-different-secret", "internal")

	token, err := iss.Issue(context.Background(), "task-service", "internal")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ver.Verify(context.Background(), token); err == nil {
		t.Fatal("expected verification to fail for token signed with a different secret")
	}
}

func TestVerify_RejectsExpiredToken(t *testing.T) {
	ver := NewVerifier(testSecret, "internal")

	// Hand-build a token that expired in the past, signed with the right secret.
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    "teamboard-internal",
		Subject:   "task-service",
		Audience:  jwt.ClaimStrings{"internal"},
		IssuedAt:  jwt.NewNumericDate(now.Add(-10 * time.Minute)),
		ExpiresAt: jwt.NewNumericDate(now.Add(-5 * time.Minute)),
		ID:        ulid.Make().String(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := ver.Verify(context.Background(), signed); err == nil {
		t.Fatal("expected verification to fail for an expired token")
	}
}

func TestVerify_RejectsWrongIssuer(t *testing.T) {
	ver := NewVerifier(testSecret, "internal")

	claims := jwt.RegisteredClaims{
		Issuer:    "someone-else",
		Subject:   "task-service",
		Audience:  jwt.ClaimStrings{"internal"},
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		ID:        ulid.Make().String(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := ver.Verify(context.Background(), signed); err == nil {
		t.Fatal("expected verification to fail for a wrong issuer")
	}
}

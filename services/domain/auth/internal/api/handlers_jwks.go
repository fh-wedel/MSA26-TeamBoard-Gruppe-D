package api

import (
	"encoding/json"
	"net/http"

	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/teamboard/services/domain/auth/internal/domain"
)

// JWKS handles GET /.well-known/jwks.json — returns the public signing keys.
func (h *Handlers) JWKS(w http.ResponseWriter, r *http.Request) {
	keys, err := h.repo.ListValidatingSigningKeys(r.Context())
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "internal_error", "failed to list signing keys")
		return
	}

	set := jwk.NewSet()
	for _, sk := range keys {
		pub, err := domain.ParseRSAPublicKey([]byte(sk.PublicKeyPEM))
		if err != nil {
			continue
		}
		jwkKey, err := jwk.FromRaw(pub)
		if err != nil {
			continue
		}
		_ = jwkKey.Set(jwk.KeyIDKey, sk.KID)
		_ = jwkKey.Set(jwk.AlgorithmKey, sk.Algorithm)
		_ = jwkKey.Set(jwk.KeyUsageKey, "sig")
		_ = set.AddKey(jwkKey)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(set)
}

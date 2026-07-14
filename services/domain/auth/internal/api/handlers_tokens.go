package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/teamboard/services/domain/auth/internal/domain"
)

// allowedPATExpiryDays mirrors the fixed set of TTL choices offered in Settings.
var allowedPATExpiryDays = map[int]bool{30: true, 90: true, 365: true}

// CreatePAT handles POST /auth/tokens (UC-8).
func (h *Handlers) CreatePAT(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authentication context")
		return
	}

	var req createPATRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	if !allowedPATExpiryDays[req.ExpiresInDays] {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "expires_in_days must be one of 30, 90, 365")
		return
	}

	pat, rawToken, err := h.svc.CreatePAT(r.Context(), userID, req.Name, time.Duration(req.ExpiresInDays)*24*time.Hour)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, patCreatedResponse{
		patResponse: mapPATResponse(pat),
		Token:       rawToken,
	})
}

// ListPATs handles GET /auth/tokens (UC-9).
func (h *Handlers) ListPATs(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authentication context")
		return
	}

	pats, err := h.svc.ListPATs(r.Context(), userID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	resp := make([]patResponse, len(pats))
	for i, pat := range pats {
		resp[i] = mapPATResponse(pat)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

// RevokePAT handles DELETE /auth/tokens/{id} (UC-10).
func (h *Handlers) RevokePAT(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authentication context")
		return
	}

	patID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid token id")
		return
	}

	if err := h.svc.RevokePAT(r.Context(), userID, patID); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// IntrospectPAT handles POST /internal/tokens/introspect (UC-11). Called by
// other services to resolve a personal access token to its owning user.
// Protected by servicetoken.RequireServiceToken, not requireAuth.
func (h *Handlers) IntrospectPAT(w http.ResponseWriter, r *http.Request) {
	var req introspectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	user, err := h.svc.IntrospectPAT(r.Context(), req.Token)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, introspectResponse{
		UserID: user.ID.String(),
		Email:  user.Email,
	})
}

func mapPATResponse(p *domain.PersonalAccessToken) patResponse {
	return patResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		TokenPrefix: p.TokenPrefix,
		CreatedAt:   p.CreatedAt,
		ExpiresAt:   p.ExpiresAt,
		LastUsedAt:  p.LastUsedAt,
		RevokedAt:   p.RevokedAt,
	}
}

package api

import (
	"encoding/json"
	"net/http"

	"github.com/teamboard/services/auth/internal/domain"
)

// Register handles POST /auth/register (UC-1).
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	user, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, mapUserResponse(user))
}

// Login handles POST /auth/login (UC-2).
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	ip := realIP(r)
	ua := r.UserAgent()

	pair, err := h.svc.Login(r.Context(), req.Email, req.Password, ip, ua)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, mapTokenPairResponse(pair))
}

// Refresh handles POST /auth/refresh (UC-3).
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	pair, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, mapTokenPairResponse(pair))
}

// Logout handles POST /auth/logout (UC-4).
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		writeDomainError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RequestPasswordReset handles POST /auth/password-reset/request (UC-5).
func (h *Handlers) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req requestPasswordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Always return 202 — don't reveal whether the email exists.
	_ = h.svc.RequestPasswordReset(r.Context(), req.Email)
	w.WriteHeader(http.StatusAccepted)
}

// ConfirmPasswordReset handles POST /auth/password-reset/confirm (UC-6).
func (h *Handlers) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req confirmPasswordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if err := h.svc.ConfirmPasswordReset(r.Context(), req.Token, req.NewPassword); err != nil {
		writeDomainError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetUser handles GET /auth/me (UC-7). Requires a valid access token.
func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authentication context")
		return
	}

	user, err := h.svc.GetUser(r.Context(), userID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, mapUserResponse(user))
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func mapUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:          u.ID.String(),
		Email:       u.Email,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}

func mapTokenPairResponse(p *domain.TokenPair) tokenPairResponse {
	return tokenPairResponse{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		ExpiresIn:    p.ExpiresIn,
		TokenType:    p.TokenType,
	}
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/domain/project/internal/domain"
)

// ── Request / Response DTOs ───────────────────────────────────────────────────

type createInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type invitationResponse struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"project_id"`
	InviteeEmail string  `json:"invitee_email"`
	Role         string  `json:"role"`
	Token        string  `json:"token"`
	InvitedBy    string  `json:"invited_by"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	ExpiresAt    string  `json:"expires_at"`
}

func mapInvitationResponse(inv *domain.Invitation) invitationResponse {
	return invitationResponse{
		ID:           inv.ID.String(),
		ProjectID:    inv.ProjectID.String(),
		InviteeEmail: inv.InviteeEmail,
		Role:         string(inv.Role),
		Token:        inv.Token,
		InvitedBy:    inv.InvitedBy.String(),
		Status:       string(inv.Status),
		CreatedAt:    inv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:    inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// POST /api/v1/projects/{projectID}/invitations
func (h *Handlers) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation_failed", "invalid project ID")
		return
	}
	requester, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	var req createInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON")
		return
	}

	inv, err := h.svc.CreateInvitation(r.Context(), projectID, requester, req.Email, domain.Role(req.Role))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapInvitationResponse(inv))
}

// GET /api/v1/projects/{projectID}/invitations
func (h *Handlers) ListInvitations(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation_failed", "invalid project ID")
		return
	}
	requester, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	invitations, err := h.svc.ListInvitations(r.Context(), projectID, requester)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	resp := make([]invitationResponse, len(invitations))
	for i, inv := range invitations {
		resp[i] = mapInvitationResponse(inv)
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/v1/invitations/{token}
func (h *Handlers) GetInvitation(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, err := h.svc.GetInvitationByToken(r.Context(), token)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapInvitationResponse(inv))
}

// POST /api/v1/invitations/{token}:accept
func (h *Handlers) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	email, _ := emailFromContext(r.Context())

	member, err := h.svc.AcceptInvitation(r.Context(), token, userID, email)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapMemberResponse(member))
}

// POST /api/v1/invitations/{token}:decline
func (h *Handlers) DeclineInvitation(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	email, _ := emailFromContext(r.Context())

	if err := h.svc.DeclineInvitation(r.Context(), token, userID, email); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

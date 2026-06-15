package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamboard/services/project/internal/domain"
	"github.com/teamboard/services/project/internal/repository/db"
)

type postgresRepo struct {
	pool *pgxpool.Pool
	q    db.Querier
}

func New(pool *pgxpool.Pool) domain.Repository {
	return &postgresRepo{pool: pool, q: db.New(pool)}
}

// WithTransaction runs fn inside a pgx transaction.
func (r *postgresRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	txRepo := &postgresRepo{pool: r.pool, q: db.New(tx)}
	if err := fn(ctx, txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateProject(ctx context.Context, id, ownerID uuid.UUID, name, description string) (*domain.Project, error) {
	p, err := r.q.CreateProject(ctx, id, ownerID, name, description)
	if err != nil {
		return nil, err
	}
	return mapProject(p), nil
}

func (r *postgresRepo) GetProject(ctx context.Context, id uuid.UUID) (*domain.Project, error) {
	p, err := r.q.GetProject(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProjectNotFound
		}
		return nil, err
	}
	return mapProject(p), nil
}

func (r *postgresRepo) ListProjectsForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Project, error) {
	rows, err := r.q.ListProjectsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Project, len(rows))
	for i, p := range rows {
		out[i] = mapProject(p)
	}
	return out, nil
}

func (r *postgresRepo) UpdateProject(ctx context.Context, id uuid.UUID, name, description *string) (*domain.Project, error) {
	p, err := r.q.UpdateProject(ctx, id, name, description)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProjectNotFound
		}
		return nil, err
	}
	return mapProject(p), nil
}

func (r *postgresRepo) SoftDeleteProject(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SoftDeleteProject(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrProjectNotFound
		}
		return err
	}
	return nil
}

// ── Members ───────────────────────────────────────────────────────────────────

func (r *postgresRepo) AddMember(ctx context.Context, projectID, userID uuid.UUID, role domain.Role, invitedBy *uuid.UUID) (*domain.ProjectMember, error) {
	m, err := r.q.AddMember(ctx, projectID, userID, string(role), invitedBy)
	if err != nil {
		return nil, err
	}
	return mapMember(m, nil), nil
}

func (r *postgresRepo) GetMember(ctx context.Context, projectID, userID uuid.UUID) (*domain.ProjectMember, error) {
	m, err := r.q.GetMember(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMemberNotFound
		}
		return nil, err
	}
	return mapMember(m, nil), nil
}

func (r *postgresRepo) ListMembers(ctx context.Context, projectID uuid.UUID) ([]*domain.ProjectMember, error) {
	rows, err := r.q.ListMembers(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.ProjectMember, len(rows))
	for i, m := range rows {
		out[i] = mapMemberWithEmail(m)
	}
	return out, nil
}

func (r *postgresRepo) UpdateMemberRole(ctx context.Context, projectID, userID uuid.UUID, role domain.Role) (*domain.ProjectMember, error) {
	m, err := r.q.UpdateMemberRole(ctx, projectID, userID, string(role))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMemberNotFound
		}
		return nil, err
	}
	return mapMember(m, nil), nil
}

func (r *postgresRepo) RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error {
	return r.q.RemoveMember(ctx, projectID, userID)
}

func (r *postgresRepo) CountOwners(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return r.q.CountOwners(ctx, projectID)
}

func (r *postgresRepo) GetMemberRole(ctx context.Context, projectID, userID uuid.UUID) (domain.Role, error) {
	role, err := r.q.GetMemberRole(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrMemberNotFound
		}
		return "", err
	}
	return domain.Role(role), nil
}

func (r *postgresRepo) RemoveAllMembershipsOfUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return r.q.RemoveAllMembershipsOfUser(ctx, userID)
}

// ── Boards ────────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateBoard(ctx context.Context, id, projectID uuid.UUID, name string, bType domain.BoardType, position int, config []byte, createdBy uuid.UUID) (*domain.Board, error) {
	b, err := r.q.CreateBoard(ctx, id, projectID, name, string(bType), position, config, createdBy)
	if err != nil {
		return nil, err
	}
	return mapBoard(b), nil
}

func (r *postgresRepo) GetBoard(ctx context.Context, id uuid.UUID) (*domain.Board, error) {
	b, err := r.q.GetBoard(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrBoardNotFound
		}
		return nil, err
	}
	return mapBoard(b), nil
}

func (r *postgresRepo) ListBoardsByProject(ctx context.Context, projectID uuid.UUID) ([]*domain.Board, error) {
	rows, err := r.q.ListBoardsByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Board, len(rows))
	for i, b := range rows {
		out[i] = mapBoard(b)
	}
	return out, nil
}

func (r *postgresRepo) UpdateBoard(ctx context.Context, id uuid.UUID, name *string, config []byte) (*domain.Board, error) {
	b, err := r.q.UpdateBoard(ctx, id, name, config)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrBoardNotFound
		}
		return nil, err
	}
	return mapBoard(b), nil
}

func (r *postgresRepo) SoftDeleteBoard(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SoftDeleteBoard(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrBoardNotFound
		}
		return err
	}
	return nil
}

func (r *postgresRepo) GetMaxBoardPosition(ctx context.Context, projectID uuid.UUID) (int, error) {
	return r.q.GetMaxBoardPosition(ctx, projectID)
}

// ── Columns ───────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int, wipLimit *int) (*domain.BoardColumn, error) {
	c, err := r.q.CreateColumn(ctx, id, boardID, name, position, wipLimit)
	if err != nil {
		return nil, err
	}
	return mapColumn(c), nil
}

func (r *postgresRepo) ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]domain.BoardColumn, error) {
	cols, err := r.q.ListColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.BoardColumn, len(cols))
	for i, c := range cols {
		out[i] = *mapColumn(&c)
	}
	return out, nil
}

// ── Known users ───────────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertKnownUser(ctx context.Context, id uuid.UUID, email string, createdAt time.Time) error {
	return r.q.UpsertKnownUser(ctx, id, email, createdAt)
}

func (r *postgresRepo) GetKnownUserByEmail(ctx context.Context, email string) (*domain.KnownUser, error) {
	u, err := r.q.GetKnownUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUnknownUser
		}
		return nil, err
	}
	return mapKnownUser(u), nil
}

func (r *postgresRepo) GetKnownUserByID(ctx context.Context, id uuid.UUID) (*domain.KnownUser, error) {
	u, err := r.q.GetKnownUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUnknownUser
		}
		return nil, err
	}
	return mapKnownUser(u), nil
}

func (r *postgresRepo) MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkKnownUserDeleted(ctx, id)
}

// ── Outbox ────────────────────────────────────────────────────────────────────

func (r *postgresRepo) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	return r.q.InsertOutboxEvent(ctx, id, aggregateID, eventType, payload)
}

func (r *postgresRepo) GetUnpublishedEvents(ctx context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	rows, err := r.q.GetUnpublishedEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.OutboxEvent, len(rows))
	for i, e := range rows {
		out[i] = &domain.OutboxEvent{
			ID:          e.ID,
			AggregateID: e.AggregateID,
			EventType:   e.EventType,
			Payload:     e.Payload,
			OccurredAt:  e.OccurredAt,
			PublishedAt: e.PublishedAt,
		}
	}
	return out, nil
}

func (r *postgresRepo) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkEventPublished(ctx, id)
}

// ── Processed events ──────────────────────────────────────────────────────────

func (r *postgresRepo) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	return r.q.WasEventProcessed(ctx, eventID)
}

func (r *postgresRepo) MarkEventProcessed(ctx context.Context, eventID string) error {
	return r.q.MarkEventProcessed(ctx, eventID)
}

// ── Mapping helpers ───────────────────────────────────────────────────────────

func mapProject(p *db.Project) *domain.Project {
	return &domain.Project{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		OwnerID:     p.OwnerID,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		DeletedAt:   p.DeletedAt,
	}
}

func mapMember(m *db.ProjectMember, email *string) *domain.ProjectMember {
	pm := &domain.ProjectMember{
		ProjectID: m.ProjectID,
		UserID:    m.UserID,
		Role:      domain.Role(m.Role),
		InvitedBy: m.InvitedBy,
		JoinedAt:  m.JoinedAt,
	}
	if email != nil {
		pm.Email = *email
	}
	return pm
}

func mapMemberWithEmail(m *db.ProjectMemberWithEmail) *domain.ProjectMember {
	pm := &domain.ProjectMember{
		ProjectID: m.ProjectID,
		UserID:    m.UserID,
		Role:      domain.Role(m.Role),
		InvitedBy: m.InvitedBy,
		JoinedAt:  m.JoinedAt,
	}
	if m.Email != nil {
		pm.Email = *m.Email
	}
	return pm
}

func mapBoard(b *db.Board) *domain.Board {
	board := &domain.Board{
		ID:        b.ID,
		ProjectID: b.ProjectID,
		Name:      b.Name,
		Type:      domain.BoardType(b.Type),
		Position:  b.Position,
		CreatedBy: b.CreatedBy,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
		DeletedAt: b.DeletedAt,
	}
	if len(b.Config) > 0 {
		_ = unmarshalJSON(b.Config, &board.Config)
	}
	if board.Config == nil {
		board.Config = map[string]any{}
	}
	return board
}

func mapColumn(c *db.BoardColumn) *domain.BoardColumn {
	return &domain.BoardColumn{
		ID:       c.ID,
		BoardID:  c.BoardID,
		Name:     c.Name,
		Position: c.Position,
		WIPLimit: c.WIPLimit,
	}
}

func mapKnownUser(u *db.KnownUser) *domain.KnownUser {
	return &domain.KnownUser{
		ID:        u.ID,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
		DeletedAt: u.DeletedAt,
	}
}

// ── Invitation repository methods ─────────────────────────────────────────────

const sqlCreateInvitation = `
INSERT INTO invitations (id, project_id, invitee_email, role, token, invited_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, project_id, invitee_email, role, token, invited_by, status, created_at, expires_at, responded_at`

func (r *postgresRepo) CreateInvitation(ctx context.Context, id, projectID uuid.UUID, email, role, token string, invitedBy uuid.UUID) (*domain.Invitation, error) {
	row := r.pool.QueryRow(ctx, sqlCreateInvitation, id, projectID, email, role, token, invitedBy)
	return scanInvitation(row)
}

const sqlGetInvitationByToken = `
SELECT id, project_id, invitee_email, role, token, invited_by, status, created_at, expires_at, responded_at
FROM invitations WHERE token = $1`

func (r *postgresRepo) GetInvitationByToken(ctx context.Context, token string) (*domain.Invitation, error) {
	row := r.pool.QueryRow(ctx, sqlGetInvitationByToken, token)
	inv, err := scanInvitation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInvitationNotFound
	}
	return inv, err
}

const sqlGetPendingByEmailAndProject = `
SELECT id, project_id, invitee_email, role, token, invited_by, status, created_at, expires_at, responded_at
FROM invitations WHERE invitee_email = $1 AND project_id = $2 AND status = 'pending' LIMIT 1`

func (r *postgresRepo) GetPendingInvitationByEmailAndProject(ctx context.Context, email string, projectID uuid.UUID) (*domain.Invitation, error) {
	row := r.pool.QueryRow(ctx, sqlGetPendingByEmailAndProject, email, projectID)
	inv, err := scanInvitation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return inv, err
}

const sqlGetInvitationsByProject = `
SELECT id, project_id, invitee_email, role, token, invited_by, status, created_at, expires_at, responded_at
FROM invitations WHERE project_id = $1 ORDER BY created_at DESC`

func (r *postgresRepo) GetInvitationsByProject(ctx context.Context, projectID uuid.UUID) ([]*domain.Invitation, error) {
	rows, err := r.pool.Query(ctx, sqlGetInvitationsByProject, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.Invitation
	for rows.Next() {
		inv, err := scanInvitationRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, inv)
	}
	return result, rows.Err()
}

const sqlUpdateInvitationStatus = `
UPDATE invitations SET status = $2, responded_at = now() WHERE id = $1`

func (r *postgresRepo) UpdateInvitationStatus(ctx context.Context, id uuid.UUID, status domain.InvitationStatus) error {
	_, err := r.pool.Exec(ctx, sqlUpdateInvitationStatus, id, string(status))
	return err
}

type invitationScanner interface {
	Scan(dest ...any) error
}

func scanInvitation(s invitationScanner) (*domain.Invitation, error) {
	var inv domain.Invitation
	var status string
	var role string
	if err := s.Scan(&inv.ID, &inv.ProjectID, &inv.InviteeEmail, &role, &inv.Token,
		&inv.InvitedBy, &status, &inv.CreatedAt, &inv.ExpiresAt, &inv.RespondedAt); err != nil {
		return nil, err
	}
	inv.Status = domain.InvitationStatus(status)
	inv.Role = domain.Role(role)
	return &inv, nil
}

func scanInvitationRow(rows pgx.Rows) (*domain.Invitation, error) {
	var inv domain.Invitation
	var status, role string
	if err := rows.Scan(&inv.ID, &inv.ProjectID, &inv.InviteeEmail, &role, &inv.Token,
		&inv.InvitedBy, &status, &inv.CreatedAt, &inv.ExpiresAt, &inv.RespondedAt); err != nil {
		return nil, err
	}
	inv.Status = domain.InvitationStatus(status)
	inv.Role = domain.Role(role)
	return &inv, nil
}

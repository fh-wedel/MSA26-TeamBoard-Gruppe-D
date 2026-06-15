package domain

import (
	"context"

	"github.com/google/uuid"
)

// BoardTypeService is the use-case interface for managing board-type definitions.
type BoardTypeService interface {
	Register(ctx context.Context, in RegisterInput) (*BoardTypeDef, error)
	Update(ctx context.Context, typ string, patch UpdatePatch) (*BoardTypeDef, error)
	Delete(ctx context.Context, typ string) error
	Get(ctx context.Context, typ string) (*BoardTypeDef, error)
	List(ctx context.Context) ([]*BoardTypeDef, error)
}

// Repository is the persistence port (implemented in repository/).
type Repository interface {
	CreateBoardType(ctx context.Context, def *BoardTypeDef) (*BoardTypeDef, error)
	GetBoardType(ctx context.Context, typ string) (*BoardTypeDef, error)
	ListBoardTypes(ctx context.Context) ([]*BoardTypeDef, error)
	UpdateBoardType(ctx context.Context, def *BoardTypeDef) (*BoardTypeDef, error)
	DeleteBoardType(ctx context.Context, typ string) error

	// Outbox
	InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error
	GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error)
	MarkEventPublished(ctx context.Context, id uuid.UUID) error

	WithTransaction(ctx context.Context, fn func(context.Context, Repository) error) error
}

package domain

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

type service struct {
	repo Repository
}

// NewService constructs the board-type domain service.
func NewService(repo Repository) BoardTypeService {
	return &service{repo: repo}
}

var _ BoardTypeService = (*service)(nil)

func (s *service) Register(ctx context.Context, in RegisterInput) (*BoardTypeDef, error) {
	if err := validateDefinition(in.Type, in.DisplayName, in.DefaultColumns, in.DefaultConfig, in.ConfigSchema, in.Presentation); err != nil {
		return nil, err
	}
	if existing, _ := s.repo.GetBoardType(ctx, in.Type); existing != nil {
		return nil, ErrAlreadyExists
	}

	def := &BoardTypeDef{
		ID:             uuid.New(),
		Type:           in.Type,
		DisplayName:    in.DisplayName,
		Icon:           in.Icon,
		DefaultColumns: in.DefaultColumns,
		DefaultConfig:  orEmpty(in.DefaultConfig),
		ConfigSchema:   orEmpty(in.ConfigSchema),
		Presentation:   orEmpty(in.Presentation),
		BuiltIn:        false,
		CreatedBy:      in.CreatedBy,
	}

	var out *BoardTypeDef
	err := s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var e error
		if out, e = tx.CreateBoardType(ctx, def); e != nil {
			return e
		}
		return insertEvent(ctx, tx, out, "boardtype.registered")
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Update(ctx context.Context, typ string, patch UpdatePatch) (*BoardTypeDef, error) {
	existing, err := s.repo.GetBoardType(ctx, typ)
	if err != nil || existing == nil {
		return nil, ErrNotFound
	}
	if existing.BuiltIn {
		return nil, ErrBuiltInImmutable
	}

	if patch.DisplayName != nil {
		existing.DisplayName = *patch.DisplayName
	}
	if patch.Icon != nil {
		existing.Icon = *patch.Icon
	}
	if patch.DefaultColumns != nil {
		existing.DefaultColumns = *patch.DefaultColumns
	}
	if patch.DefaultConfig != nil {
		existing.DefaultConfig = orEmpty(*patch.DefaultConfig)
	}
	if patch.ConfigSchema != nil {
		existing.ConfigSchema = orEmpty(*patch.ConfigSchema)
	}
	if patch.Presentation != nil {
		existing.Presentation = orEmpty(*patch.Presentation)
	}

	if err := validateDefinition(existing.Type, existing.DisplayName, existing.DefaultColumns, existing.DefaultConfig, existing.ConfigSchema, existing.Presentation); err != nil {
		return nil, err
	}

	var out *BoardTypeDef
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var e error
		if out, e = tx.UpdateBoardType(ctx, existing); e != nil {
			return e
		}
		return insertEvent(ctx, tx, out, "boardtype.updated")
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Delete(ctx context.Context, typ string) error {
	existing, err := s.repo.GetBoardType(ctx, typ)
	if err != nil || existing == nil {
		return ErrNotFound
	}
	if existing.BuiltIn {
		return ErrBuiltInImmutable
	}
	return s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if e := tx.DeleteBoardType(ctx, typ); e != nil {
			return e
		}
		return insertEvent(ctx, tx, existing, "boardtype.deleted")
	})
}

func (s *service) Get(ctx context.Context, typ string) (*BoardTypeDef, error) {
	def, err := s.repo.GetBoardType(ctx, typ)
	if err != nil || def == nil {
		return nil, ErrNotFound
	}
	return def, nil
}

func (s *service) List(ctx context.Context) ([]*BoardTypeDef, error) {
	return s.repo.ListBoardTypes(ctx)
}

// insertEvent writes a boardtype.* event to the outbox. The payload carries the
// type slug so consumers (e.g. the project service) can invalidate their cache.
func insertEvent(ctx context.Context, tx Repository, def *BoardTypeDef, eventType string) error {
	payload, _ := json.Marshal(map[string]any{
		"type":         def.Type,
		"display_name": def.DisplayName,
	})
	return tx.InsertOutboxEvent(ctx, uuid.New(), def.ID, eventType, payload)
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

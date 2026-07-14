package events

import (
	"context"
	"log/slog"
)

type UserHandler struct {
	logger *slog.Logger
}

func NewUserHandler(logger *slog.Logger) *UserHandler {
	return &UserHandler{logger: logger}
}

// OnUserDeleted handles user.deleted — in MVP we log and do nothing.
// Webhooks created by the deleted user remain for audit purposes.
func (h *UserHandler) OnUserDeleted(ctx context.Context, env Envelope) error {
	h.logger.Info("user deleted event received (no-op)", "user_id", env.AggregateID)
	return nil
}

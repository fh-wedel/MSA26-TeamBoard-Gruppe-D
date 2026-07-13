package events

import (
	"context"
	"log/slog"

	"github.com/teamboard/services/plugin/internal/domain"
)

type ProjectHandler struct {
	repo   domain.Repository
	logger *slog.Logger
}

func NewProjectHandler(repo domain.Repository, logger *slog.Logger) *ProjectHandler {
	return &ProjectHandler{repo: repo, logger: logger}
}

func (h *ProjectHandler) OnProjectCreated(ctx context.Context, env Envelope) error {
	if err := h.repo.UpsertKnownProject(ctx, env.AggregateID); err != nil {
		return err
	}
	h.logger.Info("known project registered", "project_id", env.AggregateID)
	return nil
}

func (h *ProjectHandler) OnProjectDeleted(ctx context.Context, env Envelope) error {
	if err := h.repo.MarkKnownProjectDeleted(ctx, env.AggregateID); err != nil {
		return err
	}
	if err := h.repo.DeleteWebhooksByProject(ctx, env.AggregateID); err != nil {
		return err
	}
	h.logger.Info("project deleted — webhooks removed", "project_id", env.AggregateID)
	return nil
}

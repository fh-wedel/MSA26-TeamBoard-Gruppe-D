package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/teamboard/services/notification/internal/domain"
)

type Worker struct {
	repo          domain.Repository
	retentionDays int
	interval      time.Duration
}

func NewWorker(repo domain.Repository, retentionDays int) *Worker {
	return &Worker{repo: repo, retentionDays: retentionDays, interval: 6 * time.Hour}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			before := time.Now().AddDate(0, 0, -w.retentionDays)
			n, err := w.repo.DeleteOldNotifications(ctx, before)
			if err != nil {
				slog.ErrorContext(ctx, "cleanup: delete old notifications", "error", err)
			} else if n > 0 {
				slog.InfoContext(ctx, "cleanup: deleted old notifications", "count", n)
			}
		}
	}
}

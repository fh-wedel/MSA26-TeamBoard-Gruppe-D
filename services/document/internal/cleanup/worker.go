package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/teamboard/services/document/internal/domain"
	"github.com/teamboard/services/document/internal/storage"
)

const (
	retentionPeriod    = 30 * 24 * time.Hour // 30 days
	pendingGracePeriod = 2 * time.Hour        // abandon pending uploads after 2h
	batchSize          = 50
	defaultInterval    = 10 * time.Minute
)

// Worker periodically:
//  1. Marks stale pending versions as failed.
//  2. Hard-deletes soft-deleted documents older than the retention period (removing S3 objects first).
type Worker struct {
	repo     domain.Repository
	store    storage.ObjectStorage
	interval time.Duration
}

func NewWorker(repo domain.Repository, store storage.ObjectStorage) *Worker {
	return &Worker{repo: repo, store: store, interval: defaultInterval}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) {
	w.expirePendingVersions(ctx)
	w.expirePendingDocuments(ctx)
	w.hardDeleteExpired(ctx)
}

// expirePendingVersions marks versions that have been pending for too long as failed.
func (w *Worker) expirePendingVersions(ctx context.Context) {
	before := time.Now().Add(-pendingGracePeriod)
	vers, err := w.repo.ListPendingVersionsOlderThan(ctx, before)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: list pending versions", "error", err)
		return
	}
	for _, v := range vers {
		if err := w.repo.MarkVersionFailed(ctx, v.ID); err != nil {
			slog.ErrorContext(ctx, "cleanup: mark version failed", "version_id", v.ID, "error", err)
		}
	}
}

// expirePendingDocuments soft-deletes documents that never had a version confirmed.
func (w *Worker) expirePendingDocuments(ctx context.Context) {
	before := time.Now().Add(-pendingGracePeriod)
	docs, err := w.repo.ListPendingDocumentsOlderThan(ctx, before)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: list pending documents", "error", err)
		return
	}
	for _, d := range docs {
		if err := w.repo.SoftDeleteDocument(ctx, d.ID); err != nil {
			slog.ErrorContext(ctx, "cleanup: soft delete pending document", "document_id", d.ID, "error", err)
		}
	}
}

// hardDeleteExpired removes S3 objects then hard-deletes DB rows for documents
// that have been soft-deleted longer than retentionPeriod.
func (w *Worker) hardDeleteExpired(ctx context.Context) {
	before := time.Now().Add(-retentionPeriod)
	docs, err := w.repo.ListSoftDeletedOlderThan(ctx, before, batchSize)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: list soft-deleted documents", "error", err)
		return
	}

	for _, doc := range docs {
		w.deleteDocumentObjects(ctx, doc)
	}
}

func (w *Worker) deleteDocumentObjects(ctx context.Context, doc *domain.Document) {
	vers, err := w.repo.ListVersionsByDocument(ctx, doc.ID)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: list versions", "document_id", doc.ID, "error", err)
		return
	}

	keys := make([]string, 0, len(vers))
	for _, v := range vers {
		keys = append(keys, v.StorageKey)
	}

	if len(keys) > 0 {
		if err := w.store.DeleteObjects(ctx, keys); err != nil {
			slog.ErrorContext(ctx, "cleanup: delete S3 objects", "document_id", doc.ID, "error", err)
			return
		}
	}

	if err := w.repo.HardDeleteDocument(ctx, doc.ID); err != nil {
		slog.ErrorContext(ctx, "cleanup: hard delete document", "document_id", doc.ID, "error", err)
	}
}

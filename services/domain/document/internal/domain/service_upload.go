package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/document/internal/storage"
)

const (
	uploadURLTTL   = 15 * time.Minute
	maxFileSizeB   = 500 << 20 // 500 MB
)

type svc struct {
	repo        Repository
	store       storage.ObjectStorage
	proj        ProjectClient
	allowedCT   []string
	maxFileSize int64
}

// NewService constructs the domain service.
func NewService(repo Repository, store storage.ObjectStorage, proj ProjectClient, allowedContentTypes []string) DocumentService {
	if len(allowedContentTypes) == 0 {
		allowedContentTypes = DefaultAllowedContentTypes
	}
	return &svc{
		repo:        repo,
		store:       store,
		proj:        proj,
		allowedCT:   allowedContentTypes,
		maxFileSize: maxFileSizeB,
	}
}

func (s *svc) InitiateUpload(ctx context.Context, requester uuid.UUID, input InitiateUploadInput) (*UploadInitiation, error) {
	if err := s.validateUploadInput(input.ContentType, input.SizeBytes, input.Name); err != nil {
		return nil, err
	}

	known, err := s.repo.KnownProjectExists(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, ErrProjectUnknown
	}

	perms, err := s.proj.GetPermissions(ctx, input.ProjectID, requester)
	if err != nil {
		return nil, err
	}
	if !perms.Has("document:create") {
		return nil, ErrPermissionDenied
	}

	docID := uuid.New()
	versionID := uuid.New()
	storageKey := BuildStorageKey(input.ProjectID, docID, 1, input.Name)

	uploadURL, expiresAt, err := s.store.PutPresignedURL(ctx, storageKey, input.ContentType, input.SizeBytes, uploadURLTTL)
	if err != nil {
		return nil, fmt.Errorf("presign put: %w", err)
	}

	var (
		doc *Document
		ver *DocumentVersion
	)
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		doc, txErr = tx.CreateDocument(ctx, docID, input.ProjectID, input.Name, input.ContentType, requester)
		if txErr != nil {
			return txErr
		}
		ver, txErr = tx.CreateVersion(ctx, versionID, docID, 1, storageKey, input.ContentType, input.SizeBytes, requester)
		if txErr != nil {
			return txErr
		}
		evtID := uuid.New()
		payload := []byte(fmt.Sprintf(`{"document_id":%q,"project_id":%q,"name":%q,"initiated_by":%q}`,
			docID, input.ProjectID, input.Name, requester))
		return tx.InsertOutboxEvent(ctx, evtID, docID, "document.upload_initiated", payload)
	})
	if err != nil {
		return nil, err
	}

	return &UploadInitiation{
		Document:  doc,
		Version:   ver,
		UploadURL: uploadURL,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *svc) InitiateNewVersion(ctx context.Context, documentID, requester uuid.UUID, input NewVersionInput) (*UploadInitiation, error) {
	if err := s.validateUploadInput(input.ContentType, input.SizeBytes, ""); err != nil {
		return nil, err
	}

	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if doc.Status != DocStatusActive {
		return nil, ErrDocumentNotActive
	}

	perms, err := s.proj.GetPermissions(ctx, doc.ProjectID, requester)
	if err != nil {
		return nil, err
	}
	if !perms.Has("document:update") {
		return nil, ErrPermissionDenied
	}

	maxNum, err := s.repo.GetMaxVersionNumber(ctx, documentID)
	if err != nil {
		return nil, err
	}
	newNum := maxNum + 1
	versionID := uuid.New()
	storageKey := BuildStorageKey(doc.ProjectID, documentID, newNum, doc.Name)

	uploadURL, expiresAt, err := s.store.PutPresignedURL(ctx, storageKey, input.ContentType, input.SizeBytes, uploadURLTTL)
	if err != nil {
		return nil, fmt.Errorf("presign put: %w", err)
	}

	var ver *DocumentVersion
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		ver, txErr = tx.CreateVersion(ctx, versionID, documentID, newNum, storageKey, input.ContentType, input.SizeBytes, requester)
		return txErr
	})
	if err != nil {
		return nil, err
	}

	return &UploadInitiation{
		Document:  doc,
		Version:   ver,
		UploadURL: uploadURL,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *svc) ConfirmUpload(ctx context.Context, documentID, requester uuid.UUID, versionNumber int) (*Document, error) {
	doc, err := s.repo.GetDocumentInternal(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if doc.Status == DocStatusDeleted {
		return nil, ErrDocumentNotFound
	}

	perms, err := s.proj.GetPermissions(ctx, doc.ProjectID, requester)
	if err != nil {
		return nil, err
	}
	if !perms.Has("document:create") {
		return nil, ErrPermissionDenied
	}

	ver, err := s.repo.GetVersion(ctx, documentID, versionNumber)
	if err != nil {
		return nil, err
	}

	// Idempotent: already confirmed.
	if ver.Status == VersionStatusUploaded {
		return s.repo.GetDocument(ctx, documentID)
	}
	if ver.Status == VersionStatusFailed {
		return nil, ErrVersionNotPending
	}

	// Storage check happens outside the DB transaction (per spec).
	objInfo, err := s.store.HeadObject(ctx, ver.StorageKey)
	if err != nil {
		return nil, ErrUploadNotFound
	}
	if objInfo.SizeBytes != ver.SizeBytes {
		return nil, ErrUploadSizeMismatch
	}

	var result *Document
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if _, txErr := tx.MarkVersionUploaded(ctx, ver.ID, nil); txErr != nil {
			return txErr
		}

		var txErr error
		if doc.Status == DocStatusPending {
			result, txErr = tx.ActivateDocument(ctx, documentID, versionNumber)
		} else {
			result, txErr = tx.SetCurrentVersion(ctx, documentID, versionNumber)
		}
		if txErr != nil {
			return txErr
		}

		evtID := uuid.New()
		payload := []byte(fmt.Sprintf(`{"document_id":%q,"project_id":%q,"version":%d,"confirmed_by":%q}`,
			documentID, doc.ProjectID, versionNumber, requester))
		return tx.InsertOutboxEvent(ctx, evtID, documentID, "document.uploaded", payload)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *svc) validateUploadInput(contentType string, sizeBytes int64, _ string) error {
	if !IsContentTypeAllowed(contentType, s.allowedCT) {
		return ErrUnsupportedContentType
	}
	if sizeBytes <= 0 {
		// Guard against empty uploads before they reach the size_bytes > 0
		// DB CHECK constraint, which would otherwise surface as an untyped 500.
		return ErrEmptyFile
	}
	if sizeBytes > s.maxFileSize {
		return ErrFileTooLarge
	}
	return nil
}

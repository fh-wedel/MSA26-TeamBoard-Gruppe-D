package domain

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const downloadURLTTL = 5 * time.Minute

func (s *svc) GetDocument(ctx context.Context, documentID, requester uuid.UUID) (*Document, error) {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}
	perms, err := s.proj.GetPermissions(ctx, doc.ProjectID, requester)
	if err != nil {
		return nil, err
	}
	if !perms.Has("document:read") {
		return nil, ErrPermissionDenied
	}
	return doc, nil
}

func (s *svc) GetDownloadURL(ctx context.Context, documentID, requester uuid.UUID, version *int) (*DownloadInfo, error) {
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
	if !perms.Has("document:read") {
		return nil, ErrPermissionDenied
	}

	var ver *DocumentVersion
	if version != nil {
		ver, err = s.repo.GetVersion(ctx, documentID, *version)
	} else {
		ver, err = s.repo.GetCurrentVersion(ctx, documentID)
	}
	if err != nil {
		return nil, err
	}
	if ver.Status != VersionStatusUploaded {
		return nil, ErrVersionNotUploaded
	}

	url, expiresAt, err := s.store.GetPresignedURL(ctx, ver.StorageKey, downloadURLTTL)
	if err != nil {
		return nil, fmt.Errorf("presign get: %w", err)
	}
	return &DownloadInfo{
		Document:    doc,
		Version:     ver,
		DownloadURL: url,
		ExpiresAt:   expiresAt,
	}, nil
}

func (s *svc) ListVersions(ctx context.Context, documentID, requester uuid.UUID) ([]*DocumentVersion, error) {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}
	perms, err := s.proj.GetPermissions(ctx, doc.ProjectID, requester)
	if err != nil {
		return nil, err
	}
	if !perms.Has("document:read") {
		return nil, ErrPermissionDenied
	}
	return s.repo.ListVersionsByDocument(ctx, documentID)
}

func (s *svc) ListDocuments(ctx context.Context, projectID, requester uuid.UUID, page Pagination) ([]*Document, *PageCursor, error) {
	known, err := s.repo.KnownProjectExists(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	if !known {
		return nil, nil, ErrProjectUnknown
	}

	perms, err := s.proj.GetPermissions(ctx, projectID, requester)
	if err != nil {
		return nil, nil, err
	}
	if !perms.Has("document:read") {
		return nil, nil, ErrPermissionDenied
	}

	limit := page.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var (
		cursor   *time.Time
		cursorID *uuid.UUID
	)
	if page.Cursor != nil {
		raw, decErr := base64.StdEncoding.DecodeString(*page.Cursor)
		if decErr != nil {
			return nil, nil, &Error{Code: ErrValidation.Code, Message: "invalid cursor"}
		}
		var pc PageCursor
		if jsonErr := json.Unmarshal(raw, &pc); jsonErr != nil {
			return nil, nil, &Error{Code: ErrValidation.Code, Message: "invalid cursor"}
		}
		cursor = &pc.CreatedAt
		cursorID = &pc.ID
	}

	docs, err := s.repo.ListDocumentsByProject(ctx, projectID, cursor, cursorID, limit+1)
	if err != nil {
		return nil, nil, err
	}

	var nextCursor *PageCursor
	if len(docs) > limit {
		docs = docs[:limit]
		last := docs[len(docs)-1]
		nextCursor = &PageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return docs, nextCursor, nil
}

func (s *svc) GetDocumentInfo(ctx context.Context, documentID uuid.UUID) (*DocumentInfo, error) {
	doc, err := s.repo.GetDocumentInternal(ctx, documentID)
	if err != nil {
		return nil, err
	}
	return &DocumentInfo{
		ID:        doc.ID,
		ProjectID: doc.ProjectID,
		Name:      doc.Name,
		Exists:    true,
		Active:    doc.Status == DocStatusActive,
	}, nil
}

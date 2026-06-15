package domain

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *svc) RenameDocument(ctx context.Context, documentID, requester uuid.UUID, newName string) (*Document, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return nil, &Error{Code: ErrValidation.Code, Message: "name must not be empty"}
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

	var result *Document
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		result, txErr = tx.UpdateDocumentName(ctx, documentID, newName)
		if txErr != nil {
			return txErr
		}
		evtID := uuid.New()
		payload := []byte(fmt.Sprintf(`{"document_id":%q,"project_id":%q,"new_name":%q,"renamed_by":%q}`,
			documentID, doc.ProjectID, newName, requester))
		return tx.InsertOutboxEvent(ctx, evtID, documentID, "document.renamed", payload)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *svc) DeleteDocument(ctx context.Context, documentID, requester uuid.UUID) error {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return err
	}

	perms, err := s.proj.GetPermissions(ctx, doc.ProjectID, requester)
	if err != nil {
		return err
	}
	if !perms.Has("document:delete") {
		return ErrPermissionDenied
	}

	return s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if txErr := tx.SoftDeleteDocument(ctx, documentID); txErr != nil {
			return txErr
		}
		evtID := uuid.New()
		payload := []byte(fmt.Sprintf(`{"document_id":%q,"project_id":%q,"deleted_by":%q}`,
			documentID, doc.ProjectID, requester))
		return tx.InsertOutboxEvent(ctx, evtID, documentID, "document.deleted", payload)
	})
}

func (s *svc) RestoreVersion(ctx context.Context, documentID, requester uuid.UUID, versionNumber int) (*Document, error) {
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

	ver, err := s.repo.GetVersion(ctx, documentID, versionNumber)
	if err != nil {
		return nil, err
	}
	if ver.Status != VersionStatusUploaded {
		return nil, ErrVersionNotUploaded
	}

	var result *Document
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		result, txErr = tx.SetCurrentVersion(ctx, documentID, versionNumber)
		if txErr != nil {
			return txErr
		}
		evtID := uuid.New()
		payload := []byte(fmt.Sprintf(`{"document_id":%q,"project_id":%q,"version":%d,"restored_by":%q}`,
			documentID, doc.ProjectID, versionNumber, requester))
		return tx.InsertOutboxEvent(ctx, evtID, documentID, "document.version_restored", payload)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

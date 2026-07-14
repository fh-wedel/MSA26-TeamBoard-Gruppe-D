package domain_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/domain/document/internal/domain"
)

// ── ConfirmUpload — failed version path ──────────────────────────────────────

func TestConfirmUpload_VersionFailed(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	docID := uuid.New()
	now := time.Now()
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Name: "doc.pdf",
		ContentType: "application/pdf", Status: domain.DocStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	repo.versions[docID] = []*domain.DocumentVersion{{
		ID: uuid.New(), DocumentID: docID, VersionNumber: 1,
		StorageKey: "key", SizeBytes: 100,
		Status: domain.VersionStatusFailed, CreatedAt: now,
	}}

	svc := makeService(repo, proj, store)
	_, err := svc.ConfirmUpload(context.Background(), docID, uuid.New(), 1)
	assertDomainErr(t, err, domain.ErrVersionNotPending)
}

func TestConfirmUpload_VersionNotFound(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	docID := uuid.New()
	now := time.Now()
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Name: "doc.pdf",
		ContentType: "application/pdf", Status: domain.DocStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}

	svc := makeService(repo, proj, store)
	_, err := svc.ConfirmUpload(context.Background(), docID, uuid.New(), 99)
	assertDomainErr(t, err, domain.ErrVersionNotFound)
}

func TestConfirmUpload_DocumentDeleted(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	docID := uuid.New()
	now := time.Now()
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Name: "doc.pdf",
		ContentType: "application/pdf", Status: domain.DocStatusDeleted,
		CreatedAt: now, UpdatedAt: now,
	}

	svc := makeService(repo, proj, store)
	_, err := svc.ConfirmUpload(context.Background(), docID, uuid.New(), 1)
	assertDomainErr(t, err, domain.ErrDocumentNotFound)
}

func TestConfirmUpload_NewVersionOnActiveDocument(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)

	// Simulate second pending version
	v2Key := domain.BuildStorageKey(projectID, doc.ID, 2, "test.pdf")
	repo.versions[doc.ID] = append(repo.versions[doc.ID], &domain.DocumentVersion{
		ID: uuid.New(), DocumentID: doc.ID, VersionNumber: 2,
		StorageKey: v2Key, SizeBytes: 2048,
		Status: domain.VersionStatusPending, CreatedAt: time.Now(),
	})
	store.store(v2Key, 2048)

	svc := makeService(repo, proj, store)
	result, err := svc.ConfirmUpload(context.Background(), doc.ID, uuid.New(), 2)
	require.NoError(t, err)
	assert.Equal(t, 2, *result.CurrentVersion)
}

// ── GetDownloadURL — version not uploaded ─────────────────────────────────────

func TestGetDownloadURL_PendingVersionNotDownloadable(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	docID := uuid.New()
	now := time.Now()
	v1 := 1
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Status: domain.DocStatusActive,
		Name: "x.pdf", ContentType: "application/pdf",
		CurrentVersion: &v1, CreatedAt: now, UpdatedAt: now,
	}
	repo.versions[docID] = []*domain.DocumentVersion{{
		ID: uuid.New(), DocumentID: docID, VersionNumber: 1,
		StorageKey: "key", SizeBytes: 100,
		Status: domain.VersionStatusPending, CreatedAt: now,
	}}

	svc := makeService(repo, proj, store)
	_, err := svc.GetDownloadURL(context.Background(), docID, uuid.New(), nil)
	assertDomainErr(t, err, domain.ErrVersionNotUploaded)
}

func TestGetDownloadURL_SpecificVersionNotFound(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	v := 99
	_, err := svc.GetDownloadURL(context.Background(), doc.ID, uuid.New(), &v)
	assertDomainErr(t, err, domain.ErrVersionNotFound)
}

// ── ListDocuments — cursor path ───────────────────────────────────────────────

func TestListDocuments_InvalidCursor(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	svc := makeService(repo, proj, store)
	badCursor := "not-valid-base64!!!"
	_, _, err := svc.ListDocuments(context.Background(), projectID, uuid.New(), domain.Pagination{
		Limit:  10,
		Cursor: &badCursor,
	})
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, domain.ErrValidation.Code, de.Code)
}

func TestListDocuments_ValidCursor(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	for i := 0; i < 4; i++ {
		seedActiveDocument(t, repo, store, projectID)
	}

	svc := makeService(repo, proj, store)

	// First page
	docs1, cursor, err := svc.ListDocuments(context.Background(), projectID, uuid.New(), domain.Pagination{Limit: 2})
	require.NoError(t, err)
	require.NotNil(t, cursor)
	assert.Len(t, docs1, 2)

	// Encode the cursor as the service would
	rawCursor, err := json.Marshal(cursor)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(rawCursor)

	// Second page
	docs2, _, err := svc.ListDocuments(context.Background(), projectID, uuid.New(), domain.Pagination{
		Limit:  2,
		Cursor: &encoded,
	})
	require.NoError(t, err)
	// fakeRepo ignores cursor in query but doesn't crash
	assert.NotNil(t, docs2)
}

func TestListDocuments_DefaultLimit(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	svc := makeService(repo, proj, store)
	_, _, err := svc.ListDocuments(context.Background(), projectID, uuid.New(), domain.Pagination{Limit: 0})
	require.NoError(t, err)
}

// ── RenameDocument — document not active ─────────────────────────────────────

func TestRenameDocument_DocumentNotActive(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	docID := uuid.New()
	now := time.Now()
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Status: domain.DocStatusPending,
		Name: "x.pdf", CreatedAt: now, UpdatedAt: now,
	}

	svc := makeService(repo, proj, store)
	_, err := svc.RenameDocument(context.Background(), docID, uuid.New(), "new.pdf")
	assertDomainErr(t, err, domain.ErrDocumentNotActive)
}

// ── RestoreVersion — document not active ─────────────────────────────────────

func TestRestoreVersion_DocumentNotActive(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	docID := uuid.New()
	now := time.Now()
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Status: domain.DocStatusPending,
		Name: "x.pdf", CreatedAt: now, UpdatedAt: now,
	}

	svc := makeService(repo, proj, store)
	_, err := svc.RestoreVersion(context.Background(), docID, uuid.New(), 1)
	assertDomainErr(t, err, domain.ErrDocumentNotActive)
}

// ── Error.Unwrap ──────────────────────────────────────────────────────────────

func TestDomainError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	wrapped := &domain.Error{Code: "test", Message: "msg", Cause: cause}
	assert.Equal(t, cause, wrapped.Unwrap())

	nilCause := &domain.Error{Code: "test", Message: "msg"}
	assert.Nil(t, nilCause.Unwrap())
	assert.True(t, errors.Is(wrapped, cause))
}

// ── BuildStorageKey — name truncation ─────────────────────────────────────────

func TestBuildStorageKey_LongName(t *testing.T) {
	pid := uuid.New()
	did := uuid.New()
	longName := "a"
	for len(longName) < 150 {
		longName += "a"
	}
	key := domain.BuildStorageKey(pid, did, 1, longName)
	// path segment should be <= 100 chars
	parts := len("projects/") + 36 + len("/documents/") + 36 + len("/v1/")
	segment := key[parts:]
	assert.LessOrEqual(t, len(segment), 100)
}

// ── InitiateNewVersion — permission denied ─────────────────────────────────────

func TestInitiateNewVersion_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := denyAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	_, err := svc.InitiateNewVersion(context.Background(), doc.ID, uuid.New(), domain.NewVersionInput{
		ContentType: "application/pdf", SizeBytes: 100,
	})
	assertDomainErr(t, err, domain.ErrPermissionDenied)
}

// ── DeleteDocument — not found ────────────────────────────────────────────────

func TestDeleteDocument_NotFound(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	svc := makeService(repo, proj, store)

	err := svc.DeleteDocument(context.Background(), uuid.New(), uuid.New())
	assertDomainErr(t, err, domain.ErrDocumentNotFound)
}

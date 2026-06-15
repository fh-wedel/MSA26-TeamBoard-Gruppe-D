package domain_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/document/internal/domain"
	"github.com/teamboard/services/document/internal/storage"
)

// ── Fake repository ────────────────────────────────────────────────────────────

type fakeRepo struct {
	mu        sync.Mutex
	docs      map[uuid.UUID]*domain.Document
	versions  map[uuid.UUID][]*domain.DocumentVersion // documentID → versions
	projects  map[uuid.UUID]bool
	users     map[uuid.UUID]bool
	outbox    []*domain.OutboxEvent
	processed map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		docs:      make(map[uuid.UUID]*domain.Document),
		versions:  make(map[uuid.UUID][]*domain.DocumentVersion),
		projects:  make(map[uuid.UUID]bool),
		users:     make(map[uuid.UUID]bool),
		processed: make(map[string]bool),
	}
}

func (r *fakeRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	return fn(ctx, r)
}

func (r *fakeRepo) UpsertKnownProject(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.projects[id] = true; return nil
}
func (r *fakeRepo) KnownProjectExists(_ context.Context, id uuid.UUID) (bool, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	return r.projects[id], nil
}
func (r *fakeRepo) MarkKnownProjectDeleted(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	delete(r.projects, id); return nil
}
func (r *fakeRepo) UpsertKnownUser(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.users[id] = true; return nil
}
func (r *fakeRepo) MarkKnownUserDeleted(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	delete(r.users, id); return nil
}

func (r *fakeRepo) CreateDocument(_ context.Context, id, projectID uuid.UUID, name, contentType string, createdBy uuid.UUID) (*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	doc := &domain.Document{
		ID: id, ProjectID: projectID, Name: name, ContentType: contentType,
		Status: domain.DocStatusPending, CreatedBy: createdBy,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	r.docs[id] = doc
	return doc, nil
}

func (r *fakeRepo) GetDocument(_ context.Context, id uuid.UUID) (*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok || doc.Status == domain.DocStatusDeleted {
		return nil, domain.ErrDocumentNotFound
	}
	return doc, nil
}

func (r *fakeRepo) GetDocumentInternal(_ context.Context, id uuid.UUID) (*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok {
		return nil, domain.ErrDocumentNotFound
	}
	return doc, nil
}

func (r *fakeRepo) ListDocumentsByProject(_ context.Context, projectID uuid.UUID, cursor *time.Time, cursorID *uuid.UUID, limit int) ([]*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var result []*domain.Document
	for _, d := range r.docs {
		if d.ProjectID == projectID && d.Status == domain.DocStatusActive {
			result = append(result, d)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *fakeRepo) UpdateDocumentName(_ context.Context, id uuid.UUID, name string) (*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok {
		return nil, domain.ErrDocumentNotFound
	}
	doc.Name = name
	return doc, nil
}

func (r *fakeRepo) ActivateDocument(_ context.Context, id uuid.UUID, versionNumber int) (*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok {
		return nil, domain.ErrDocumentNotFound
	}
	doc.Status = domain.DocStatusActive
	doc.CurrentVersion = &versionNumber
	return doc, nil
}

func (r *fakeRepo) SetCurrentVersion(_ context.Context, id uuid.UUID, versionNumber int) (*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok {
		return nil, domain.ErrDocumentNotFound
	}
	doc.CurrentVersion = &versionNumber
	return doc, nil
}

func (r *fakeRepo) SoftDeleteDocument(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok {
		return domain.ErrDocumentNotFound
	}
	doc.Status = domain.DocStatusDeleted
	now := time.Now()
	doc.DeletedAt = &now
	return nil
}

func (r *fakeRepo) SoftDeleteDocumentsByProject(_ context.Context, projectID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var res []struct{ ID, ProjectID uuid.UUID }
	for _, d := range r.docs {
		if d.ProjectID == projectID {
			d.Status = domain.DocStatusDeleted
			res = append(res, struct{ ID, ProjectID uuid.UUID }{d.ID, d.ProjectID})
		}
	}
	return res, nil
}

func (r *fakeRepo) ListPendingDocumentsOlderThan(_ context.Context, before time.Time) ([]*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var res []*domain.Document
	for _, d := range r.docs {
		if d.Status == domain.DocStatusPending && d.CreatedAt.Before(before) {
			res = append(res, d)
		}
	}
	return res, nil
}

func (r *fakeRepo) ListSoftDeletedOlderThan(_ context.Context, before time.Time, limit int) ([]*domain.Document, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var res []*domain.Document
	for _, d := range r.docs {
		if d.Status == domain.DocStatusDeleted && d.DeletedAt != nil && d.DeletedAt.Before(before) {
			res = append(res, d)
		}
	}
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *fakeRepo) HardDeleteDocument(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	delete(r.docs, id)
	return nil
}

func (r *fakeRepo) CreateVersion(_ context.Context, id, documentID uuid.UUID, versionNumber int, storageKey, contentType string, sizeBytes int64, uploadedBy uuid.UUID) (*domain.DocumentVersion, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	v := &domain.DocumentVersion{
		ID: id, DocumentID: documentID, VersionNumber: versionNumber,
		StorageKey: storageKey, ContentType: contentType, SizeBytes: sizeBytes,
		Status: domain.VersionStatusPending, UploadedBy: uploadedBy,
		CreatedAt: time.Now(),
	}
	r.versions[documentID] = append(r.versions[documentID], v)
	return v, nil
}

func (r *fakeRepo) GetVersion(_ context.Context, documentID uuid.UUID, versionNumber int) (*domain.DocumentVersion, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	for _, v := range r.versions[documentID] {
		if v.VersionNumber == versionNumber {
			return v, nil
		}
	}
	return nil, domain.ErrVersionNotFound
}

func (r *fakeRepo) GetCurrentVersion(_ context.Context, documentID uuid.UUID) (*domain.DocumentVersion, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	doc, ok := r.docs[documentID]
	if !ok || doc.CurrentVersion == nil {
		return nil, domain.ErrVersionNotFound
	}
	for _, v := range r.versions[documentID] {
		if v.VersionNumber == *doc.CurrentVersion {
			return v, nil
		}
	}
	return nil, domain.ErrVersionNotFound
}

func (r *fakeRepo) ListVersionsByDocument(_ context.Context, documentID uuid.UUID) ([]*domain.DocumentVersion, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	return r.versions[documentID], nil
}

func (r *fakeRepo) GetMaxVersionNumber(_ context.Context, documentID uuid.UUID) (int, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	max := 0
	for _, v := range r.versions[documentID] {
		if v.VersionNumber > max {
			max = v.VersionNumber
		}
	}
	return max, nil
}

func (r *fakeRepo) MarkVersionUploaded(_ context.Context, id uuid.UUID, checksum *string) (*domain.DocumentVersion, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	for _, vs := range r.versions {
		for _, v := range vs {
			if v.ID == id {
				v.Status = domain.VersionStatusUploaded
				now := time.Now()
				v.UploadedAt = &now
				return v, nil
			}
		}
	}
	return nil, domain.ErrVersionNotFound
}

func (r *fakeRepo) MarkVersionFailed(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	for _, vs := range r.versions {
		for _, v := range vs {
			if v.ID == id {
				v.Status = domain.VersionStatusFailed
				return nil
			}
		}
	}
	return nil
}

func (r *fakeRepo) ListPendingVersionsOlderThan(_ context.Context, before time.Time) ([]*domain.DocumentVersion, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var res []*domain.DocumentVersion
	for _, vs := range r.versions {
		for _, v := range vs {
			if v.Status == domain.VersionStatusPending && v.CreatedAt.Before(before) {
				res = append(res, v)
			}
		}
	}
	return res, nil
}

func (r *fakeRepo) ListFailedVersions(_ context.Context, limit int) ([]*domain.DocumentVersion, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var res []*domain.DocumentVersion
	for _, vs := range r.versions {
		for _, v := range vs {
			if v.Status == domain.VersionStatusFailed {
				res = append(res, v)
			}
		}
	}
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *fakeRepo) DeleteVersion(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	for docID, vs := range r.versions {
		for i, v := range vs {
			if v.ID == id {
				r.versions[docID] = append(vs[:i], vs[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

func (r *fakeRepo) InsertOutboxEvent(_ context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.outbox = append(r.outbox, &domain.OutboxEvent{ID: id, AggregateID: aggregateID, EventType: eventType, Payload: payload})
	return nil
}

func (r *fakeRepo) GetUnpublishedEvents(_ context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var res []*domain.OutboxEvent
	for _, e := range r.outbox {
		if e.PublishedAt == nil {
			res = append(res, e)
		}
	}
	if int32(len(res)) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *fakeRepo) MarkEventPublished(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	now := time.Now()
	for _, e := range r.outbox {
		if e.ID == id {
			e.PublishedAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) WasEventProcessed(_ context.Context, eventID string) (bool, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	return r.processed[eventID], nil
}

func (r *fakeRepo) MarkEventProcessed(_ context.Context, eventID string) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.processed[eventID] = true
	return nil
}

// ── Fake project client ────────────────────────────────────────────────────────

type fakeProjClient struct {
	perms *domain.PermissionSet
	err   error
}

func allowAll() *fakeProjClient {
	return &fakeProjClient{perms: &domain.PermissionSet{
		Role:          "member",
		Permissions:   []string{"document:read", "document:create", "document:update", "document:delete"},
		ProjectExists: true,
		IsMember:      true,
	}}
}

func denyAll() *fakeProjClient {
	return &fakeProjClient{perms: &domain.PermissionSet{
		Role: "none", Permissions: []string{}, ProjectExists: true, IsMember: false,
	}}
}

func (f *fakeProjClient) GetPermissions(_ context.Context, _, _ uuid.UUID) (*domain.PermissionSet, error) {
	return f.perms, f.err
}

// ── Fake storage ──────────────────────────────────────────────────────────────

type fakeStorage struct {
	mu      sync.Mutex
	objects map[string]*storage.ObjectInfo
	putErr  error
	getErr  error
	headErr error
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: make(map[string]*storage.ObjectInfo)}
}

func (s *fakeStorage) store(key string, sizeBytes int64) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.objects[key] = &storage.ObjectInfo{
		Key: key, SizeBytes: sizeBytes, ContentType: "application/pdf",
		ETag: "etag", LastModified: time.Now(),
	}
}

func (s *fakeStorage) PutPresignedURL(_ context.Context, key, _ string, _ int64, ttl time.Duration) (string, time.Time, error) {
	if s.putErr != nil {
		return "", time.Time{}, s.putErr
	}
	return "http://minio:9000/test/" + key, time.Now().Add(ttl), nil
}

func (s *fakeStorage) GetPresignedURL(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	if s.getErr != nil {
		return "", time.Time{}, s.getErr
	}
	return "http://minio:9000/test/" + key + "?dl=1", time.Now().Add(ttl), nil
}

func (s *fakeStorage) HeadObject(_ context.Context, key string) (*storage.ObjectInfo, error) {
	if s.headErr != nil {
		return nil, s.headErr
	}
	s.mu.Lock(); defer s.mu.Unlock()
	info, ok := s.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return info, nil
}

func (s *fakeStorage) DeleteObject(_ context.Context, key string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	delete(s.objects, key); return nil
}

func (s *fakeStorage) DeleteObjects(_ context.Context, keys []string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.objects, k)
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func makeService(repo *fakeRepo, proj *fakeProjClient, store *fakeStorage) domain.DocumentService {
	return domain.NewService(repo, store, proj, nil)
}

func assertDomainErr(t *testing.T, err error, expected *domain.Error) {
	t.Helper()
	var de *domain.Error
	require.True(t, errors.As(err, &de), "expected domain.Error, got %T: %v", err, err)
	assert.Equal(t, expected.Code, de.Code)
}

// seedActiveDocument creates a document in active state with one uploaded version.
func seedActiveDocument(t *testing.T, repo *fakeRepo, store *fakeStorage, projectID uuid.UUID) *domain.Document {
	t.Helper()
	ctx := context.Background()
	docID := uuid.New()
	now := time.Now()
	doc := &domain.Document{
		ID: docID, ProjectID: projectID, Name: "test.pdf",
		ContentType: "application/pdf", Status: domain.DocStatusActive,
		CreatedBy: uuid.New(), CreatedAt: now, UpdatedAt: now,
	}
	v1 := 1
	doc.CurrentVersion = &v1
	repo.docs[docID] = doc

	verID := uuid.New()
	storageKey := domain.BuildStorageKey(projectID, docID, 1, "test.pdf")
	repo.versions[docID] = []*domain.DocumentVersion{{
		ID: verID, DocumentID: docID, VersionNumber: 1,
		StorageKey: storageKey, ContentType: "application/pdf",
		SizeBytes: 1024, Status: domain.VersionStatusUploaded,
		UploadedBy: doc.CreatedBy, CreatedAt: now,
	}}
	store.store(storageKey, 1024)
	_ = ctx
	return doc
}

// ── Tests — InitiateUpload ────────────────────────────────────────────────────

func TestInitiateUpload_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	svc := makeService(repo, proj, store)
	result, err := svc.InitiateUpload(context.Background(), uuid.New(), domain.InitiateUploadInput{
		ProjectID:   projectID,
		Name:        "report.pdf",
		ContentType: "application/pdf",
		SizeBytes:   2048,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.UploadURL)
	assert.Equal(t, domain.DocStatusPending, result.Document.Status)
	assert.Equal(t, 1, result.Version.VersionNumber)
}

func TestInitiateUpload_UnknownProject(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	svc := makeService(repo, proj, store)

	_, err := svc.InitiateUpload(context.Background(), uuid.New(), domain.InitiateUploadInput{
		ProjectID:   uuid.New(),
		Name:        "file.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	})
	assertDomainErr(t, err, domain.ErrProjectUnknown)
}

func TestInitiateUpload_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := denyAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	svc := makeService(repo, proj, store)
	_, err := svc.InitiateUpload(context.Background(), uuid.New(), domain.InitiateUploadInput{
		ProjectID: projectID, Name: "file.pdf", ContentType: "application/pdf", SizeBytes: 1024,
	})
	assertDomainErr(t, err, domain.ErrPermissionDenied)
}

func TestInitiateUpload_UnsupportedContentType(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	svc := makeService(repo, proj, store)
	_, err := svc.InitiateUpload(context.Background(), uuid.New(), domain.InitiateUploadInput{
		ProjectID: projectID, Name: "malware.exe", ContentType: "application/octet-stream", SizeBytes: 100,
	})
	assertDomainErr(t, err, domain.ErrUnsupportedContentType)
}

func TestInitiateUpload_FileTooLarge(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	svc := makeService(repo, proj, store)
	_, err := svc.InitiateUpload(context.Background(), uuid.New(), domain.InitiateUploadInput{
		ProjectID: projectID, Name: "huge.pdf", ContentType: "application/pdf",
		SizeBytes: 600 << 20, // 600 MB
	})
	assertDomainErr(t, err, domain.ErrFileTooLarge)
}

// ── Tests — InitiateNewVersion ────────────────────────────────────────────────

func TestInitiateNewVersion_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	result, err := svc.InitiateNewVersion(context.Background(), doc.ID, uuid.New(), domain.NewVersionInput{
		ContentType: "application/pdf", SizeBytes: 4096,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Version.VersionNumber)
	assert.NotEmpty(t, result.UploadURL)
}

func TestInitiateNewVersion_DocumentNotActive(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	docID := uuid.New()
	projectID := uuid.New()
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Status: domain.DocStatusPending, Name: "file.pdf",
	}

	svc := makeService(repo, proj, store)
	_, err := svc.InitiateNewVersion(context.Background(), docID, uuid.New(), domain.NewVersionInput{
		ContentType: "application/pdf", SizeBytes: 100,
	})
	assertDomainErr(t, err, domain.ErrDocumentNotActive)
}

// ── Tests — ConfirmUpload ─────────────────────────────────────────────────────

func TestConfirmUpload_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	// Create pending document with pending version
	docID := uuid.New()
	now := time.Now()
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Name: "doc.pdf",
		ContentType: "application/pdf", Status: domain.DocStatusPending,
		CreatedBy: uuid.New(), CreatedAt: now, UpdatedAt: now,
	}
	storageKey := domain.BuildStorageKey(projectID, docID, 1, "doc.pdf")
	repo.versions[docID] = []*domain.DocumentVersion{{
		ID: uuid.New(), DocumentID: docID, VersionNumber: 1,
		StorageKey: storageKey, ContentType: "application/pdf",
		SizeBytes: 1024, Status: domain.VersionStatusPending,
		CreatedAt: now,
	}}
	store.store(storageKey, 1024)

	svc := makeService(repo, proj, store)
	doc, err := svc.ConfirmUpload(context.Background(), docID, uuid.New(), 1)
	require.NoError(t, err)
	assert.Equal(t, domain.DocStatusActive, doc.Status)
	assert.NotNil(t, doc.CurrentVersion)
	assert.Equal(t, 1, *doc.CurrentVersion)
}

func TestConfirmUpload_Idempotent(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	// Version is already uploaded — should return 200 with current state.
	result, err := svc.ConfirmUpload(context.Background(), doc.ID, uuid.New(), 1)
	require.NoError(t, err)
	assert.Equal(t, domain.DocStatusActive, result.Status)
}

func TestConfirmUpload_ObjectNotInStorage(t *testing.T) {
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
		StorageKey: "non-existent-key", SizeBytes: 100,
		Status: domain.VersionStatusPending, CreatedAt: now,
	}}
	// Do NOT store object in fakeStorage.

	svc := makeService(repo, proj, store)
	_, err := svc.ConfirmUpload(context.Background(), docID, uuid.New(), 1)
	assertDomainErr(t, err, domain.ErrUploadNotFound)
}

func TestConfirmUpload_SizeMismatch(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	docID := uuid.New()
	now := time.Now()
	storageKey := domain.BuildStorageKey(projectID, docID, 1, "doc.pdf")
	repo.docs[docID] = &domain.Document{
		ID: docID, ProjectID: projectID, Name: "doc.pdf",
		ContentType: "application/pdf", Status: domain.DocStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	repo.versions[docID] = []*domain.DocumentVersion{{
		ID: uuid.New(), DocumentID: docID, VersionNumber: 1,
		StorageKey: storageKey, SizeBytes: 9999, // declared 9999
		Status: domain.VersionStatusPending, CreatedAt: now,
	}}
	store.store(storageKey, 1234) // actual is 1234

	svc := makeService(repo, proj, store)
	_, err := svc.ConfirmUpload(context.Background(), docID, uuid.New(), 1)
	assertDomainErr(t, err, domain.ErrUploadSizeMismatch)
}

// ── Tests — GetDocument ────────────────────────────────────────────────────────

func TestGetDocument_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	got, err := svc.GetDocument(context.Background(), doc.ID, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, doc.ID, got.ID)
}

func TestGetDocument_NotFound(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	svc := makeService(repo, proj, store)

	_, err := svc.GetDocument(context.Background(), uuid.New(), uuid.New())
	assertDomainErr(t, err, domain.ErrDocumentNotFound)
}

func TestGetDocument_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := denyAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	_, err := svc.GetDocument(context.Background(), doc.ID, uuid.New())
	assertDomainErr(t, err, domain.ErrPermissionDenied)
}

// ── Tests — GetDownloadURL ────────────────────────────────────────────────────

func TestGetDownloadURL_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	info, err := svc.GetDownloadURL(context.Background(), doc.ID, uuid.New(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, info.DownloadURL)
	assert.Equal(t, 1, info.Version.VersionNumber)
}

func TestGetDownloadURL_SpecificVersion(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	v := 1
	info, err := svc.GetDownloadURL(context.Background(), doc.ID, uuid.New(), &v)
	require.NoError(t, err)
	assert.Equal(t, 1, info.Version.VersionNumber)
}

func TestGetDownloadURL_DocumentNotActive(t *testing.T) {
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
	_, err := svc.GetDownloadURL(context.Background(), docID, uuid.New(), nil)
	assertDomainErr(t, err, domain.ErrDocumentNotActive)
}

// ── Tests — ListDocuments ─────────────────────────────────────────────────────

func TestListDocuments_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	seedActiveDocument(t, repo, store, projectID)
	seedActiveDocument(t, repo, store, projectID)

	svc := makeService(repo, proj, store)
	docs, cursor, err := svc.ListDocuments(context.Background(), projectID, uuid.New(), domain.Pagination{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, docs, 2)
	assert.Nil(t, cursor)
}

func TestListDocuments_UnknownProject(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	svc := makeService(repo, proj, store)

	_, _, err := svc.ListDocuments(context.Background(), uuid.New(), uuid.New(), domain.Pagination{Limit: 10})
	assertDomainErr(t, err, domain.ErrProjectUnknown)
}

func TestListDocuments_Pagination(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()
	require.NoError(t, repo.UpsertKnownProject(context.Background(), projectID))

	for i := 0; i < 5; i++ {
		seedActiveDocument(t, repo, store, projectID)
	}

	svc := makeService(repo, proj, store)
	docs, cursor, err := svc.ListDocuments(context.Background(), projectID, uuid.New(), domain.Pagination{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, docs, 3)
	assert.NotNil(t, cursor)
}

// ── Tests — RenameDocument ────────────────────────────────────────────────────

func TestRenameDocument_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	renamed, err := svc.RenameDocument(context.Background(), doc.ID, uuid.New(), "new-name.pdf")
	require.NoError(t, err)
	assert.Equal(t, "new-name.pdf", renamed.Name)
}

func TestRenameDocument_EmptyName(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	_, err := svc.RenameDocument(context.Background(), doc.ID, uuid.New(), "  ")
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, domain.ErrValidation.Code, de.Code)
}

func TestRenameDocument_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := denyAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	_, err := svc.RenameDocument(context.Background(), doc.ID, uuid.New(), "other.pdf")
	assertDomainErr(t, err, domain.ErrPermissionDenied)
}

// ── Tests — DeleteDocument ────────────────────────────────────────────────────

func TestDeleteDocument_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	err := svc.DeleteDocument(context.Background(), doc.ID, uuid.New())
	require.NoError(t, err)

	// Verify soft-deleted
	deleted, _ := repo.GetDocumentInternal(context.Background(), doc.ID)
	assert.Equal(t, domain.DocStatusDeleted, deleted.Status)
}

func TestDeleteDocument_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := denyAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	err := svc.DeleteDocument(context.Background(), doc.ID, uuid.New())
	assertDomainErr(t, err, domain.ErrPermissionDenied)
}

// ── Tests — RestoreVersion ────────────────────────────────────────────────────

func TestRestoreVersion_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)

	// Add a second uploaded version
	v2Key := domain.BuildStorageKey(projectID, doc.ID, 2, "test.pdf")
	repo.versions[doc.ID] = append(repo.versions[doc.ID], &domain.DocumentVersion{
		ID: uuid.New(), DocumentID: doc.ID, VersionNumber: 2,
		StorageKey: v2Key, ContentType: "application/pdf",
		SizeBytes: 2048, Status: domain.VersionStatusUploaded,
		UploadedBy: uuid.New(), CreatedAt: time.Now(),
	})
	store.store(v2Key, 2048)

	// Set current to v2
	v2 := 2
	repo.docs[doc.ID].CurrentVersion = &v2

	svc := makeService(repo, proj, store)

	// Restore to v1
	restored, err := svc.RestoreVersion(context.Background(), doc.ID, uuid.New(), 1)
	require.NoError(t, err)
	assert.Equal(t, 1, *restored.CurrentVersion)
}

func TestRestoreVersion_NotUploaded(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)

	// Add a pending version
	repo.versions[doc.ID] = append(repo.versions[doc.ID], &domain.DocumentVersion{
		ID: uuid.New(), DocumentID: doc.ID, VersionNumber: 2,
		StorageKey: "key2", SizeBytes: 100,
		Status: domain.VersionStatusPending, CreatedAt: time.Now(),
	})

	svc := makeService(repo, proj, store)
	_, err := svc.RestoreVersion(context.Background(), doc.ID, uuid.New(), 2)
	assertDomainErr(t, err, domain.ErrVersionNotUploaded)
}

// ── Tests — GetDocumentInfo ───────────────────────────────────────────────────

func TestGetDocumentInfo_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	info, err := svc.GetDocumentInfo(context.Background(), doc.ID)
	require.NoError(t, err)
	assert.Equal(t, doc.ID, info.ID)
	assert.True(t, info.Active)
	assert.True(t, info.Exists)
}

func TestGetDocumentInfo_NotFound(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	svc := makeService(repo, proj, store)

	_, err := svc.GetDocumentInfo(context.Background(), uuid.New())
	assertDomainErr(t, err, domain.ErrDocumentNotFound)
}

// ── Tests — Entity helpers ────────────────────────────────────────────────────

func TestBuildStorageKey(t *testing.T) {
	pid := uuid.MustParse("11111111-0000-0000-0000-000000000000")
	did := uuid.MustParse("22222222-0000-0000-0000-000000000000")
	key := domain.BuildStorageKey(pid, did, 3, "My Report (2024).pdf")
	assert.Contains(t, key, "projects/")
	assert.Contains(t, key, "/v3/")
	assert.NotContains(t, key, " ")
	assert.NotContains(t, key, "(")
}

func TestIsContentTypeAllowed(t *testing.T) {
	list := []string{"application/pdf", "image/png"}
	assert.True(t, domain.IsContentTypeAllowed("application/pdf", list))
	assert.True(t, domain.IsContentTypeAllowed("APPLICATION/PDF", list))
	assert.False(t, domain.IsContentTypeAllowed("application/octet-stream", list))
}

func TestDomainError_Formatting(t *testing.T) {
	err := domain.ErrDocumentNotFound
	assert.Contains(t, err.Error(), "document_not_found")

	wrapped := &domain.Error{Code: "test", Message: "msg", Cause: errors.New("cause")}
	assert.Contains(t, wrapped.Error(), "cause")
	assert.Equal(t, "test", wrapped.GetCode())
}

func TestPermissionSet_Has(t *testing.T) {
	ps := &domain.PermissionSet{Permissions: []string{"document:read", "document:create"}}
	assert.True(t, ps.Has("document:read"))
	assert.False(t, ps.Has("document:delete"))
}

// ── Tests — ListVersions ──────────────────────────────────────────────────────

func TestListVersions_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := allowAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	vers, err := svc.ListVersions(context.Background(), doc.ID, uuid.New())
	require.NoError(t, err)
	assert.Len(t, vers, 1)
	assert.Equal(t, 1, vers[0].VersionNumber)
}

func TestListVersions_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStorage()
	proj := denyAll()
	projectID := uuid.New()

	doc := seedActiveDocument(t, repo, store, projectID)
	svc := makeService(repo, proj, store)

	_, err := svc.ListVersions(context.Background(), doc.ID, uuid.New())
	assertDomainErr(t, err, domain.ErrPermissionDenied)
}

package api

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/document/internal/domain"
)

// ── Request DTOs ──────────────────────────────────────────────────────────────

type initiateUploadRequest struct {
	ProjectID   uuid.UUID `json:"project_id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
}

type initiateNewVersionRequest struct {
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type renameDocumentRequest struct {
	Name string `json:"name"`
}

// ── Response DTOs ─────────────────────────────────────────────────────────────

type documentDTO struct {
	ID             uuid.UUID  `json:"id"`
	ProjectID      uuid.UUID  `json:"project_id"`
	Name           string     `json:"name"`
	ContentType    string     `json:"content_type"`
	CurrentVersion *int       `json:"current_version"`
	Status         string     `json:"status"`
	CreatedBy      uuid.UUID  `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type versionDTO struct {
	ID            uuid.UUID  `json:"id"`
	DocumentID    uuid.UUID  `json:"document_id"`
	VersionNumber int        `json:"version_number"`
	ContentType   string     `json:"content_type"`
	SizeBytes     int64      `json:"size_bytes"`
	Status        string     `json:"status"`
	UploadedBy    uuid.UUID  `json:"uploaded_by"`
	UploadedAt    *time.Time `json:"uploaded_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type uploadInitiationDTO struct {
	Document  documentDTO `json:"document"`
	Version   versionDTO  `json:"version"`
	UploadURL string      `json:"upload_url"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type downloadInfoDTO struct {
	Document    documentDTO `json:"document"`
	Version     versionDTO  `json:"version"`
	DownloadURL string      `json:"download_url"`
	ExpiresAt   time.Time   `json:"expires_at"`
}

type documentListDTO struct {
	Data       []documentDTO   `json:"data"`
	Pagination *paginationDTO  `json:"pagination,omitempty"`
}

type paginationDTO struct {
	NextCursor *string `json:"next_cursor,omitempty"`
}

// ── Mapping ───────────────────────────────────────────────────────────────────

func mapDocument(d *domain.Document) documentDTO {
	return documentDTO{
		ID:             d.ID,
		ProjectID:      d.ProjectID,
		Name:           d.Name,
		ContentType:    d.ContentType,
		CurrentVersion: d.CurrentVersion,
		Status:         string(d.Status),
		CreatedBy:      d.CreatedBy,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

func mapVersion(v *domain.DocumentVersion) versionDTO {
	return versionDTO{
		ID:            v.ID,
		DocumentID:    v.DocumentID,
		VersionNumber: v.VersionNumber,
		ContentType:   v.ContentType,
		SizeBytes:     v.SizeBytes,
		Status:        string(v.Status),
		UploadedBy:    v.UploadedBy,
		UploadedAt:    v.UploadedAt,
		CreatedAt:     v.CreatedAt,
	}
}

func mapUploadInitiation(ui *domain.UploadInitiation) uploadInitiationDTO {
	return uploadInitiationDTO{
		Document:  mapDocument(ui.Document),
		Version:   mapVersion(ui.Version),
		UploadURL: ui.UploadURL,
		ExpiresAt: ui.ExpiresAt,
	}
}

func mapDownloadInfo(di *domain.DownloadInfo) downloadInfoDTO {
	return downloadInfoDTO{
		Document:    mapDocument(di.Document),
		Version:     mapVersion(di.Version),
		DownloadURL: di.DownloadURL,
		ExpiresAt:   di.ExpiresAt,
	}
}

func encodeCursor(c *domain.PageCursor) *string {
	if c == nil {
		return nil
	}
	raw, _ := json.Marshal(c)
	s := base64.StdEncoding.EncodeToString(raw)
	return &s
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/domain/document/internal/domain"
)

func handleInitiateUpload(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req initiateUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		result, err := svc.InitiateUpload(r.Context(), requester, domain.InitiateUploadInput{
			ProjectID:   req.ProjectID,
			Name:        req.Name,
			ContentType: req.ContentType,
			SizeBytes:   req.SizeBytes,
		})
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, mapUploadInitiation(result))
	}
}

func handleInitiateNewVersion(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		var req initiateNewVersionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		result, err := svc.InitiateNewVersion(r.Context(), docID, requester, domain.NewVersionInput{
			ContentType: req.ContentType,
			SizeBytes:   req.SizeBytes,
		})
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, mapUploadInitiation(result))
	}
}

func handleConfirmUpload(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		// Strip :confirm suffix from {version}:confirm capture
		versionStr := strings.TrimSuffix(chi.URLParam(r, "version"), ":confirm")
		versionNum, err := strconv.Atoi(versionStr)
		if err != nil {
			http.Error(w, `{"error":"invalid version"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		doc, err := svc.ConfirmUpload(r.Context(), docID, requester, versionNum)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, mapDocument(doc))
	}
}

func handleGetDocument(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		doc, err := svc.GetDocument(r.Context(), docID, requester)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, mapDocument(doc))
	}
}

func handleGetDownloadURL(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)

		var version *int
		if v := r.URL.Query().Get("version"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				http.Error(w, `{"error":"invalid version"}`, http.StatusBadRequest)
				return
			}
			version = &n
		}

		info, err := svc.GetDownloadURL(r.Context(), docID, requester, version)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, mapDownloadInfo(info))
	}
}

func handleListVersions(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		vers, err := svc.ListVersions(r.Context(), docID, requester)
		if err != nil {
			writeError(w, r, err)
			return
		}
		dtos := make([]versionDTO, len(vers))
		for i, v := range vers {
			dtos[i] = mapVersion(v)
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

func handleListDocuments(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectIDStr := r.URL.Query().Get("project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			http.Error(w, `{"error":"invalid project_id"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)

		var cursor *string
		if c := r.URL.Query().Get("cursor"); c != "" {
			cursor = &c
		}
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				limit = n
			}
		}

		docs, nextCursor, err := svc.ListDocuments(r.Context(), projectID, requester, domain.Pagination{
			Limit:  limit,
			Cursor: cursor,
		})
		if err != nil {
			writeError(w, r, err)
			return
		}

		dtos := make([]documentDTO, len(docs))
		for i, d := range docs {
			dtos[i] = mapDocument(d)
		}

		var pagination *paginationDTO
		if nc := encodeCursor(nextCursor); nc != nil {
			pagination = &paginationDTO{NextCursor: nc}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(documentListDTO{Data: dtos, Pagination: pagination})
	}
}

func handleRenameDocument(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		var req renameDocumentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		doc, err := svc.RenameDocument(r.Context(), docID, requester, req.Name)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, mapDocument(doc))
	}
}

func handleDeleteDocument(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		if err := svc.DeleteDocument(r.Context(), docID, requester); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRestoreVersion(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		versionStr := strings.TrimSuffix(chi.URLParam(r, "version"), ":restore")
		versionNum, err := strconv.Atoi(versionStr)
		if err != nil {
			http.Error(w, `{"error":"invalid version"}`, http.StatusBadRequest)
			return
		}
		requester := mustUserID(r)
		doc, err := svc.RestoreVersion(r.Context(), docID, requester, versionNum)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, mapDocument(doc))
	}
}

func handleGetDocumentInfo(svc domain.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
		if err != nil {
			http.Error(w, `{"error":"invalid document id"}`, http.StatusBadRequest)
			return
		}
		info, err := svc.GetDocumentInfo(r.Context(), docID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}

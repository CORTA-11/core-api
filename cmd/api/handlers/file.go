package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (router *Router) getFiles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ctx := r.Context()
	}
}

func (router *Router) uploadFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		orgID, teamID, ok := fileScopeFromContext(ctx)
		if !ok {
			http.Error(w, "failed to get file scope", http.StatusInternalServerError)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "error reading file", http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()

		err = router.fileService.UploadFile(ctx, orgID, teamID, header.Filename, file)
		if err != nil {
			if errors.Is(err, service.ErrInvalidFileName) {
				http.Error(w, "invalid file name", http.StatusBadRequest)
				return
			}
			slog.ErrorContext(ctx, "failed to upload file", "error", err)
			http.Error(w, "failed to upload file", http.StatusInternalServerError)
			return
		}
		if _, err := fmt.Fprintf(w, "File %s uploaded successfully.", header.Filename); err != nil {
			slog.ErrorContext(ctx, "failed to write upload response", "error", err)
		}
	}
}

func (router *Router) downloadFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		orgID, teamID, ok := fileScopeFromContext(ctx)
		if !ok {
			http.Error(w, "failed to get file scope", http.StatusInternalServerError)
			return
		}

		fileName := chi.URLParam(r, "filename")
		if fileName == "" {
			http.Error(w, "filename is not valid", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
		w.Header().Set("Content-Type", "application/octet-stream")

		err := router.fileService.DownloadFile(ctx, orgID, teamID, fileName, w)

		if err != nil {
			w.Header().Del("Content-Disposition")
			if errors.Is(err, service.ErrInvalidFileName) {
				http.Error(w, "invalid file name", http.StatusBadRequest)
				return
			}
			slog.ErrorContext(ctx, "failed to download file", "error", err)
			http.Error(w, "failed to download file", http.StatusInternalServerError)
			return
		}
	}
}

func fileScopeFromContext(ctx context.Context) (uuid.UUID, int, bool) {
	orgIDString, ok := appMiddleware.OrgIDFromContext(ctx)
	if !ok {
		slog.ErrorContext(ctx, "organization ID missing from request context")
		return uuid.Nil, 0, false
	}

	orgID, err := uuid.Parse(orgIDString)
	if err != nil {
		slog.ErrorContext(ctx, "invalid organization ID in request context", "error", err)
		return uuid.Nil, 0, false
	}

	teamID, ok := appMiddleware.TeamIDFromContext(ctx)
	if !ok {
		slog.ErrorContext(ctx, "team ID missing from request context")
		return uuid.Nil, 0, false
	}

	return orgID, teamID, true
}

package handlers

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"mime"
	"net/http"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
)

func (router *Router) getFiles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := router.trustedTeam(w, r); !ok {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (router *Router) uploadFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		team, ok := router.trustedTeam(w, r)
		if !ok {
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "error reading file", http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()

		err = router.fileService.UploadFile(ctx, team, header.Filename, file)
		if err != nil {
			if errors.Is(err, service.ErrInvalidFileName) {
				http.Error(w, "invalid file name", http.StatusBadRequest)
				return
			}
			slog.ErrorContext(ctx, "failed to upload file", "error", err)
			http.Error(w, "failed to upload file", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := fmt.Fprintf(w, "File %s uploaded successfully.", html.EscapeString(header.Filename)); err != nil {
			slog.ErrorContext(ctx, "failed to write upload response", "error", err)
		}
	}
}

func (router *Router) downloadFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		team, ok := router.trustedTeam(w, r)
		if !ok {
			return
		}

		fileName := chi.URLParam(r, "filename")
		if fileName == "" {
			http.Error(w, "filename is not valid", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
		w.Header().Set("Content-Type", "application/octet-stream")

		err := router.fileService.DownloadFile(ctx, team, fileName, w)

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

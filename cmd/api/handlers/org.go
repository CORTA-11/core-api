package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (router *Router) getOrgs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		orgs, err := router.orgService.GetOrgs(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to get organizations", "error", err)
			http.Error(w, "failed to get organizations", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(orgs); err != nil {
			slog.ErrorContext(ctx, "failed to encode organization object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (router *Router) createOrg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req struct {
			Name string `json:"name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid response body", http.StatusBadRequest)
			return
		}

		req.Name = strings.TrimSpace(req.Name)

		if req.Name == "" {
			http.Error(w, "organization name is required", http.StatusBadRequest)
			return
		}

		if len(req.Name) > 255 {
			http.Error(w, "organization name must be less than 255 characters", http.StatusBadRequest)
			return
		}

		org, err := router.orgService.CreateOrg(ctx, req.Name)
		if err != nil {
			slog.ErrorContext(ctx, "failed to create organization", "error", err)
			http.Error(w, "failed to create organization", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		// Creation commits provisioning intent only; schema allocation and tenant
		// migrations continue asynchronously in the provisioner.
		w.WriteHeader(http.StatusAccepted)

		if err := json.NewEncoder(w).Encode(org); err != nil {
			slog.ErrorContext(ctx, "failed to encode organization object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (router *Router) updateOrg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		orgIDStr := chi.URLParam(r, "orgID")
		if orgIDStr == "" {
			http.Error(w, "organization id is required", http.StatusBadRequest)
			return
		}

		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "invalid organization public_id", http.StatusBadRequest)
			return
		}

		var req struct {
			Name string `json:"name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid response body", http.StatusBadRequest)
			return
		}

		req.Name = strings.TrimSpace(req.Name)

		if req.Name == "" {
			http.Error(w, "organization name is required", http.StatusBadRequest)
			return
		}

		if len(req.Name) > 255 {
			http.Error(w, "organization name must be less than 255 characters", http.StatusBadRequest)
			return
		}

		org, err := router.orgService.UpdateOrg(ctx, orgID, req.Name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}

			slog.ErrorContext(ctx, "failed to update organization", "error", err)
			http.Error(w, "failed to update organization", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(org); err != nil {
			slog.ErrorContext(ctx, "failed to encode organization object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (router *Router) deleteOrg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		orgIDStr := chi.URLParam(r, "orgID")

		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "invalid organization id", http.StatusBadRequest)
			return
		}

		org, err := router.orgService.SoftDeleteOrg(ctx, orgID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}

			slog.ErrorContext(ctx, "failed to delete organization", "error", err)
			http.Error(w, "failed to delete organization", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(org); err != nil {
			slog.ErrorContext(ctx, "failed to encode organization object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (router *Router) restoreOrg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var req struct {
			PublicID string `json:"public_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		orgID, err := uuid.Parse(req.PublicID)
		if err != nil {
			http.Error(w, "invalid organization id", http.StatusBadRequest)
			return
		}

		org, err := router.orgService.RestoreOrg(ctx, orgID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "organization not found or already active", http.StatusNotFound)
				return
			}

			slog.ErrorContext(ctx, "failed to restore organization", "error", err)
			http.Error(w, "failed to restore organization", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(org); err != nil {
			slog.ErrorContext(ctx, "failed to encode organization object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

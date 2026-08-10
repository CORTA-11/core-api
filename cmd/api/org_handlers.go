package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (router *Router) getOrgs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgs, err := router.queries.GetOrgs(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(orgs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (router *Router) createOrg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		org, err := router.queries.CreateOrg(r.Context(), req.Name)
		if err != nil {
			http.Error(w, "failed to create organization", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		var response struct {
			PublicID  string    `json:"public_id"`
			Name      string    `json:"name"`
			CreatedAt time.Time `json:"created_at"`
		}

		response.PublicID = org.PublicID.String()
		response.Name = org.Name
		response.CreatedAt = org.CreatedAt

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (router *Router) updateOrg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicIDStr := chi.URLParam(r, "orgId")

		publicID, err := uuid.Parse(publicIDStr)
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

		org, err := router.queries.UpdateOrg(r.Context(), repository.UpdateOrgParams{
			PublicID: publicID,
			Name:     req.Name,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}

			http.Error(w, "failed to update organization", http.StatusInternalServerError)
			return
		}

		var response struct {
			PublicID  string    `json:"public_id"`
			Name      string    `json:"name"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		}

		response.PublicID = org.PublicID.String()
		response.Name = org.Name
		response.CreatedAt = org.CreatedAt
		response.UpdatedAt = org.UpdatedAt

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

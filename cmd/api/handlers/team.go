package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
)

func (router *Router) getTeams() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		orgIDStr, ok := appMiddleware.OrgIDFromContext(ctx)
		if !ok {
			slog.ErrorContext(ctx, "organization ID missing from request context")
			http.Error(w, "failed to get organization ID", http.StatusInternalServerError)
			return
		}

		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			slog.ErrorContext(ctx, "invalid organization ID in request context", "error", err)
			http.Error(w, "failed to get organization ID", http.StatusInternalServerError)
			return
		}

		schemaName := service.SchemaName(orgID)

		teams, err := router.teamService.GetTeams(ctx, schemaName)
		if err != nil {
			slog.ErrorContext(ctx, "failed to get teams", "error", err)
			http.Error(w, "failed to get teams", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(teams); err != nil {
			slog.ErrorContext(ctx, "failed to encode team object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	})
}

func (router *Router) createTeam() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req struct {
			Name string `json:"name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid response body", http.StatusBadRequest)
			return
		}

		if req.Name == "" {
			http.Error(w, "team name is required", http.StatusBadRequest)
			return
		}

		if len(req.Name) > 255 {
			http.Error(w, "team name must be less than 255 characters", http.StatusBadRequest)
			return
		}

		orgIDStr, ok := appMiddleware.OrgIDFromContext(ctx)
		if !ok {
			slog.ErrorContext(ctx, "organization ID missing from request context")
			http.Error(w, "failed to get organization ID", http.StatusInternalServerError)
			return
		}

		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			slog.ErrorContext(ctx, "invalid organization ID in request context", "error", err)
			http.Error(w, "failed to get organization ID", http.StatusInternalServerError)
			return
		}

		schemaName := service.SchemaName(orgID)

		team, err := router.teamService.CreateTeam(ctx, req.Name, schemaName)
		if err != nil {
			slog.ErrorContext(ctx, "failed to create team", "error", err)
			http.Error(w, "failed to create team", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(team); err != nil {
			slog.ErrorContext(ctx, "failed to encode team object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

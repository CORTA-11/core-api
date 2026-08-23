package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
)

func (router *Router) resolveOrganizationContext(ctx context.Context) (tenancy.OrganizationContext, error) {
	organizationValue, ok := appMiddleware.OrgIDFromContext(ctx)
	if !ok {
		return tenancy.OrganizationContext{}, tenancy.ErrOrganizationUnavailable
	}
	userValue, ok := appMiddleware.UserIDFromContext(ctx)
	if !ok {
		return tenancy.OrganizationContext{}, tenancy.ErrOrganizationUnavailable
	}
	organizationID, err := uuid.Parse(organizationValue)
	if err != nil {
		return tenancy.OrganizationContext{}, tenancy.ErrOrganizationUnavailable
	}
	userID, err := uuid.Parse(userValue)
	if err != nil {
		return tenancy.OrganizationContext{}, tenancy.ErrOrganizationUnavailable
	}
	if router.tenantResolver == nil {
		return tenancy.OrganizationContext{}, tenancy.ErrOrganizationUnavailable
	}
	return router.tenantResolver.ResolveOrganization(ctx, userID, organizationID)
}

func (router *Router) getTeams() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		organization, err := router.resolveOrganizationContext(ctx)
		if err != nil {
			http.Error(w, "organization unavailable", http.StatusNotFound)
			return
		}

		teams, err := router.teamService.GetTeams(ctx, organization)
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

		organization, err := router.resolveOrganizationContext(ctx)
		if err != nil {
			http.Error(w, "organization unavailable", http.StatusNotFound)
			return
		}

		team, err := router.teamService.CreateTeam(ctx, organization, req.Name)
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

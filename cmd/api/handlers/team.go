package handlers

import (
	"encoding/json"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
)

func (router *Router) getTeams() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		orgIDStr, _ := appMiddleware.OrgIDFromContext(r.Context())
		orgID, _ := uuid.Parse(orgIDStr)

		schemaName := service.SchemaName(orgID)

		teams, err := router.teamService.GetTeams(ctx, schemaName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err = json.NewEncoder(w).Encode(teams); err != nil {
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

		orgIDStr, _ := appMiddleware.OrgIDFromContext(r.Context())
		orgID, _ := uuid.Parse(orgIDStr)

		schemaName := service.SchemaName(orgID)

		team, err := router.teamService.CreateTeam(ctx, req.Name, schemaName)
		if err != nil {
			http.Error(w, "failed to create team", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(team); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

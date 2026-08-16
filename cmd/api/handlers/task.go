package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (router *Router) getTasks() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		teamIDStr := chi.URLParam(r, "teamID")

		teamID, err := strconv.Atoi(teamIDStr)
		if err != nil {
			http.Error(w, "invalid team id", http.StatusBadRequest)
			return
		}

		orgIDStr, _ := appMiddleware.OrgIDFromContext(ctx)
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "invalid orgID", http.StatusBadRequest)
		}

		schemaName := SchemaName(orgID)

		tasks, err := router.taskService.GetTasks(ctx, schemaName, teamID)
		if err != nil {
			http.Error(w, "failed to fetch tasks: %q", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func (router *Router) createTask() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req struct {
			Description string `json:"description"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		teamIDStr := chi.URLParam(r, "teamID")

		teamID, err := strconv.Atoi(teamIDStr)
		if err != nil {
			http.Error(w, "invalid team id", http.StatusBadRequest)
			return
		}

		orgIDStr, _ := appMiddleware.OrgIDFromContext(ctx)
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "invalid orgID", http.StatusBadRequest)
		}

		schemaName := SchemaName(orgID)

		task, err := router.taskService.CreateTask(ctx, schemaName, teamID, req.Description)
		if err != nil {
			http.Error(w, "failed to create task", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(task); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

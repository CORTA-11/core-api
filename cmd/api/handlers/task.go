package handlers

import (
	"encoding/json"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
)

func (router *Router) getTasks() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		teamID, _ := appMiddleware.TeamIDFromContext(ctx)

		orgIDStr, _ := appMiddleware.OrgIDFromContext(ctx)
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "invalid orgID", http.StatusBadRequest)
		}

		schemaName := service.SchemaName(orgID)

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

		teamID, _ := appMiddleware.TeamIDFromContext(ctx)

		orgIDStr, _ := appMiddleware.OrgIDFromContext(ctx)
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "invalid orgID", http.StatusBadRequest)
		}

		schemaName := service.SchemaName(orgID)

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

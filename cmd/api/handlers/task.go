package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
)

func (router *Router) getTasks() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		teamID, ok := appMiddleware.TeamIDFromContext(ctx)
		if !ok {
			slog.ErrorContext(ctx, "team ID missing from request context")
			http.Error(w, "failed to get team ID", http.StatusInternalServerError)
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

		tasks, err := router.taskService.GetTasks(ctx, schemaName, teamID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to fetch tasks", "error", err)
			http.Error(w, "failed to fetch tasks", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			slog.ErrorContext(ctx, "failed to encode task object", "error", err)
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

		teamID, ok := appMiddleware.TeamIDFromContext(ctx)
		if !ok {
			slog.ErrorContext(ctx, "team ID missing from request context")
			http.Error(w, "failed to get team ID", http.StatusInternalServerError)
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

		task, err := router.taskService.CreateTask(ctx, schemaName, teamID, req.Description)
		if err != nil {
			slog.ErrorContext(ctx, "failed to create task", "error", err)
			http.Error(w, "failed to create task", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(task); err != nil {
			slog.ErrorContext(ctx, "failed to encode task object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
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
			Status      string `json:"status"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(req.Description) == "" {
			http.Error(w, "task description is required", http.StatusBadRequest)
			return
		}

		if req.Status != "" && !service.IsValidTaskStatus(req.Status) {
			http.Error(w, "task status is invalid", http.StatusBadRequest)
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

		task, err := router.taskService.CreateTask(ctx, schemaName, teamID, req.Description, req.Status)
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

func (router *Router) updateTask() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		taskIDStr := chi.URLParam(r, "taskID")
		taskID, err := strconv.Atoi(taskIDStr)
		if err != nil || taskID <= 0 {
			http.Error(w, "invalid task id", http.StatusBadRequest)
			return
		}

		var req struct {
			Description string `json:"description"`
			Status      string `json:"status"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(req.Description) == "" {
			http.Error(w, "task description is required", http.StatusBadRequest)
			return
		}

		if !service.IsValidTaskStatus(req.Status) {
			http.Error(w, "task status is invalid", http.StatusBadRequest)
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

		task, err := router.taskService.UpdateTask(ctx, schemaName, teamID, taskID, req.Description, req.Status)
		if err != nil {
			slog.ErrorContext(ctx, "failed to update task", "error", err)
			http.Error(w, "failed to update task", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(task); err != nil {
			slog.ErrorContext(ctx, "failed to encode task object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func (router *Router) deleteTask() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		taskIDStr := chi.URLParam(r, "taskID")
		taskID, err := strconv.Atoi(taskIDStr)
		if err != nil || taskID <= 0 {
			http.Error(w, "invalid task id", http.StatusBadRequest)
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

		task, err := router.taskService.DeleteTask(ctx, schemaName, teamID, taskID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to delete task", "error", err)
			http.Error(w, "failed to delete task", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(task); err != nil {
			slog.ErrorContext(ctx, "failed to encode task object", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

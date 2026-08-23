package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (router *Router) trustedTeam(w http.ResponseWriter, r *http.Request) (tenancy.TeamContext, bool) {
	teamID, err := uuid.Parse(chi.URLParam(r, "team"))
	if err != nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return tenancy.TeamContext{}, false
	}
	organization, err := router.resolveOrganizationContext(r.Context())
	if err != nil {
		http.Error(w, "team unavailable", http.StatusNotFound)
		return tenancy.TeamContext{}, false
	}
	team, err := router.tenantResolver.ResolveTeam(r.Context(), organization, teamID)
	if err != nil {
		http.Error(w, "team unavailable", http.StatusNotFound)
		return tenancy.TeamContext{}, false
	}
	return team, true
}

func writeTaskServiceError(ctx context.Context, w http.ResponseWriter, operation string, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "task unavailable", http.StatusNotFound)
		return
	}
	slog.ErrorContext(ctx, operation, "error", err)
	http.Error(w, operation, http.StatusInternalServerError)
}

func (router *Router) getTasks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		team, ok := router.trustedTeam(w, r)
		if !ok {
			return
		}
		tasks, err := router.taskService.GetTasks(r.Context(), team)
		if err != nil {
			writeTaskServiceError(r.Context(), w, "failed to fetch tasks", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			slog.ErrorContext(r.Context(), "failed to encode tasks", "error", err)
		}
	}
}

func (router *Router) createTask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Description string `json:"description"`
			Status      string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(request.Description) == "" {
			http.Error(w, "task description is required", http.StatusBadRequest)
			return
		}
		if request.Status != "" && !service.IsValidTaskStatus(request.Status) {
			http.Error(w, "task status is invalid", http.StatusBadRequest)
			return
		}
		team, ok := router.trustedTeam(w, r)
		if !ok {
			return
		}
		task, err := router.taskService.CreateTask(r.Context(), team, request.Description, request.Status)
		if err != nil {
			writeTaskServiceError(r.Context(), w, "failed to create task", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(task)
	}
}

func (router *Router) updateTask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
		if err != nil {
			http.Error(w, "invalid task id", http.StatusBadRequest)
			return
		}
		var request struct {
			Description string `json:"description"`
			Status      string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(request.Description) == "" {
			http.Error(w, "task description is required", http.StatusBadRequest)
			return
		}
		if !service.IsValidTaskStatus(request.Status) {
			http.Error(w, "task status is invalid", http.StatusBadRequest)
			return
		}
		team, ok := router.trustedTeam(w, r)
		if !ok {
			return
		}
		task, err := router.taskService.UpdateTask(r.Context(), team, taskID, request.Description, request.Status)
		if err != nil {
			writeTaskServiceError(r.Context(), w, "failed to update task", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(task)
	}
}

func (router *Router) deleteTask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
		if err != nil {
			http.Error(w, "invalid task id", http.StatusBadRequest)
			return
		}
		team, ok := router.trustedTeam(w, r)
		if !ok {
			return
		}
		task, err := router.taskService.DeleteTask(r.Context(), team, taskID)
		if err != nil {
			writeTaskServiceError(r.Context(), w, "failed to delete task", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(task)
	}
}

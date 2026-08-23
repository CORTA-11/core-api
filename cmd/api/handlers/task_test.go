package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTaskService struct {
	getTasksFn   func(context.Context, tenancy.TeamContext) ([]service.Task, error)
	createTaskFn func(context.Context, tenancy.TeamContext, string, string) (*service.Task, error)
	updateTaskFn func(context.Context, tenancy.TeamContext, uuid.UUID, string, string) (*service.Task, error)
	deleteTaskFn func(context.Context, tenancy.TeamContext, uuid.UUID) (*service.Task, error)
}

func (stub *stubTaskService) GetTasks(ctx context.Context, team tenancy.TeamContext) ([]service.Task, error) {
	return stub.getTasksFn(ctx, team)
}

func (stub *stubTaskService) CreateTask(ctx context.Context, team tenancy.TeamContext, description, status string) (*service.Task, error) {
	return stub.createTaskFn(ctx, team, description, status)
}

func (stub *stubTaskService) UpdateTask(ctx context.Context, team tenancy.TeamContext, taskID uuid.UUID, description, status string) (*service.Task, error) {
	return stub.updateTaskFn(ctx, team, taskID, description, status)
}

func (stub *stubTaskService) DeleteTask(ctx context.Context, team tenancy.TeamContext, taskID uuid.UUID) (*service.Task, error) {
	return stub.deleteTaskFn(ctx, team, taskID)
}

func taskFixture() service.Task {
	return service.Task{
		PublicID:    uuid.MustParse("3daaba7d-bab9-41b8-bf1c-1d4977774120"),
		Description: "Add pagination",
		Status:      "todo",
		CreatedAt:   time.Date(2026, time.August, 23, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, time.August, 23, 10, 0, 0, 0, time.UTC),
	}
}

func performTrustedTaskRequest(t *testing.T, taskService service.TaskService, method, path, body string, resolver stubTeamResolver) *httptest.ResponseRecorder {
	t.Helper()
	tokenService := service.NewTokenService("task-handler-test-secret")
	token, err := tokenService.GenerateToken(uuid.MustParse("5a17231d-7570-4b82-b7cf-24ab0248d724"), "task@example.test")
	require.NoError(t, err)
	router := chi.NewRouter()
	router.Mount("/{team}/tasks", taskRouter(&Router{
		taskService: taskService, tokenService: tokenService, tenantResolver: resolver,
	}))
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Org-ID", "30ee7153-9b48-4560-8cbf-972587a60fda")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestTaskRoutesUseTrustedUUIDContextsAndPublicDTOs(t *testing.T) {
	teamID := uuid.MustParse("7ba60f7b-0ae7-4be4-a444-ce344276ed6e")
	task := taskFixture()
	resolver := stubTeamResolver{resolveTeamFn: func(_ context.Context, _ tenancy.OrganizationContext, got uuid.UUID) (tenancy.TeamContext, error) {
		assert.Equal(t, teamID, got)
		return tenancy.TeamContext{}, nil
	}}

	t.Run("list", func(t *testing.T) {
		serviceStub := &stubTaskService{getTasksFn: func(context.Context, tenancy.TeamContext) ([]service.Task, error) {
			return []service.Task{task}, nil
		}}
		response := performTrustedTaskRequest(t, serviceStub, http.MethodGet, "/"+teamID.String()+"/tasks/", "", resolver)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.NotContains(t, response.Body.String(), `"team_id"`)
		assert.NotContains(t, response.Body.String(), `"id":`)
		assert.Contains(t, response.Body.String(), `"public_id"`)
	})

	t.Run("create", func(t *testing.T) {
		serviceStub := &stubTaskService{createTaskFn: func(_ context.Context, _ tenancy.TeamContext, description, status string) (*service.Task, error) {
			assert.Equal(t, "Add pagination", description)
			assert.Equal(t, "todo", status)
			return &task, nil
		}}
		response := performTrustedTaskRequest(t, serviceStub, http.MethodPost, "/"+teamID.String()+"/tasks/", `{"description":"Add pagination","status":"todo"}`, resolver)
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("update", func(t *testing.T) {
		serviceStub := &stubTaskService{updateTaskFn: func(_ context.Context, _ tenancy.TeamContext, gotTaskID uuid.UUID, _, _ string) (*service.Task, error) {
			assert.Equal(t, task.PublicID, gotTaskID)
			return &task, nil
		}}
		response := performTrustedTaskRequest(t, serviceStub, http.MethodPut, "/"+teamID.String()+"/tasks/"+task.PublicID.String(), `{"description":"Updated","status":"done"}`, resolver)
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("delete", func(t *testing.T) {
		serviceStub := &stubTaskService{deleteTaskFn: func(_ context.Context, _ tenancy.TeamContext, gotTaskID uuid.UUID) (*service.Task, error) {
			assert.Equal(t, task.PublicID, gotTaskID)
			return &task, nil
		}}
		response := performTrustedTaskRequest(t, serviceStub, http.MethodDelete, "/"+teamID.String()+"/tasks/"+task.PublicID.String(), "", resolver)
		assert.Equal(t, http.StatusOK, response.Code)
	})
}

func TestTaskRoutesRejectUntrustedSelectorsAndMissingJWT(t *testing.T) {
	tokenService := service.NewTokenService("task-handler-test-secret")
	router := chi.NewRouter()
	router.Mount("/{team}/tasks", taskRouter(&Router{taskService: &stubTaskService{}, tokenService: tokenService, tenantResolver: stubTeamResolver{}}))
	request := httptest.NewRequest(http.MethodGet, "/"+uuid.NewString()+"/tasks/", nil)
	request.Header.Set("X-Org-ID", uuid.NewString())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)

	response = performTrustedTaskRequest(t, &stubTaskService{}, http.MethodGet, "/not-a-uuid/tasks/", "", stubTeamResolver{})
	assert.Equal(t, http.StatusBadRequest, response.Code)

	teamID := uuid.New()
	response = performTrustedTaskRequest(t, &stubTaskService{}, http.MethodPut, "/"+teamID.String()+"/tasks/not-a-uuid", `{}`, stubTeamResolver{})
	assert.Equal(t, http.StatusBadRequest, response.Code)

	response = performTrustedTaskRequest(t, &stubTaskService{}, http.MethodGet, "/"+teamID.String()+"/tasks/", "", stubTeamResolver{
		resolveTeamFn: func(context.Context, tenancy.OrganizationContext, uuid.UUID) (tenancy.TeamContext, error) {
			return tenancy.TeamContext{}, tenancy.ErrTeamUnavailable
		},
	})
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestTaskRouteReturnsServiceErrorWithoutLeakingIt(t *testing.T) {
	teamID := uuid.New()
	serviceStub := &stubTaskService{getTasksFn: func(context.Context, tenancy.TeamContext) ([]service.Task, error) {
		return nil, errors.New("database secret")
	}}
	response := performTrustedTaskRequest(t, serviceStub, http.MethodGet, "/"+teamID.String()+"/tasks/", "", stubTeamResolver{})
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "database secret")

	var decoded []service.Task
	_ = json.Unmarshal(response.Body.Bytes(), &decoded)
}

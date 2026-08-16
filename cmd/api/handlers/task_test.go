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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTaskService struct {
	getTasksFn   func(context.Context, string, int) ([]service.Task, error)
	createTaskFn func(context.Context, string, int, string) (*service.Task, error)
}

func (s *stubTaskService) GetTasks(ctx context.Context, schema string, teamID int) ([]service.Task, error) {
	if s.getTasksFn == nil {
		panic("unexpected GetTasks call")
	}
	return s.getTasksFn(ctx, schema, teamID)
}

func (s *stubTaskService) CreateTask(ctx context.Context, schema string, teamID int, description string) (*service.Task, error) {
	if s.createTaskFn == nil {
		panic("unexpected CreateTask call")
	}
	return s.createTaskFn(ctx, schema, teamID, description)
}

type stubTaskTeamService struct {
	getTeamIDFn func(context.Context, string, string) (int, error)
}

func (s *stubTaskTeamService) GetTeams(context.Context, string) ([]service.Team, error) {
	panic("unexpected GetTeams call")
}

func (s *stubTaskTeamService) CreateTeam(context.Context, string, string) (*service.Team, error) {
	panic("unexpected CreateTeam call")
}

func (s *stubTaskTeamService) GetTeamID(ctx context.Context, slug, schema string) (int, error) {
	if s.getTeamIDFn == nil {
		panic("unexpected GetTeamID call")
	}
	return s.getTeamIDFn(ctx, slug, schema)
}

func performTaskRequest(
	t *testing.T,
	teamService service.TeamService,
	taskService service.TaskService,
	method, teamSlug, body, orgID string,
) *httptest.ResponseRecorder {
	t.Helper()

	router := chi.NewRouter()
	router.Mount("/{team}/tasks", taskRouter(&Router{
		teamService: teamService,
		taskService: taskService,
	}))

	req := httptest.NewRequest(method, "/"+teamSlug+"/tasks/", strings.NewReader(body))
	if orgID != "" {
		req.Header.Set("X-Org-ID", orgID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func testTask() service.Task {
	return service.Task{
		Description: "Add pagination to the API",
		CreatedAt:   time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, time.August, 15, 11, 0, 0, 0, time.UTC),
	}
}

func TestGetTasks(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")
	const (
		teamSlug = "platform-engineering"
		teamID   = 42
	)

	teamService := func() *stubTaskTeamService {
		return &stubTaskTeamService{
			getTeamIDFn: func(ctx context.Context, slug, schema string) (int, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, teamSlug, slug)
				assert.Equal(t, service.SchemaName(orgID), schema)
				return teamID, nil
			},
		}
	}

	t.Run("returns tasks for the team", func(t *testing.T) {
		want := []service.Task{testTask()}
		taskService := &stubTaskService{
			getTasksFn: func(ctx context.Context, schema string, gotTeamID int) ([]service.Task, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, service.SchemaName(orgID), schema)
				assert.Equal(t, teamID, gotTeamID)
				return want, nil
			},
		}

		response := performTaskRequest(t, teamService(), taskService, http.MethodGet, teamSlug, "", orgID.String())

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got []service.Task
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, want, got)
	})

	t.Run("returns service error", func(t *testing.T) {
		taskService := &stubTaskService{
			getTasksFn: func(context.Context, string, int) ([]service.Task, error) {
				return nil, errors.New("database unavailable")
			},
		}

		response := performTaskRequest(t, teamService(), taskService, http.MethodGet, teamSlug, "", orgID.String())

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to fetch tasks: %q\n", response.Body.String())
	})
}

func TestCreateTask(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")
	const (
		teamSlug = "platform-engineering"
		teamID   = 42
	)

	teamService := func() *stubTaskTeamService {
		return &stubTaskTeamService{
			getTeamIDFn: func(ctx context.Context, slug, schema string) (int, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, teamSlug, slug)
				assert.Equal(t, service.SchemaName(orgID), schema)
				return teamID, nil
			},
		}
	}

	t.Run("creates a task for the team", func(t *testing.T) {
		want := testTask()
		taskService := &stubTaskService{
			createTaskFn: func(ctx context.Context, schema string, gotTeamID int, description string) (*service.Task, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, service.SchemaName(orgID), schema)
				assert.Equal(t, teamID, gotTeamID)
				assert.Equal(t, want.Description, description)
				return &want, nil
			},
		}

		response := performTaskRequest(t, teamService(), taskService, http.MethodPost, teamSlug, `{"description":"Add pagination to the API"}`, orgID.String())

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got service.Task
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, want, got)
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		response := performTaskRequest(t, teamService(), &stubTaskService{}, http.MethodPost, teamSlug, `{`, orgID.String())

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "invalid request body\n", response.Body.String())
	})

	t.Run("returns service error", func(t *testing.T) {
		taskService := &stubTaskService{
			createTaskFn: func(context.Context, string, int, string) (*service.Task, error) {
				return nil, errors.New("create failed")
			},
		}

		response := performTaskRequest(t, teamService(), taskService, http.MethodPost, teamSlug, `{"description":"Add pagination to the API"}`, orgID.String())

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to create task\n", response.Body.String())
	})
}

func TestTaskRoutesRejectInvalidRequestContext(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")

	t.Run("rejects a missing organization header", func(t *testing.T) {
		response := performTaskRequest(t, &stubTaskTeamService{}, &stubTaskService{}, http.MethodGet, "platform-engineering", "", "")

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "organization id header is missing\n", response.Body.String())
	})

	t.Run("rejects an invalid organization ID", func(t *testing.T) {
		response := performTaskRequest(t, &stubTaskTeamService{}, &stubTaskService{}, http.MethodGet, "platform-engineering", "", "not-a-uuid")

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "invalid uuid\n", response.Body.String())
	})

	t.Run("returns a team lookup error", func(t *testing.T) {
		teamService := &stubTaskTeamService{
			getTeamIDFn: func(context.Context, string, string) (int, error) {
				return 0, errors.New("team not found")
			},
		}

		response := performTaskRequest(t, teamService, &stubTaskService{}, http.MethodGet, "unknown-team", "", orgID.String())

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to get teamID\n", response.Body.String())
	})
}

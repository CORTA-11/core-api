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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTeamService struct {
	getTeamsFn   func(context.Context, string) ([]service.Team, error)
	createTeamFn func(context.Context, string, string) (*service.Team, error)
	getTeamIDFn  func(context.Context, string, string) (int, error)
}

func (s *stubTeamService) GetTeams(ctx context.Context, schema string) ([]service.Team, error) {
	if s.getTeamsFn == nil {
		panic("unexpected GetTeams call")
	}
	return s.getTeamsFn(ctx, schema)
}

func (s *stubTeamService) CreateTeam(ctx context.Context, name, schema string) (*service.Team, error) {
	if s.createTeamFn == nil {
		panic("unexpected CreateTeam call")
	}
	return s.createTeamFn(ctx, name, schema)
}

func (s *stubTeamService) GetTeamID(ctx context.Context, slug, schema string) (int, error) {
	if s.getTeamIDFn == nil {
		panic("unexpected GetTeamID call")
	}
	return s.getTeamIDFn(ctx, slug, schema)
}

func performTeamRequest(t *testing.T, teamService service.TeamService, method, target, body, orgID string) *httptest.ResponseRecorder {
	t.Helper()

	router := teamRouter(&Router{teamService: teamService})
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if orgID != "" {
		req.Header.Set("X-Org-ID", orgID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func testTeam() service.Team {
	return service.Team{
		Name:      "Platform Engineering",
		Slug:      "platform-engineering",
		CreatedAt: time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.August, 15, 11, 0, 0, 0, time.UTC),
	}
}

func TestGetTeams(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")

	t.Run("returns teams for the organization", func(t *testing.T) {
		want := []service.Team{testTeam()}
		teamService := &stubTeamService{
			getTeamsFn: func(ctx context.Context, schema string) ([]service.Team, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, service.SchemaName(orgID), schema)
				return want, nil
			},
		}

		response := performTeamRequest(t, teamService, http.MethodGet, "/", "", orgID.String())

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got []service.Team
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, want, got)
	})

	t.Run("returns service error", func(t *testing.T) {
		teamService := &stubTeamService{
			getTeamsFn: func(context.Context, string) ([]service.Team, error) {
				return nil, errors.New("database unavailable")
			},
		}

		response := performTeamRequest(t, teamService, http.MethodGet, "/", "", orgID.String())

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "database unavailable\n", response.Body.String())
	})
}

func TestCreateTeam(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")

	t.Run("creates a team for the organization", func(t *testing.T) {
		want := testTeam()
		teamService := &stubTeamService{
			createTeamFn: func(ctx context.Context, name, schema string) (*service.Team, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, want.Name, name)
				assert.Equal(t, service.SchemaName(orgID), schema)
				return &want, nil
			},
		}

		response := performTeamRequest(t, teamService, http.MethodPost, "/", `{"name":"Platform Engineering"}`, orgID.String())

		assert.Equal(t, http.StatusCreated, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got service.Team
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, want, got)
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		tests := []struct {
			name        string
			body        string
			wantMessage string
		}{
			{name: "malformed JSON", body: `{`, wantMessage: "invalid response body\n"},
			{name: "empty name", body: `{"name":""}`, wantMessage: "team name is required\n"},
			{name: "name too long", body: `{"name":"` + strings.Repeat("a", 256) + `"}`, wantMessage: "team name must be less than 255 characters\n"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				response := performTeamRequest(t, &stubTeamService{}, http.MethodPost, "/", tt.body, orgID.String())

				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Equal(t, tt.wantMessage, response.Body.String())
			})
		}
	})

	t.Run("returns service error", func(t *testing.T) {
		teamService := &stubTeamService{
			createTeamFn: func(context.Context, string, string) (*service.Team, error) {
				return nil, errors.New("create failed")
			},
		}

		response := performTeamRequest(t, teamService, http.MethodPost, "/", `{"name":"Platform Engineering"}`, orgID.String())

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to create team\n", response.Body.String())
	})
}

func TestTeamRoutesRequireValidOrganizationID(t *testing.T) {
	tests := []struct {
		name        string
		orgID       string
		wantMessage string
	}{
		{name: "missing header", wantMessage: "organization id header is missing\n"},
		{name: "invalid UUID", orgID: "not-a-uuid", wantMessage: "invalid uuid\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performTeamRequest(t, &stubTeamService{}, http.MethodGet, "/", "", tt.orgID)

			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Equal(t, tt.wantMessage, response.Body.String())
		})
	}
}

package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
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

type stubOrgService struct {
	getOrgsFn       func(context.Context) ([]service.Organization, error)
	createOrgFn     func(context.Context, string) (*service.Organization, error)
	updateOrgFn     func(context.Context, uuid.UUID, string) (*service.Organization, error)
	softDeleteOrgFn func(context.Context, uuid.UUID) (*service.Organization, error)
	restoreOrgFn    func(context.Context, uuid.UUID) (*service.Organization, error)
}

func (s *stubOrgService) GetOrgs(ctx context.Context) ([]service.Organization, error) {
	if s.getOrgsFn == nil {
		panic("unexpected GetOrgs call")
	}
	return s.getOrgsFn(ctx)
}

func (s *stubOrgService) CreateOrg(ctx context.Context, name string) (*service.Organization, error) {
	if s.createOrgFn == nil {
		panic("unexpected CreateOrg call")
	}
	return s.createOrgFn(ctx, name)
}

func (s *stubOrgService) UpdateOrg(ctx context.Context, publicID uuid.UUID, name string) (*service.Organization, error) {
	if s.updateOrgFn == nil {
		panic("unexpected UpdateOrg call")
	}
	return s.updateOrgFn(ctx, publicID, name)
}

func (s *stubOrgService) SoftDeleteOrg(ctx context.Context, publicID uuid.UUID) (*service.Organization, error) {
	if s.softDeleteOrgFn == nil {
		panic("unexpected SoftDeleteOrg call")
	}
	return s.softDeleteOrgFn(ctx, publicID)
}

func (s *stubOrgService) RestoreOrg(ctx context.Context, publicID uuid.UUID) (*service.Organization, error) {
	if s.restoreOrgFn == nil {
		panic("unexpected RestoreOrg call")
	}
	return s.restoreOrgFn(ctx, publicID)
}

func performOrgRequest(t *testing.T, orgService service.OrgService, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	router := orgRouter(&Router{orgService: orgService})
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func testOrganization() service.Organization {
	return service.Organization{
		PublicID:       uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda"),
		Name:           "University of Aratuwa",
		LifecycleState: "provisioning",
		CreatedAt:      time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, time.August, 15, 11, 0, 0, 0, time.UTC),
	}
}

func TestGetOrgs(t *testing.T) {
	t.Run("returns organizations", func(t *testing.T) {
		want := []service.Organization{testOrganization()}
		orgService := &stubOrgService{
			getOrgsFn: func(ctx context.Context) ([]service.Organization, error) {
				require.NotNil(t, ctx)
				return want, nil
			},
		}

		response := performOrgRequest(t, orgService, http.MethodGet, "/", "")

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got []service.Organization
		require.NoError(t, jsonDecode(response.Body, &got))
		assert.Equal(t, want, got)
	})

	t.Run("returns service error", func(t *testing.T) {
		orgService := &stubOrgService{
			getOrgsFn: func(context.Context) ([]service.Organization, error) {
				return nil, errors.New("database unavailable")
			},
		}

		response := performOrgRequest(t, orgService, http.MethodGet, "/", "")

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to get organizations\n", response.Body.String())
	})
}

func TestCreateOrg(t *testing.T) {
	t.Run("trims the name and creates an organization", func(t *testing.T) {
		want := testOrganization()
		orgService := &stubOrgService{
			createOrgFn: func(ctx context.Context, name string) (*service.Organization, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, want.Name, name)
				return &want, nil
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPost, "/", `{"name":"  University of Aratuwa  "}`)

		assert.Equal(t, http.StatusAccepted, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		assert.NotContains(t, response.Body.String(), "schema_name")
		var got service.Organization
		require.NoError(t, jsonDecode(response.Body, &got))
		assert.Equal(t, want, got)
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		tests := []struct {
			name        string
			body        string
			wantMessage string
		}{
			{name: "malformed JSON", body: `{`, wantMessage: "invalid response body\n"},
			{name: "blank name", body: `{"name":"   "}`, wantMessage: "organization name is required\n"},
			{name: "name too long", body: `{"name":"` + strings.Repeat("a", 256) + `"}`, wantMessage: "organization name must be less than 255 characters\n"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				response := performOrgRequest(t, &stubOrgService{}, http.MethodPost, "/", tt.body)

				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Equal(t, tt.wantMessage, response.Body.String())
			})
		}
	})

	t.Run("returns service error", func(t *testing.T) {
		orgService := &stubOrgService{
			createOrgFn: func(context.Context, string) (*service.Organization, error) {
				return nil, errors.New("create failed")
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPost, "/", `{"name":"MedSync"}`)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to create organization\n", response.Body.String())
	})
}

func TestUpdateOrg(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")

	t.Run("uses the route id and updates the organization", func(t *testing.T) {
		want := testOrganization()
		want.Name = "Aratuwa University"
		orgService := &stubOrgService{
			updateOrgFn: func(ctx context.Context, gotID uuid.UUID, name string) (*service.Organization, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, orgID, gotID)
				assert.Equal(t, want.Name, name)
				return &want, nil
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPut, "/"+orgID.String(), `{"name":"  Aratuwa University  "}`)

		assert.Equal(t, http.StatusOK, response.Code)
		var got service.Organization
		require.NoError(t, jsonDecode(response.Body, &got))
		assert.Equal(t, want, got)
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		tests := []struct {
			name        string
			target      string
			body        string
			wantMessage string
		}{
			{name: "invalid id", target: "/not-a-uuid", body: `{"name":"New name"}`, wantMessage: "invalid organization public_id\n"},
			{name: "malformed JSON", target: "/" + orgID.String(), body: `{`, wantMessage: "invalid response body\n"},
			{name: "blank name", target: "/" + orgID.String(), body: `{"name":" "}`, wantMessage: "organization name is required\n"},
			{name: "name too long", target: "/" + orgID.String(), body: `{"name":"` + strings.Repeat("a", 256) + `"}`, wantMessage: "organization name must be less than 255 characters\n"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				response := performOrgRequest(t, &stubOrgService{}, http.MethodPut, tt.target, tt.body)

				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Equal(t, tt.wantMessage, response.Body.String())
			})
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		orgService := &stubOrgService{
			updateOrgFn: func(context.Context, uuid.UUID, string) (*service.Organization, error) {
				return nil, sql.ErrNoRows
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPut, "/"+orgID.String(), `{"name":"New name"}`)

		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(t, "organization not found\n", response.Body.String())
	})

	t.Run("returns service error", func(t *testing.T) {
		orgService := &stubOrgService{
			updateOrgFn: func(context.Context, uuid.UUID, string) (*service.Organization, error) {
				return nil, errors.New("update failed")
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPut, "/"+orgID.String(), `{"name":"New name"}`)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to update organization\n", response.Body.String())
	})
}

func TestDeleteOrg(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")

	t.Run("soft deletes the organization", func(t *testing.T) {
		want := testOrganization()
		want.DeletedAt = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
		orgService := &stubOrgService{
			softDeleteOrgFn: func(ctx context.Context, gotID uuid.UUID) (*service.Organization, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, orgID, gotID)
				return &want, nil
			},
		}

		response := performOrgRequest(t, orgService, http.MethodDelete, "/"+orgID.String(), "")

		assert.Equal(t, http.StatusOK, response.Code)
		var got service.Organization
		require.NoError(t, jsonDecode(response.Body, &got))
		assert.Equal(t, want, got)
	})

	t.Run("rejects an invalid id", func(t *testing.T) {
		response := performOrgRequest(t, &stubOrgService{}, http.MethodDelete, "/not-a-uuid", "")

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "invalid organization id\n", response.Body.String())
	})

	t.Run("returns not found", func(t *testing.T) {
		orgService := &stubOrgService{
			softDeleteOrgFn: func(context.Context, uuid.UUID) (*service.Organization, error) {
				return nil, sql.ErrNoRows
			},
		}

		response := performOrgRequest(t, orgService, http.MethodDelete, "/"+orgID.String(), "")

		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(t, "organization not found\n", response.Body.String())
	})

	t.Run("returns service error", func(t *testing.T) {
		orgService := &stubOrgService{
			softDeleteOrgFn: func(context.Context, uuid.UUID) (*service.Organization, error) {
				return nil, errors.New("delete failed")
			},
		}

		response := performOrgRequest(t, orgService, http.MethodDelete, "/"+orgID.String(), "")

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to delete organization\n", response.Body.String())
	})
}

func TestRestoreOrg(t *testing.T) {
	orgID := uuid.MustParse("30ee7153-9b48-4560-8cbf-972587a60fda")

	t.Run("restores the organization", func(t *testing.T) {
		want := testOrganization()
		orgService := &stubOrgService{
			restoreOrgFn: func(ctx context.Context, gotID uuid.UUID) (*service.Organization, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, orgID, gotID)
				return &want, nil
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPost, "/restore", `{"public_id":"`+orgID.String()+`"}`)

		assert.Equal(t, http.StatusOK, response.Code)
		var got service.Organization
		require.NoError(t, jsonDecode(response.Body, &got))
		assert.Equal(t, want, got)
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		tests := []struct {
			name        string
			body        string
			wantMessage string
		}{
			{name: "malformed JSON", body: `{`, wantMessage: "invalid request body\n"},
			{name: "invalid id", body: `{"public_id":"not-a-uuid"}`, wantMessage: "invalid organization id\n"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				response := performOrgRequest(t, &stubOrgService{}, http.MethodPost, "/restore", tt.body)

				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Equal(t, tt.wantMessage, response.Body.String())
			})
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		orgService := &stubOrgService{
			restoreOrgFn: func(context.Context, uuid.UUID) (*service.Organization, error) {
				return nil, sql.ErrNoRows
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPost, "/restore", `{"public_id":"`+orgID.String()+`"}`)

		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(t, "organization not found or already active\n", response.Body.String())
	})

	t.Run("returns service error", func(t *testing.T) {
		orgService := &stubOrgService{
			restoreOrgFn: func(context.Context, uuid.UUID) (*service.Organization, error) {
				return nil, errors.New("restore failed")
			},
		}

		response := performOrgRequest(t, orgService, http.MethodPost, "/restore", `{"public_id":"`+orgID.String()+`"}`)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "failed to restore organization\n", response.Body.String())
	})
}

func jsonDecode(body io.Reader, target any) error {
	return json.NewDecoder(body).Decode(target)
}

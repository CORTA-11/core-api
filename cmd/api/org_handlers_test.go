package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/auth"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockOrgStore struct {
	orgs           []string
	getOrgsErr     error
	createOrgRow   repository.CreateOrgRow
	createOrgErr   error
	updateOrgRow   repository.UpdateOrgRow
	updateOrgErr   error
	softDeleteOrg  repository.Org
	softDeleteErr  error
	restoreOrgRow  repository.RestoreOrgRow
	restoreErr     error
	lastCreateName string
	lastUpdateArg  repository.UpdateOrgParams
	lastDeleteID   uuid.UUID
	lastRestoreID  uuid.UUID
}

func (f *mockOrgStore) GetOrgs(ctx context.Context) ([]string, error) {
	return f.orgs, f.getOrgsErr
}

func (f *mockOrgStore) CreateOrg(ctx context.Context, name string) (repository.CreateOrgRow, error) {
	f.lastCreateName = name
	return f.createOrgRow, f.createOrgErr
}

func (f *mockOrgStore) UpdateOrg(ctx context.Context, arg repository.UpdateOrgParams) (repository.UpdateOrgRow, error) {
	f.lastUpdateArg = arg
	return f.updateOrgRow, f.updateOrgErr
}

func (f *mockOrgStore) SoftDeleteOrg(ctx context.Context, publicID uuid.UUID) (repository.Org, error) {
	f.lastDeleteID = publicID
	return f.softDeleteOrg, f.softDeleteErr
}

func (f *mockOrgStore) RestoreOrg(ctx context.Context, publicID uuid.UUID) (repository.RestoreOrgRow, error) {
	f.lastRestoreID = publicID
	return f.restoreOrgRow, f.restoreErr
}

func TestGetOrgs(t *testing.T) {
	store := &mockOrgStore{orgs: []string{"Acme", "Beta"}}
	router := &Router{orgs: store}

	req := httptest.NewRequest(http.MethodGet, "/orgs", nil)
	rec := httptest.NewRecorder()

	router.getOrgs().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response []string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, []string{"Acme", "Beta"}, response)
}

func TestCreateOrg(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 30, 0, 0, time.UTC)
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	store := &mockOrgStore{
		createOrgRow: repository.CreateOrgRow{
			PublicID:  orgID,
			Name:      "Acme",
			CreatedAt: now,
		},
	}
	router := &Router{orgs: store}

	req := httptest.NewRequest(http.MethodPost, "/orgs", bytes.NewBufferString(`{"name":"  Acme  "}`))
	rec := httptest.NewRecorder()

	router.createOrg().ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "Acme", store.lastCreateName)

	var response struct {
		PublicID  string    `json:"public_id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, orgID.String(), response.PublicID)
	assert.Equal(t, "Acme", response.Name)
	assert.True(t, response.CreatedAt.Equal(now))
}

func TestCreateOrgValidationAndErrors(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		store          *mockOrgStore
		wantStatus     int
		wantBody       string
		wantCreateCall bool
	}{
		{
			name:           "invalid json",
			body:           "not-json",
			store:          &mockOrgStore{},
			wantStatus:     http.StatusBadRequest,
			wantBody:       "invalid response body\n",
			wantCreateCall: false,
		},
		{
			name:           "blank name",
			body:           `{"name":"   "}`,
			store:          &mockOrgStore{},
			wantStatus:     http.StatusBadRequest,
			wantBody:       "organization name is required\n",
			wantCreateCall: false,
		},
		{
			name:           "store failure",
			body:           `{"name":"Acme"}`,
			store:          &mockOrgStore{createOrgErr: errors.New("boom")},
			wantStatus:     http.StatusInternalServerError,
			wantBody:       "failed to create organization\n",
			wantCreateCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &Router{orgs: tt.store}
			req := httptest.NewRequest(http.MethodPost, "/orgs", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.createOrg().ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantBody, rec.Body.String())
			if !tt.wantCreateCall {
				assert.Empty(t, tt.store.lastCreateName)
			}
		})
	}
}

func TestUpdateOrg(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 31, 0, 0, time.UTC)
	orgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	store := &mockOrgStore{
		updateOrgRow: repository.UpdateOrgRow{
			PublicID:  orgID,
			Name:      "Renamed",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
	}
	router := &Router{orgs: store}

	req := httptest.NewRequest(http.MethodPut, "/orgs/"+orgID.String(), bytes.NewBufferString(`{"name":"  Renamed  "}`))
	req = withChiURLParam(req, "orgId", orgID.String())
	rec := httptest.NewRecorder()

	router.updateOrg().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, repository.UpdateOrgParams{PublicID: orgID, Name: "Renamed"}, store.lastUpdateArg)

	var response struct {
		PublicID  string    `json:"public_id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, orgID.String(), response.PublicID)
	assert.Equal(t, "Renamed", response.Name)
	assert.True(t, response.UpdatedAt.Equal(now))
}

func TestUpdateOrgValidationAndErrors(t *testing.T) {
	orgID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	tests := []struct {
		name       string
		param      string
		body       string
		store      *mockOrgStore
		wantStatus int
		wantBody   string
	}{
		{
			name:       "invalid uuid",
			param:      "bad-id",
			body:       `{"name":"Acme"}`,
			store:      &mockOrgStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid organization public_id\n",
		},
		{
			name:       "missing org",
			param:      orgID.String(),
			body:       `{"name":"Acme"}`,
			store:      &mockOrgStore{updateOrgErr: sql.ErrNoRows},
			wantStatus: http.StatusNotFound,
			wantBody:   "organization not found\n",
		},
		{
			name:       "store failure",
			param:      orgID.String(),
			body:       `{"name":"Acme"}`,
			store:      &mockOrgStore{updateOrgErr: errors.New("boom")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to update organization\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &Router{orgs: tt.store}
			req := httptest.NewRequest(http.MethodPut, "/orgs/"+tt.param, bytes.NewBufferString(tt.body))
			req = withChiURLParam(req, "orgId", tt.param)
			rec := httptest.NewRecorder()

			router.updateOrg().ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantBody, rec.Body.String())
		})
	}
}

func TestDeleteOrg(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 32, 0, 0, time.UTC)
	deletedAt := pgtype.Timestamptz{Time: now, Valid: true}
	orgID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	store := &mockOrgStore{
		softDeleteOrg: repository.Org{
			PublicID:  orgID,
			Name:      "Acme",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
			DeletedAt: deletedAt,
		},
	}
	router := &Router{orgs: store}

	req := httptest.NewRequest(http.MethodDelete, "/orgs/"+orgID.String(), nil)
	req = withChiURLParam(req, "orgId", orgID.String())
	rec := httptest.NewRecorder()

	router.deleteOrg().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, orgID, store.lastDeleteID)

	var response struct {
		PublicID  string     `json:"public_id"`
		Name      string     `json:"name"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
		DeletedAt *time.Time `json:"deleted_at"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.NotNil(t, response.DeletedAt)
	assert.Equal(t, orgID.String(), response.PublicID)
	assert.Equal(t, "Acme", response.Name)
	assert.True(t, response.DeletedAt.Equal(now))
}

func TestRestoreOrg(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 33, 0, 0, time.UTC)
	orgID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	store := &mockOrgStore{
		restoreOrgRow: repository.RestoreOrgRow{
			PublicID:  orgID,
			Name:      "Acme",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
	}
	router := &Router{orgs: store}

	req := httptest.NewRequest(http.MethodPost, "/orgs/restore", bytes.NewBufferString(`{"public_id":"`+orgID.String()+`"}`))
	rec := httptest.NewRecorder()

	router.restoreOrg().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, orgID, store.lastRestoreID)

	var response struct {
		PublicID  string     `json:"public_id"`
		Name      string     `json:"name"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
		DeletedAt *time.Time `json:"deleted_at"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, orgID.String(), response.PublicID)
	assert.Equal(t, "Acme", response.Name)
	assert.Nil(t, response.DeletedAt)
}

func TestRestoreOrgValidationAndErrors(t *testing.T) {
	orgID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	tests := []struct {
		name       string
		body       string
		store      *mockOrgStore
		wantStatus int
		wantBody   string
	}{
		{
			name:       "invalid json",
			body:       "not-json",
			store:      &mockOrgStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid request body\n",
		},
		{
			name:       "invalid uuid",
			body:       `{"public_id":"bad-id"}`,
			store:      &mockOrgStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid organization id\n",
		},
		{
			name:       "already active",
			body:       `{"public_id":"` + orgID.String() + `"}`,
			store:      &mockOrgStore{restoreErr: sql.ErrNoRows},
			wantStatus: http.StatusNotFound,
			wantBody:   "organization not found or already active\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &Router{orgs: tt.store}
			req := httptest.NewRequest(http.MethodPost, "/orgs/restore", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.restoreOrg().ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantBody, rec.Body.String())
		})
	}
}

func TestOrgRoutesSmoke(t *testing.T) {
	prevSecret := os.Getenv("JWT_SECRET")
	t.Setenv("JWT_SECRET", "route-smoke-secret")
	t.Cleanup(func() {
		if prevSecret == "" {
			return
		}
		_ = os.Setenv("JWT_SECRET", prevSecret)
	})

	orgID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	now := time.Date(2026, time.August, 12, 10, 34, 0, 0, time.UTC)
	store := &mockOrgStore{
		orgs: []string{"Acme"},
		createOrgRow: repository.CreateOrgRow{
			PublicID:  orgID,
			Name:      "Acme",
			CreatedAt: now,
		},
		updateOrgRow: repository.UpdateOrgRow{
			PublicID:  orgID,
			Name:      "Renamed",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
		softDeleteOrg: repository.Org{
			PublicID:  orgID,
			Name:      "Acme",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
			DeletedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		restoreOrgRow: repository.RestoreOrgRow{
			PublicID:  orgID,
			Name:      "Acme",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
	}

	router := &Router{mux: chi.NewRouter(), orgs: store}
	router.SetupRoutes()

	token, err := auth.GenerateToken(100, 200, "ORG_USER")
	require.NoError(t, err)

	requestWithAuth := func(method, target, body string) *http.Request {
		req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	t.Run("get", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, requestWithAuth(http.MethodGet, "/orgs/", ""))

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `["Acme"]`, rec.Body.String())
	})

	t.Run("create", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, requestWithAuth(http.MethodPost, "/orgs/", `{"name":"Acme"}`))

		require.Equal(t, http.StatusCreated, rec.Code)
		assert.Equal(t, "Acme", store.lastCreateName)
	})

	t.Run("update", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, requestWithAuth(http.MethodPut, "/orgs/"+orgID.String()+"/", `{"name":"Renamed"}`))

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, repository.UpdateOrgParams{PublicID: orgID, Name: "Renamed"}, store.lastUpdateArg)
	})

	t.Run("delete", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, requestWithAuth(http.MethodDelete, "/orgs/"+orgID.String()+"/", ""))

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, orgID, store.lastDeleteID)
	})

	t.Run("restore", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, requestWithAuth(http.MethodPost, "/orgs/restore", `{"public_id":"`+orgID.String()+`"}`))

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, orgID, store.lastRestoreID)
	})
}

func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

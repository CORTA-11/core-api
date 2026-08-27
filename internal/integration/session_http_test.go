//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/CORTA-11/core-api/cmd/api/handlers/v1"
	"github.com/CORTA-11/core-api/internal/apicontract"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedCredentialVerifier struct {
	userID uuid.UUID
}

func TestDarkAuthRouterAllOperationsConformToOpenAPI(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	hasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	require.NoError(t, err)
	currentPassword := "current-password-value"
	userID := createPasswordSessionTestUser(t, pool, hasher, currentPassword)
	manager, err := session.NewManager(pool, bytes.Repeat([]byte{0x6d}, 32))
	require.NoError(t, err)
	router := v1.NewAuthRouter(manager, fixedCredentialVerifier{userID: userID}, hasher,
		"test", []string{"https://app.example"})
	document, err := apicontract.Load(context.Background(),
		testsupport.RepositoryRoot()+"/api/openapi.yaml")
	require.NoError(t, err)
	validator, err := apicontract.NewValidator(document)
	require.NoError(t, err)

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	missingResponse := serveConformant(t, validator, router, missing, true)
	assert.Equal(t, http.StatusUnauthorized, missingResponse.Code)

	malformed := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"submitted-secret","extra":true}`))
	malformed.Header.Set("Content-Type", "application/json")
	malformedResponse := serveConformant(t, validator, router, malformed, false)
	assert.Equal(t, http.StatusBadRequest, malformedResponse.Code)
	assert.NotContains(t, malformedResponse.Body.String(), "submitted-secret")

	firstResponse := loginConformant(t, validator, router)
	firstCookie, firstCSRF, _ := authResponseValues(t, firstResponse)

	current := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	current.AddCookie(firstCookie)
	assert.Equal(t, http.StatusOK, serveConformant(t, validator, router, current, true).Code)

	list := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	list.AddCookie(firstCookie)
	assert.Equal(t, http.StatusOK, serveConformant(t, validator, router, list, true).Code)

	secondResponse := loginConformant(t, validator, router)
	_, _, secondSessionID := authResponseValues(t, secondResponse)
	revokeSpecific := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/"+secondSessionID, nil)
	revokeSpecific.AddCookie(firstCookie)
	revokeSpecific.Header.Set("Origin", "https://app.example")
	revokeSpecific.Header.Set("X-CSRF-Token", firstCSRF)
	assert.Equal(t, http.StatusNoContent, serveConformant(t, validator, router, revokeSpecific, true).Code)

	change := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(
		`{"current_password":"current-password-value","new_password":"replacement-password-value"}`))
	change.Header.Set("Content-Type", "application/json")
	change.Header.Set("Origin", "https://app.example")
	change.Header.Set("X-CSRF-Token", firstCSRF)
	change.AddCookie(firstCookie)
	changeResponse := serveConformant(t, validator, router, change, true)
	require.Equal(t, http.StatusOK, changeResponse.Code)
	replacementCookie, replacementCSRF, _ := authResponseValues(t, changeResponse)

	revokeAll := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions", nil)
	revokeAll.AddCookie(replacementCookie)
	revokeAll.Header.Set("Origin", "https://app.example")
	revokeAll.Header.Set("X-CSRF-Token", replacementCSRF)
	assert.Equal(t, http.StatusNoContent, serveConformant(t, validator, router, revokeAll, true).Code)

	thirdResponse := loginConformant(t, validator, router)
	thirdCookie, thirdCSRF, _ := authResponseValues(t, thirdResponse)
	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	logout.AddCookie(thirdCookie)
	logout.Header.Set("Origin", "https://app.example")
	logout.Header.Set("X-CSRF-Token", thirdCSRF)
	assert.Equal(t, http.StatusNoContent, serveConformant(t, validator, router, logout, true).Code)
}

func loginConformant(
	t *testing.T,
	validator *apicontract.Validator,
	router http.Handler,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewBufferString(`{"email":"password-session@example.com","password":"login-value"}`))
	request.Header.Set("Content-Type", "application/json")
	return serveConformant(t, validator, router, request, true)
}

func authResponseValues(t *testing.T, response *httptest.ResponseRecorder) (*http.Cookie, string, string) {
	t.Helper()
	require.Equal(t, http.StatusOK, response.Code)
	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	var body struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		CSRFToken string `json:"csrf_token"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotEmpty(t, body.Session.ID)
	require.NotEmpty(t, body.CSRFToken)
	return cookies[0], body.CSRFToken, body.Session.ID
}

func serveConformant(
	t *testing.T,
	validator *apicontract.Validator,
	router http.Handler,
	request *http.Request,
	validateRequest bool,
) *httptest.ResponseRecorder {
	t.Helper()
	operation, err := validator.Match(request)
	require.NoError(t, err)
	require.NotEmpty(t, operation.OperationID())
	if validateRequest {
		require.NoErrorf(t, operation.ValidateRequest(request.Context()), "operation %s", operation.OperationID())
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.NoError(t, operation.ValidateResponse(request.Context(), recorder.Code,
		recorder.Header(), recorder.Body.Bytes()))
	return recorder
}

func (verifier fixedCredentialVerifier) Verify(context.Context, string, string) (identity.CredentialPrincipal, error) {
	return identity.CredentialPrincipal{UserPublicID: verifier.userID}, nil
}

func TestDarkAuthRouterCookieCSRFAndIdempotentLogout(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	userID := createSessionTestUser(t, pool, "browser-session@example.com")
	now := time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC)
	manager, err := session.NewManager(pool, bytes.Repeat([]byte{0x3d}, 32),
		session.WithClock(func() time.Time { return now }),
		session.WithRandom(bytes.NewReader(bytes.Repeat([]byte{0x73}, 32))))
	require.NoError(t, err)
	router := v1.NewAuthRouter(manager, fixedCredentialVerifier{userID: userID}, nil,
		"test", []string{"https://app.example"})

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewBufferString(`{"email":"browser-session@example.com","password":"password-value"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, login)
	require.Equal(t, http.StatusOK, loginResponse.Code)
	cookies := loginResponse.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "synodus_dev_session", cookies[0].Name)
	assert.True(t, cookies[0].HttpOnly)
	assert.False(t, cookies[0].Secure)

	var loginBody map[string]any
	require.NoError(t, json.Unmarshal(loginResponse.Body.Bytes(), &loginBody))
	assert.ElementsMatch(t, []string{"user", "session", "csrf_token"}, mapKeys(loginBody))
	csrfToken, ok := loginBody["csrf_token"].(string)
	require.True(t, ok)

	current := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	current.AddCookie(cookies[0])
	currentResponse := httptest.NewRecorder()
	router.ServeHTTP(currentResponse, current)
	assert.Equal(t, http.StatusOK, currentResponse.Code)

	missingOrigin := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	missingOrigin.AddCookie(cookies[0])
	missingOrigin.Header.Set("X-CSRF-Token", csrfToken)
	missingOriginResponse := httptest.NewRecorder()
	router.ServeHTTP(missingOriginResponse, missingOrigin)
	assert.Equal(t, http.StatusForbidden, missingOriginResponse.Code)

	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	logout.AddCookie(cookies[0])
	logout.Header.Set("Origin", "https://app.example")
	logout.Header.Set("X-CSRF-Token", csrfToken)
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logout)
	assert.Equal(t, http.StatusNoContent, logoutResponse.Code)
	require.Len(t, logoutResponse.Result().Cookies(), 1)
	assert.Less(t, logoutResponse.Result().Cookies()[0].MaxAge, 0)

	staleLogoutResponse := httptest.NewRecorder()
	router.ServeHTTP(staleLogoutResponse, logout.Clone(context.Background()))
	assert.Equal(t, http.StatusNoContent, staleLogoutResponse.Code)

	malformed := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	malformed.AddCookie(&http.Cookie{Name: "synodus_dev_session", Value: "malformed"})
	malformedResponse := httptest.NewRecorder()
	router.ServeHTTP(malformedResponse, malformed)
	assert.Equal(t, http.StatusNoContent, malformedResponse.Code)
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

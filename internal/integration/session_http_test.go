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

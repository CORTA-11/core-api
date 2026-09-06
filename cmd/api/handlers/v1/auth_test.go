package v1

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/ratelimit"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type verifierStub struct{ err error }

func (stub verifierStub) Verify(context.Context, string, string) (identity.CredentialPrincipal, error) {
	return identity.CredentialPrincipal{}, stub.err
}

type hasherStub struct{ err error }

func (stub hasherStub) Hash(context.Context, string) (string, error) { return "hash", stub.err }
func (hasherStub) Verify(context.Context, string, string) (identity.PasswordVerification, error) {
	return identity.PasswordVerification{}, nil
}

type rateLimiterStub struct {
	err     error
	calls   int
	consume []bool
}

func (stub *rateLimiterStub) Check(_ context.Context, _ ratelimit.Policy, _ string, consume bool) (ratelimit.Decision, error) {
	stub.calls++
	stub.consume = append(stub.consume, consume)
	return ratelimit.Decision{Allowed: stub.err == nil, RetryAfter: time.Second}, stub.err
}

func (*rateLimiterStub) Clear(context.Context, ratelimit.Policy, string) error { return nil }

func rateLimitedRouter(t *testing.T, limiter ratelimit.Limiter, verifier identity.CredentialVerifier) http.Handler {
	t.Helper()
	guard, err := ratelimit.NewLoginGuard(limiter, ratelimit.DefaultPolicies())
	require.NoError(t, err)
	trusted, err := httpx.ParseTrustedProxies("")
	require.NoError(t, err)
	return trusted.Middleware(NewRateLimitedAuthRouter(nil, verifier, nil, "test", nil, guard))
}

func TestLoginRateLimitConsumesIPBeforeDecodeAndCountsOnlyInvalidCredentials(t *testing.T) {
	limiter := &rateLimiterStub{}
	router := rateLimitedRouter(t, limiter, verifierStub{err: identity.ErrInvalidCredentials})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, 1, limiter.calls)

	request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"invalid password"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Equal(t, []bool{true, true, false, true}, limiter.consume)
}

func TestLoginRateLimitDependencyFailureStopsBeforeVerification(t *testing.T) {
	limiter := &rateLimiterStub{err: errors.New("redis unavailable")}
	router := rateLimitedRouter(t, limiter, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"password"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "redis unavailable")
}

func TestRegistrationRateLimitRunsBeforeDecodeAndFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"admitted malformed body", nil, http.StatusBadRequest},
		{"Redis unavailable", errors.New("redis unavailable"), http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			limiter := &rateLimiterStub{err: test.err}
			guard, err := ratelimit.NewRegistrationGuard(limiter, ratelimit.DefaultPolicies().RegistrationIP)
			require.NoError(t, err)
			trusted, err := httpx.ParseTrustedProxies("")
			require.NoError(t, err)
			router := NewRouter(RouterConfig{Environment: "test", TrustedProxies: trusted, RegistrationGuard: guard})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.Handler().ServeHTTP(response, request)
			assert.Equal(t, test.status, response.Code)
			assert.Equal(t, 1, limiter.calls)
			assert.NotContains(t, response.Body.String(), "redis unavailable")
		})
	}
}

func TestRegistrationReturnsSafeFieldViolationsBeforeHashing(t *testing.T) {
	secret := strings.Repeat("a", 11)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(
		`{"display_name":"","email":"","password":"`+secret+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewAuthRouter(nil, nil, hasherStub{err: errors.New("must not hash")}, "test", nil).ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), `"field":"display_name"`)
	assert.Contains(t, response.Body.String(), `"field":"email"`)
	assert.Contains(t, response.Body.String(), `"field":"password"`)
	assert.NotContains(t, response.Body.String(), secret)
	assert.NotContains(t, response.Body.String(), "must not hash")
}

func TestRegistrationHashFailureIsClosedDependencyProblem(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(
		`{"display_name":"Researcher","email":"user@example.com","password":"valid-password-value"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewAuthRouter(nil, nil, hasherStub{err: errors.New("hashing secret")}, "test", nil).ServeHTTP(response, request)
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.NotContains(t, response.Body.String(), "hashing secret")
	assert.NotContains(t, response.Body.String(), "valid-password-value")
}

func TestRegistrationCreatesSessionCookieAndAuthResponse(t *testing.T) {
	pool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer pool.Close()
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	userID, sessionID := uuid.New(), uuid.New()
	pool.ExpectBegin()
	pool.ExpectQuery("(?s)CreateUser :one.*INSERT").
		WithArgs("User@Example.com", "hash", "Researcher", identity.PasswordNormalizationNFCV1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "email", "password_hash", "display_name", "created_at", "updated_at",
			"deleted_at", "email_canonical", "password_normalization",
		}).AddRow(int64(1), userID, "User@Example.com", "hash", "Researcher", now, now,
			pgtype.Timestamptz{}, "user@example.com", identity.PasswordNormalizationNFCV1))
	pool.ExpectQuery("(?s)CreateSession :one.*INSERT").
		WithArgs(pgxmock.AnyArg(), "Browser", now, now.Add(session.AbsoluteLifetime), userID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "session_id", "user_id", "token_hash", "user_agent", "created_at",
			"last_seen_at", "absolute_expires_at", "revoked_at",
		}).AddRow(int64(2), sessionID, int64(1), bytes.Repeat([]byte{7}, 32), "Browser", now, now,
			now.Add(session.AbsoluteLifetime), pgtype.Timestamptz{}))
	pool.ExpectQuery("(?s)GetUserByID :one.*SELECT").WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "email", "password_hash", "display_name", "created_at", "updated_at",
			"deleted_at", "email_canonical", "password_normalization",
		}).AddRow(int64(1), userID, "User@Example.com", "hash", "Researcher", now, now,
			pgtype.Timestamptz{}, "user@example.com", identity.PasswordNormalizationNFCV1))
	pool.ExpectCommit()
	manager, err := session.NewManager(pool, bytes.Repeat([]byte{8}, 32),
		session.WithClock(func() time.Time { return now }),
		session.WithRandom(bytes.NewReader(bytes.Repeat([]byte{9}, 32))))
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(
		`{"display_name":" Researcher ","email":" User@Example.com ","password":"valid-password-value"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Browser")
	response := httptest.NewRecorder()
	NewAuthRouter(manager, nil, hasherStub{}, "test", nil).ServeHTTP(response, request)
	assert.Equal(t, http.StatusCreated, response.Code)
	assert.Contains(t, response.Header().Get("Set-Cookie"), session.CookiePolicy("test").Name+"=")
	assert.Contains(t, response.Header().Get("Set-Cookie"), "HttpOnly")
	assert.Contains(t, response.Body.String(), `"csrf_token":`)
	assert.Contains(t, response.Body.String(), userID.String())
	assert.NotContains(t, response.Body.String(), "valid-password-value")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestAuthRouterBoundariesUseClosedProblems(t *testing.T) {
	tests := []struct {
		name        string
		router      http.Handler
		method      string
		path        string
		contentType string
		body        string
		status      int
		problemType string
	}{
		{"missing route", NewAuthRouter(nil, nil, nil, "test", nil), http.MethodGet, "/api/v1/auth/missing", "", "", 404, "/problems/not-found"},
		{"wrong method", NewAuthRouter(nil, nil, nil, "test", nil), http.MethodPatch, "/api/v1/auth/session", "", "", 404, "/problems/not-found"},
		{"missing media type", NewAuthRouter(nil, nil, nil, "test", nil), http.MethodPost, "/api/v1/auth/login", "", `{}`, 400, "/problems/invalid-request"},
		{"unknown field", NewAuthRouter(nil, nil, nil, "test", nil), http.MethodPost, "/api/v1/auth/login", "application/json", `{"email":"a@example.com","password":"safe","submitted_secret":"never echo"}`, 400, "/problems/invalid-request"},
		{"invalid credentials", NewAuthRouter(nil, verifierStub{err: identity.ErrInvalidCredentials}, nil, "test", nil), http.MethodPost, "/api/v1/auth/login", "application/json", `{"email":"a@example.com","password":"never echo"}`, 401, "/problems/unauthenticated"},
		{"dependency", NewAuthRouter(nil, verifierStub{err: errors.New("database secret")}, nil, "test", nil), http.MethodPost, "/api/v1/auth/login", "application/json", `{"email":"a@example.com","password":"never echo"}`, 503, "/problems/dependency-unavailable"},
		{"missing session", NewAuthRouter(nil, nil, nil, "test", nil), http.MethodGet, "/api/v1/auth/session", "", "", 401, "/problems/unauthenticated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			recorder := httptest.NewRecorder()
			test.router.ServeHTTP(recorder, request)
			assert.Equal(t, test.status, recorder.Code)
			assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))
			assert.Contains(t, recorder.Body.String(), `"type":"`+test.problemType+`"`)
			assert.NotContains(t, recorder.Body.String(), "never echo")
			assert.NotContains(t, recorder.Body.String(), "database secret")
			require.NotEmpty(t, recorder.Header().Get("X-Request-ID"))
		})
	}
}

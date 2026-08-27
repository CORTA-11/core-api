package v1

import (
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type verifierStub struct{ err error }

func (stub verifierStub) Verify(context.Context, string, string) (identity.CredentialPrincipal, error) {
	return identity.CredentialPrincipal{}, stub.err
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

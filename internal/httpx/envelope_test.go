package httpx

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCORSGrantsOnlyExactValidCredentialedRequests(t *testing.T) {
	policy, err := ParseOriginPolicy("https://app.example.com", "production")
	require.NoError(t, err)
	handler := SecurityHeaders("production", false, CORS(policy, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = WriteJSON(writer, http.StatusOK, struct{}{})
	})))

	valid := httptest.NewRequest(http.MethodOptions, "/api/v1/orgs", nil)
	valid.Header.Set("Origin", "https://app.example.com")
	valid.Header.Set("Access-Control-Request-Method", http.MethodPost)
	valid.Header.Set("Access-Control-Request-Headers", "Content-Type, X-CSRF-Token, If-Match, Idempotency-Key")
	valid = valid.WithContext(context.WithValue(valid.Context(), clientContextKey{}, Client{Scheme: "https"}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, valid)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "https://app.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "GET, POST, PUT, PATCH, DELETE, OPTIONS", recorder.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, X-CSRF-Token, If-Match, Idempotency-Key", recorder.Header().Get("Access-Control-Allow-Headers"))
	for _, vary := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
		assert.Contains(t, recorder.Header().Values("Vary"), vary)
	}
	assert.Equal(t, "max-age=31536000", recorder.Header().Get("Strict-Transport-Security"))

	for _, mutate := range []func(*http.Request){
		func(request *http.Request) { request.Header.Set("Origin", "https://app.example.com.evil") },
		func(request *http.Request) { request.Header.Set("Access-Control-Request-Method", "TRACE") },
		func(request *http.Request) { request.Header.Set("Access-Control-Request-Headers", "Authorization") },
		func(request *http.Request) { request.Header.Del("Access-Control-Request-Method") },
	} {
		request := valid.Clone(valid.Context())
		request.Header = valid.Header.Clone()
		mutate(request)
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Less(t, recorder.Body.Len(), 1024)
	}
}

func TestSecurityHeadersAndNoStorePolicy(t *testing.T) {
	handler := SecurityHeaders("test", true, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = WriteJSON(writer, http.StatusOK, struct{}{})
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
	assert.Equal(t, "camera=(), microphone=(), geolocation=()", recorder.Header().Get("Permissions-Policy"))
	assert.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", recorder.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	assert.Empty(t, recorder.Header().Get("Strict-Transport-Security"))

	problemRecorder := httptest.NewRecorder()
	require.NoError(t, WriteProblem(problemRecorder, httptest.NewRequest(http.MethodGet, "/", nil), NewError(ProblemForbidden, nil)))
	assert.Equal(t, "no-store", problemRecorder.Header().Get("Cache-Control"))
}

func TestBoundaryLogIsOneBoundedRedactedEventIncludingPanic(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	policy, err := ParseTrustedProxies("")
	require.NoError(t, err)
	userID := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	sessionID := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler { return BoundaryLog(logger, next) })
	router.Use(Recover)
	router.Get("/objects/{object_id}", func(http.ResponseWriter, *http.Request) {
		panic("panic-secret@example.com database-secret")
	})
	handler := RequestID(policy.Middleware(router))
	request := httptest.NewRequest(http.MethodGet, "/objects/private-key?token=query-secret", strings.NewReader("body-secret"))
	request.RemoteAddr = "192.0.2.129:4444"
	request.Header.Set("Authorization", "Bearer credential-secret")
	request.Header.Set("User-Agent", "agent-secret")
	request = request.WithContext(session.ContextWithPrincipal(request.Context(), session.Principal{UserID: userID, SessionID: sessionID}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 1)
	logOutput := lines[0]
	for _, secret := range []string{"private-key", "query-secret", "body-secret", "credential-secret", "agent-secret", "panic-secret", "database-secret", "192.0.2.129"} {
		assert.NotContains(t, logOutput, secret)
	}
	for _, safe := range []string{"request_boundary", "192.0.2.0/24", "GET", "/objects/{object_id}", userID.String(), sessionID.String(), "internal-failure"} {
		assert.Contains(t, logOutput, safe)
	}
	assert.Less(t, len(logOutput), 2048)
}

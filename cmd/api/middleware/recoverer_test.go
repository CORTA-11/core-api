package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverer(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	handler := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom secret@example.com database-secret")
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Body.String(), `"type":"/problems/internal-failure"`)
	assert.NotContains(t, recorder.Body.String(), "secret")

	logOutput := logs.String()
	assert.Empty(t, logOutput)
}

func TestRecovererPassesThroughSuccessfulResponses(t *testing.T) {
	handler := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}

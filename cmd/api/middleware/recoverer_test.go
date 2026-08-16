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
		panic("boom")
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "Internal Server Error\n", recorder.Body.String())

	logOutput := logs.String()
	assert.Contains(t, logOutput, `msg="panic recovered"`)
	assert.Contains(t, logOutput, "panic=boom")
	assert.Contains(t, logOutput, "method=GET")
	assert.Contains(t, logOutput, "path=/panic")
	assert.Contains(t, logOutput, "stack=")
	assert.Contains(t, logOutput, "TestRecoverer")
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

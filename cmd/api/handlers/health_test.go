package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestHealthLive(t *testing.T) {
	recorder := httptest.NewRecorder()
	healthLive().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestPprofIsNeverMountedOnPublicRouter(t *testing.T) {
	router := &Router{mux: chi.NewRouter(), readinessTimeout: time.Second}
	router.SetupRoutes()
	recorder := httptest.NewRecorder()
	router.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestHealthReadyReportsSortedFailures(t *testing.T) {
	checks := map[string]ReadinessCheck{
		"redis":    func(context.Context) error { return errors.New("unavailable") },
		"postgres": func(context.Context) error { return errors.New("unavailable") },
		"minio":    func(context.Context) error { return nil },
	}
	recorder := httptest.NewRecorder()
	healthReady(checks, time.Second).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.JSONEq(t, `{"failed":["postgres","redis"]}`, recorder.Body.String())
}

func TestHealthReadyTransitionsToReady(t *testing.T) {
	failing := true
	handler := healthReady(map[string]ReadinessCheck{"postgres": func(context.Context) error {
		if failing {
			return errors.New("unavailable")
		}
		return nil
	}}, time.Second)
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	assert.Equal(t, http.StatusServiceUnavailable, first.Code)
	failing = false
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	assert.Equal(t, http.StatusNoContent, second.Code)
}

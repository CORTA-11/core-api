package v1

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/CORTA-11/core-api/internal/apicontract"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterMountsOnlyReviewedV1AndHealthOperations(t *testing.T) {
	t.Parallel()
	router := NewRouter(RouterConfig{Environment: "test"})
	actual := make([]string, 0, len(apicontract.Routes)+2)
	require.NoError(t, chi.Walk(router.mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		actual = append(actual, method+" "+route)
		return nil
	}))
	expected := []string{"GET /health/live", "GET /health/ready"}
	for _, route := range apicontract.Routes {
		expected = append(expected, route.Method+" "+route.Pattern)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	assert.Equal(t, expected, actual)
}

func TestRouterRejectsLegacySurfacesAndAppliesGlobalEnvelope(t *testing.T) {
	t.Parallel()
	router := NewRouter(RouterConfig{Environment: "test"})
	for _, path := range []string{"/", "/users", "/orgs", "/teams", "/debug/pprof/", "/api/v1/files"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set("Authorization", "Bearer legacy.jwt.value")
		request.Header.Set("X-Org-ID", "11111111-1111-4111-8111-111111111111")
		response := httptest.NewRecorder()
		router.Handler().ServeHTTP(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code, path)
		assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"), path)
		assert.NotEmpty(t, response.Header().Get(httpx.RequestIDHeader), path)
		assert.Less(t, response.Body.Len(), 1024, path)
	}
}

func TestProtectedRouteFailsBeforeServiceWithoutSession(t *testing.T) {
	t.Parallel()
	router := NewRouter(RouterConfig{Environment: "test"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/orgs", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	router.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
}

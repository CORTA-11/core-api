package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type changingKeyAccess struct {
	service.KeyAccessRequestService
	calls         int
	statuses      []string
	failAt        int
	principal     session.Principal
	orgID, teamID uuid.UUID
}

func (stub *changingKeyAccess) ListKeyAccessRequests(_ context.Context, principal session.Principal, orgID, teamID uuid.UUID) ([]service.KeyAccessRequestView, error) {
	stub.calls++
	stub.principal, stub.orgID, stub.teamID = principal, orgID, teamID
	if stub.calls == stub.failAt {
		return nil, authorization.ErrResourceNotFound
	}
	return []service.KeyAccessRequestView{{Status: stub.statuses[stub.calls-1]}}, nil
}

func keyAccessEventRequest() *http.Request {
	request := authenticatedResourceRequest(http.MethodGet, "/api/v1/orgs/10000000-0000-0000-0000-000000000001/teams/20000000-0000-0000-0000-000000000002/key-access-requests/events")
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("org_id", "10000000-0000-0000-0000-000000000001")
	routeContext.URLParams.Add("team_id", "20000000-0000-0000-0000-000000000002")
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func TestKeyAccessEventsStreamsInitialAndChangedStatus(t *testing.T) {
	for _, status := range []string{"granted", "denied"} {
		t.Run(status, func(t *testing.T) {
			stub := &changingKeyAccess{statuses: []string{"pending", status}}
			handler := &ResourceHandler{keyAccess: stub}
			request := keyAccessEventRequest()
			ctx, cancel := context.WithCancel(request.Context())
			defer cancel()
			writer := &cancelOnFlush{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
			httpx.Recover(http.HandlerFunc(handler.keyAccessEvents)).ServeHTTP(writer, request.WithContext(ctx))
			require.Equal(t, http.StatusOK, writer.Code)
			assert.Equal(t, "text/event-stream", writer.Header().Get("Content-Type"))
			assert.Equal(t, "no", writer.Header().Get("X-Accel-Buffering"))
			assert.Equal(t, 2, writer.flushes)
			assert.Contains(t, writer.Body.String(), "event: key-access-requests\ndata: [")
			assert.Contains(t, writer.Body.String(), `"status":"pending"`)
			assert.Contains(t, writer.Body.String(), `"status":"`+status+`"`)
			authentication, _ := authenticationFrom(request)
			assert.Equal(t, authentication.Principal, stub.principal)
			assert.Equal(t, chi.URLParam(request, "org_id"), stub.orgID.String())
			assert.Equal(t, chi.URLParam(request, "team_id"), stub.teamID.String())
		})
	}
}

func TestKeyAccessEventsRevocationClosesStream(t *testing.T) {
	stub := &changingKeyAccess{statuses: []string{"pending"}, failAt: 2}
	handler := &ResourceHandler{keyAccess: stub}
	writer := httptest.NewRecorder()
	handler.keyAccessEvents(writer, keyAccessEventRequest())
	assert.Equal(t, 2, stub.calls)
	assert.Equal(t, 1, strings.Count(writer.Body.String(), "event: key-access-requests"))
}

func TestKeyAccessEventsRejectsNonmemberBeforeStreaming(t *testing.T) {
	handler := &ResourceHandler{keyAccess: &changingKeyAccess{failAt: 1}}
	writer := httptest.NewRecorder()
	handler.keyAccessEvents(writer, keyAccessEventRequest())
	assert.Equal(t, http.StatusNotFound, writer.Code)
	assert.False(t, writer.Flushed)
}

func TestKeyAccessEventsRequiresSession(t *testing.T) {
	router := NewRouter(RouterConfig{})
	writer := httptest.NewRecorder()
	request := keyAccessEventRequest()
	router.Handler().ServeHTTP(writer, httptest.NewRequest(http.MethodGet, request.URL.Path, nil))
	assert.NotEqual(t, http.StatusOK, writer.Code)
	assert.NotEqual(t, http.StatusNotFound, writer.Code)
}

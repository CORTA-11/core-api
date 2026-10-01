package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type changingContentAccess struct {
	service.ContentAccessService
	calls         int
	statuses      []string
	failAt        int
	principal     session.Principal
	orgID, teamID uuid.UUID
}

func (s *changingContentAccess) List(_ context.Context, p session.Principal, org, team uuid.UUID) (service.ContentAccessSnapshot, error) {
	s.calls++
	s.principal, s.orgID, s.teamID = p, org, team
	if s.calls == s.failAt {
		return service.ContentAccessSnapshot{}, authorization.ErrResourceNotFound
	}
	return service.ContentAccessSnapshot{Items: []tenantdb.ListContentAccessRow{}, Requests: []tenantdb.ListContentAccessRequestsRow{{Status: s.statuses[s.calls-1]}}}, nil
}

func TestContentAccessEventsDeliverInitialAndDecisionSnapshots(t *testing.T) {
	for _, status := range []string{"granted", "denied"} {
		t.Run(status, func(t *testing.T) {
			stub := &changingContentAccess{statuses: []string{"pending", status}}
			handler := &ResourceHandler{contentAccess: stub}
			request := keyAccessEventRequest()
			ctx, cancel := context.WithCancel(request.Context())
			defer cancel()
			writer := &cancelOnFlush{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
			handler.contentAccessEvents(writer, request.WithContext(ctx))
			require.Equal(t, http.StatusOK, writer.Code)
			assert.Equal(t, "text/event-stream", writer.Header().Get("Content-Type"))
			assert.Equal(t, "no", writer.Header().Get("X-Accel-Buffering"))
			assert.Equal(t, 2, writer.flushes)
			assert.Contains(t, writer.Body.String(), "event: content-access\ndata: {")
			assert.Contains(t, writer.Body.String(), `"status":"pending"`)
			assert.Contains(t, writer.Body.String(), `"status":"`+status+`"`)
			authentication, _ := authenticationFrom(request)
			assert.Equal(t, authentication.Principal, stub.principal)
			assert.NotEqual(t, uuid.Nil, stub.orgID)
			assert.NotEqual(t, uuid.Nil, stub.teamID)
		})
	}
}
func TestContentAccessEventsRecheckMembershipAndRejectBeforeStreaming(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		stub := &changingContentAccess{statuses: []string{"pending"}, failAt: failAt}
		handler := &ResourceHandler{contentAccess: stub}
		writer := httptest.NewRecorder()
		handler.contentAccessEvents(writer, keyAccessEventRequest())
		assert.Equal(t, failAt, stub.calls)
		if failAt == 1 {
			assert.Equal(t, http.StatusNotFound, writer.Code)
			assert.False(t, writer.Flushed)
		} else {
			assert.Equal(t, 1, strings.Count(writer.Body.String(), "event: content-access"))
		}
	}
}
func TestContentAccessEventsRequireAuthenticatedSession(t *testing.T) {
	router := NewRouter(RouterConfig{})
	writer := httptest.NewRecorder()
	router.Handler().ServeHTTP(writer, httptest.NewRequest(http.MethodGet,
		"/api/v1/orgs/10000000-0000-0000-0000-000000000001/teams/20000000-0000-0000-0000-000000000002/content-access/events", nil))
	assert.NotEqual(t, http.StatusOK, writer.Code)
	assert.NotEqual(t, http.StatusNotFound, writer.Code)
}
